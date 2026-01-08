package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

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
	IncludeNamespaces []string `json:"include-namespace,omitempty"`
	ExcludeNamespaces []string `json:"exclude-namespace,omitempty"`
	HighRatio         float64  `json:"high-ratio,omitempty"`
}

// Plugin implements the memory management NRI plugin
type Plugin struct {
	stub.Stub
	config *Config
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

// Default returns the default configuration
func (p *Plugin) Default() plugins.Configer {
	return &Config{HighRatio: 0.8}
}

// Configure loads the plugin configuration and detects cgroup version
func (p *Plugin) Configure(ctx context.Context, config, runtime, version string) (stub.EventMask, error) {
	logger := log.G(ctx).WithField(plugins.FieldName, PluginName)
	logger.Info("Configuring memory plugin")

	// Override with config parameter if provided
	if config != "" {
		tempConfig := &Config{}
		if err := json.Unmarshal([]byte(config), tempConfig); err != nil {
			logger.WithError(err).Error("Failed to parse config parameter")
			return 0, fmt.Errorf("failed to parse config parameter: %w", err)
		}

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
	}
	// Validate final configuration
	if p.config.HighRatio <= 0 || p.config.HighRatio > 1 {
		return 0, fmt.Errorf("invalid high percentage: %f (must be between 0 and 1)", p.config.HighRatio)
	}

	logger.WithField("config", p.config).
		Info("Memory plugin configured")

	// Subscribe to container start events only
	var mask api.EventMask
	mask.Set(api.Event_START_CONTAINER)
	return mask, nil
}

// StartContainer handles container start events and sets memory.high
func (p *Plugin) StartContainer(ctx context.Context, pod *api.PodSandbox, container *api.Container) error {
	logger := log.G(ctx).WithField(plugins.FieldName, PluginName)

	// Check if we should process this namespace
	if !p.shouldProcessNamespace(pod.Namespace) {
		logger.WithField("namespace", pod.Namespace).Debug("Skipping namespace")
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
		Info("Setting memory.high for container")

	// Set memory.high using detected cgroup version
	if err := p.setMemoryHigh(ctx, container, memoryHigh); err != nil {
		logger.WithError(err).Error("Failed to set memory.high")
		return err
	}

	logger.WithField("container", container.Name).
		WithField("memory_high", memoryHigh).
		Info("Successfully set memory.high for container")

	return nil
}

// setMemoryHigh sets memory.high using the detected cgroup version
func (p *Plugin) setMemoryHigh(ctx context.Context, container *api.Container, memoryHigh int64) error {
	logger := log.G(ctx).WithField(plugins.FieldName, PluginName)

	// Get container cgroup path
	if container.Linux == nil || container.Linux.CgroupsPath == "" {
		return fmt.Errorf("container has no cgroup path")
	}

	cgroupPath := container.Linux.CgroupsPath
	logger.WithField("cgroup_path", cgroupPath).Debug("Using container cgroup path")

	// Prepare the value to write
	memoryHighStr := strconv.FormatInt(memoryHigh, 10)

	// For v1, we need to specify the subsystem
	subsystem := ""
	if !cgroups.IsCgroup2UnifiedMode() {
		subsystem = "memory"
	}

	// Write memory.high using the unified cgroup file writer
	if err := cgroup.WriteCgroupFile(cgroupPath, subsystem, "memory.high", memoryHighStr); err != nil {
		// For v1, memory.high might not be available
		if os.IsNotExist(err) {
			logger.Warn("memory.high not available, skipping")
			return nil
		}
		return fmt.Errorf("failed to write memory.high: %w", err)
	}

	logger.WithField("memory_high", memoryHigh).
		WithField("is_cgroupv2", cgroups.IsCgroup2UnifiedMode()).
		Info("Successfully set memory.high")
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

// ReadFrom implements the Configer interface
func (c *Config) ReadFrom(r io.Reader) (int64, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}

	if err := json.Unmarshal(data, c); err != nil {
		return 0, err
	}

	return int64(len(data)), nil
}

// WriteTo implements the Configer interface
func (c *Config) WriteTo(w io.Writer) (int64, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return 0, err
	}

	n, err := w.Write(data)
	return int64(n), err
}
