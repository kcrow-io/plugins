package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/containerd/nri/pkg/api"
	"github.com/containerd/nri/pkg/stub"
	"github.com/kcrow-io/plugins/pkg/cgroup"
	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/kcrow-io/plugins/pkg/plugins"
	"github.com/opencontainers/cgroups"
)

const (
	PluginName = "memory"
	PluginIdx  = "10"
)

// Config represents the memory plugin configuration
type Config struct {
	// Disabled indicates whether the plugin is disabled
	Disabled bool `json:"disabled,omitempty"`

	IncludeNamespaces []string `json:"include-namespace,omitempty"`
	ExcludeNamespaces []string `json:"exclude-namespace,omitempty"`
	HighRatio         float64  `json:"high-ratio,omitempty"`
	LogPath           string   `json:"log_path,omitempty"`
}

// Plugin implements the memory management NRI plugin
type Plugin struct {
	stub.Stub
	config              *Config
	parentMemoryHighSet bool // tracks if kubepods.slice/memory.high has been set
}

// New creates a new memory plugin instance
func New() *Plugin {
	return &Plugin{
		config: &Config{
			HighRatio: 0.8, // Default to 80% memory usage
		},
	}
}

// Name returns the plugin name
func (p *Plugin) Name() string {
	return PluginName
}

// Configure loads the plugin configuration and detects cgroup version
func (p *Plugin) Configure(ctx context.Context, config, runtime, version string) (stub.EventMask, error) {
	logger := log.G(ctx).WithField(plugins.FieldName, PluginName)
	logger.Info("Configuring memory plugin")
	var mask api.EventMask
	mask.Set(api.Event_START_CONTAINER)
	// Override with config parameter if provided
	if config != "" {
		tempConfig := &Config{}
		if err := json.Unmarshal([]byte(config), tempConfig); err != nil {
			logger.WithError(err).Error("Failed to parse config parameter")
			return 0, fmt.Errorf("failed to parse config parameter: %w", err)
		}
		p.config.Disabled = tempConfig.Disabled
		// Merge config, being careful with float 0 values
		if len(tempConfig.IncludeNamespaces) > 0 {
			p.config.IncludeNamespaces = tempConfig.IncludeNamespaces
		}
		if len(tempConfig.ExcludeNamespaces) > 0 {
			p.config.ExcludeNamespaces = tempConfig.ExcludeNamespaces
		}
		// Only override High if it's explicitly set (not 0)
		if tempConfig.HighRatio > 0 {
			p.config.HighRatio = tempConfig.HighRatio
		}
		// Setup file logging if log_path is provided
		if tempConfig.LogPath != "" {
			p.config.LogPath = tempConfig.LogPath
			if err := log.SetupFileLogging(p.config.LogPath); err != nil {
				logger.WithError(err).Warnf("Failed to setup file logging to %s, continuing with stdout", p.config.LogPath)
			}
		}
	}
	log.PrintBuildInfo(ctx)
	if p.config.Disabled {
		logger.Info("Memory plugin disabled")
		return mask, nil
	}
	// Validate final configuration
	if p.config.HighRatio <= 0 || p.config.HighRatio > 1 {
		return 0, fmt.Errorf("invalid high percentage: %f (must be between 0 and 1)", p.config.HighRatio)
	}

	logger.WithField("config", p.config).
		Info("Memory plugin configured")

	// Subscribe to container start events only
	return mask, nil
}

// StartContainer handles container start events and sets memory.high
func (p *Plugin) StartContainer(ctx context.Context, pod *api.PodSandbox, container *api.Container) error {
	logger := log.G(ctx).WithField(plugins.FieldName, PluginName).
		WithField("container_name", container.Name).
		WithField("pod_namespace", pod.Namespace)

	if p.config.Disabled {
		return nil
	}

	// Check if we should process this namespace
	if !p.shouldProcessNamespace(pod.Namespace) {
		logger.Debug("Skipping namespace")
		return nil
	}

	// Check if container has memory limit - if not, skip
	if container.Linux == nil || container.Linux.Resources == nil || container.Linux.Resources.Memory == nil {
		logger.Debug("Container has no memory resources configured, skipping")
		return nil
	}

	memoryLimit := container.Linux.Resources.Memory.Limit
	if memoryLimit == nil || memoryLimit.Value <= 0 {
		logger.Debug("Container has no memory limit set, skipping")
		return nil
	}

	// Calculate memory.high value based on configured ratio
	memoryHigh := int64(float64(memoryLimit.Value) * p.config.HighRatio)

	logger.WithField("memory_limit", memoryLimit.Value).
		WithField("memory_high", memoryHigh).
		WithField("high_ratio", p.config.HighRatio).
		Info("Setting memoryHigh for container")

	// Set memory.high using detected cgroup version
	if err := p.setMemoryHigh(ctx, container, memoryHigh); err != nil {
		logger.WithError(err).Warningf("Failed to set memoryHigh")
	}

	// Set kubepods memory.high/soft_limit on first container start
	if !p.parentMemoryHighSet {
		if err := p.setParentMemoryHigh(ctx, container); err != nil {
			logger.WithError(err).Warningf("Failed to set kubepods memory limit")
		}
	}

	return nil
}

// setMemoryHigh sets memory.high (v2) or memory.soft_limit_in_bytes (v1)
func (p *Plugin) setMemoryHigh(ctx context.Context, container *api.Container, memoryHigh int64) error {
	logger := log.G(ctx).WithField(plugins.FieldName, PluginName).
		WithField("container_id", container.Id).
		WithField("container_name", container.Name)

	// Get container cgroup path
	if container.Linux == nil || container.Linux.CgroupsPath == "" {
		return fmt.Errorf("container has no cgroup path")
	}

	cgroupPath := container.Linux.CgroupsPath
	logger.WithField("cgroup_path", cgroupPath).Debug("Using container cgroup path")

	// Prepare the value to write
	memoryHighStr := strconv.FormatInt(memoryHigh, 10)

	var err error
	if cgroups.IsCgroup2UnifiedMode() {
		// For cgroup v2, use memory.high
		err = cgroup.WriteCgroupFile(cgroupPath, "", "memory.high", memoryHighStr)
		if err != nil {
			return fmt.Errorf("failed to write memory.high: %w", err)
		}
		logger.WithField("memory_high", memoryHigh).
			Info("Successfully set memory.high")
	} else {
		// For cgroup v1, use memory.soft_limit_in_bytes
		err = cgroup.WriteCgroupFile(cgroupPath, "memory", "memory.soft_limit_in_bytes", memoryHighStr)
		if err != nil {
			return fmt.Errorf("failed to write memory.soft_limit_in_bytes: %w", err)
		}
		logger.WithField("memory_soft_limit", memoryHigh).
			Info("Successfully set memory.soft_limit_in_bytes")
	}

	return nil
}

// setParentMemoryHigh sets memory.high (v2) or memory.soft_limit_in_bytes (v1) for kubepods
// This is only called once (on first container start) when set-parent-memory-high is enabled
func (p *Plugin) setParentMemoryHigh(ctx context.Context, container *api.Container) error {
	logger := log.G(ctx).WithField(plugins.FieldName, PluginName)

	if container.Linux == nil || container.Linux.CgroupsPath == "" {
		return fmt.Errorf("container has no cgroup path")
	}

	cgroupPath := container.Linux.CgroupsPath
	normalizedPath := cgroup.NormalizeCgroupPath(cgroupPath)

	if normalizedPath == "" {
		return fmt.Errorf("cgroup path is empty")
	}

	// Extract kubepods path (first level)
	kubepods, _, _ := strings.Cut(normalizedPath, "/")
	if kubepods == "" {
		return fmt.Errorf("could not extract kubepods path from: %s", normalizedPath)
	}

	logger.WithField("kubepods_path", kubepods).Debug("Found kubepods cgroup path")

	var (
		memoryLimitPath  string
		memoryLimitFile  string
		memoryTargetFile string
	)

	if cgroups.IsCgroup2UnifiedMode() {
		// cgroup v2 paths
		memoryLimitPath = fmt.Sprintf("/sys/fs/cgroup/%s/memory.max", kubepods)
		memoryLimitFile = "memory.max"
		memoryTargetFile = "memory.high"
	} else {
		// cgroup v1 paths
		memoryLimitPath = fmt.Sprintf("/sys/fs/cgroup/memory/%s/memory.limit_in_bytes", kubepods)
		memoryLimitFile = "memory.limit_in_bytes"
		memoryTargetFile = "memory.soft_limit_in_bytes"
	}

	// Read memory limit from kubepods
	data, err := os.ReadFile(memoryLimitPath)
	if err != nil {
		return fmt.Errorf("failed to read %s from %s: %w", memoryLimitFile, kubepods, err)
	}

	memoryLimitStr := strings.TrimSpace(string(data))

	// Handle "max" value (v2) or very large values indicating unlimited
	if memoryLimitStr == "max" {
		logger.Infof("kubepods %s is 'max' (unlimited), skipping %s setting", memoryLimitFile, memoryTargetFile)
		return nil
	}

	memoryLimit, err := strconv.ParseInt(memoryLimitStr, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse %s value '%s': %w", memoryLimitFile, memoryLimitStr, err)
	}

	// Calculate target value based on ratio, with max reduction of 200MB
	memoryTarget := int64(float64(memoryLimit) * p.config.HighRatio)
	maxReduction := int64(200 * 1024 * 1024) // 200MB
	if memoryLimit-memoryTarget > maxReduction {
		memoryTarget = memoryLimit - maxReduction
	}
	if memoryTarget < 0 {
		logger.WithField(memoryLimitFile, memoryLimit).
			Warnf("kubepods high memory(%d) too low, skipping %s setting", memoryTarget, memoryTargetFile)
		p.parentMemoryHighSet = true
		return nil
	}

	logger.WithField(memoryLimitFile, memoryLimit).
		WithField(memoryTargetFile, memoryTarget).
		WithField("high_ratio", p.config.HighRatio).
		Infof("Setting kubepods %s", memoryTargetFile)

	// Write to kubepods
	memoryTargetStr := strconv.FormatInt(memoryTarget, 10)
	var parentPath string
	if cgroups.IsCgroup2UnifiedMode() {
		parentPath = fmt.Sprintf("/sys/fs/cgroup/%s/%s", kubepods, memoryTargetFile)
	} else {
		parentPath = fmt.Sprintf("/sys/fs/cgroup/memory/%s/%s", kubepods, memoryTargetFile)
	}

	if err := os.WriteFile(parentPath, []byte(memoryTargetStr), 0644); err != nil {
		return fmt.Errorf("failed to write %s to %s: %w", memoryTargetFile, parentPath, err)
	}

	// Mark as set
	p.parentMemoryHighSet = true
	logger.Infof("Successfully set kubepods %s", memoryTargetFile)

	return nil
}

// shouldProcessNamespace checks if the namespace should be processed
func (p *Plugin) shouldProcessNamespace(namespace string) bool {
	// If include list is specified, namespace must be in it
	if len(p.config.IncludeNamespaces) > 0 {
		for _, ns := range p.config.IncludeNamespaces {
			if ns == namespace {
				return true
			}
		}
		return false
	}

	// If exclude list is specified, namespace must not be in it
	if len(p.config.ExcludeNamespaces) > 0 {
		for _, ns := range p.config.ExcludeNamespaces {
			if ns == namespace {
				return false
			}
		}
	}

	return true
}
