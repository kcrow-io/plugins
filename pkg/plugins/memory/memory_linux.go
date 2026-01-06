package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containerd/nri/pkg/api"
	"github.com/containerd/nri/pkg/stub"
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
	High              float64  `json:"high,omitempty"`
}

// Plugin implements the memory management NRI plugin
type Plugin struct {
	stub.Stub
	config      *Config
	isCgroupV2  bool
	memoryMount string
}

// New creates a new memory plugin instance
func New() *Plugin {
	return &Plugin{
		config: &Config{High: 0.8}, // Default to 80%
	}
}

// Name returns the plugin name
func (p *Plugin) Name() string {
	return PluginName
}

// Default returns the default configuration
func (p *Plugin) Default() plugins.Configer {
	return &Config{High: 0.8}
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
		if tempConfig.High > 0 {
			p.config.High = tempConfig.High
		}
	}

	// Validate final configuration
	if p.config.High <= 0 || p.config.High > 1 {
		return 0, fmt.Errorf("invalid high percentage: %f (must be between 0 and 1)", p.config.High)
	}

	// Detect cgroup version once at startup
	p.isCgroupV2 = cgroups.IsCgroup2UnifiedMode()

	// For cgroup v1, find memory mount point at startup
	if !p.isCgroupV2 {
		memoryMount, err := p.findMemoryMountPoint()
		if err != nil {
			logger.WithError(err).Warn("Failed to find memory mount point, will try at runtime")
		} else {
			p.memoryMount = memoryMount
		}
	}

	logger.WithField("cgroup_version", map[bool]string{true: "v2", false: "v1"}[p.isCgroupV2]).
		WithField("memory_mount", p.memoryMount).
		WithField("config", p.config).
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

	// Calculate memory.high value based on configured percentage
	memoryHigh := int64(float64(memoryLimit.Value) * p.config.High)

	logger.WithField("memory_limit", memoryLimit.Value).
		WithField("memory_high", memoryHigh).
		WithField("high_percentage", p.config.High).
		Info("Setting memory.high for container")

	// Set memory.high using detected cgroup version
	if err := p.setMemoryHigh(ctx, container, memoryHigh); err != nil {
		logger.WithError(err).Error("Failed to set memory.high")
		return err
	}

	logger.WithField("container_id", container.Id).
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

	// Use pre-detected cgroup version
	if p.isCgroupV2 {
		return p.setMemoryHighCgroupV2(ctx, cgroupPath, memoryHigh)
	} else {
		return p.setMemoryHighCgroupV1(ctx, cgroupPath, memoryHigh)
	}
}

// setMemoryHighCgroupV2 sets memory.high for cgroup v2
func (p *Plugin) setMemoryHighCgroupV2(ctx context.Context, cgroupPath string, memoryHigh int64) error {
	logger := log.G(ctx).WithField(plugins.FieldName, PluginName)
	logger.WithField("cgroup_version", "v2").Debug("Setting memory.high for cgroup v2")

	// Convert cgroup path to filesystem path
	// For cgroup v2, the path is usually /sys/fs/cgroup + cgroupPath
	fsPath := filepath.Join("/sys/fs/cgroup", strings.TrimPrefix(cgroupPath, "/"))

	// Ensure the directory exists
	if _, err := os.Stat(fsPath); os.IsNotExist(err) {
		return fmt.Errorf("cgroup directory does not exist: %s", fsPath)
	}

	// Write memory.high using cgroups.WriteFile
	memoryHighStr := strconv.FormatInt(memoryHigh, 10)
	if err := cgroups.WriteFile(fsPath, "memory.high", memoryHighStr); err != nil {
		return fmt.Errorf("failed to write memory.high in cgroup v2: %w", err)
	}

	logger.WithField("memory_high", memoryHigh).
		WithField("fs_path", fsPath).
		Info("Successfully set memory.high in cgroup v2")
	return nil
}

// setMemoryHighCgroupV1 sets memory.high for cgroup v1
func (p *Plugin) setMemoryHighCgroupV1(ctx context.Context, cgroupPath string, memoryHigh int64) error {
	logger := log.G(ctx).WithField(plugins.FieldName, PluginName)
	logger.WithField("cgroup_version", "v1").Debug("Setting memory.high for cgroup v1")

	// Use pre-detected memory mount point, or try to find it now
	memoryMountPoint := p.memoryMount
	if memoryMountPoint == "" {
		var err error
		memoryMountPoint, err = p.findMemoryMountPoint()
		if err != nil {
			return fmt.Errorf("failed to find memory mount point: %w", err)
		}
	}

	// Construct the full path to the memory cgroup
	var fsPath string
	if p.isSystemdDriver(cgroupPath) {
		// systemd driver: convert systemd path to filesystem path
		fsPath = p.convertSystemdPathToFs(memoryMountPoint, cgroupPath)
	} else {
		// cgroupfs driver: direct path mapping
		fsPath = filepath.Join(memoryMountPoint, strings.TrimPrefix(cgroupPath, "/"))
	}

	// Ensure the directory exists
	if _, err := os.Stat(fsPath); os.IsNotExist(err) {
		return fmt.Errorf("cgroup directory does not exist: %s", fsPath)
	}

	// Check if memory.high exists (it might not be available in cgroup v1)
	memoryHighPath := filepath.Join(fsPath, "memory.high")
	if _, err := os.Stat(memoryHighPath); os.IsNotExist(err) {
		logger.Warn("memory.high not available in cgroup v1, skipping")
		return nil
	}

	// Write memory.high using cgroups.WriteFile
	memoryHighStr := strconv.FormatInt(memoryHigh, 10)
	if err := cgroups.WriteFile(fsPath, "memory.high", memoryHighStr); err != nil {
		return fmt.Errorf("failed to write memory.high in cgroup v1: %w", err)
	}

	logger.WithField("memory_high", memoryHigh).
		WithField("fs_path", fsPath).
		Info("Successfully set memory.high in cgroup v1")
	return nil
}

// isSystemdDriver checks if the cgroup path indicates systemd driver
func (p *Plugin) isSystemdDriver(cgroupPath string) bool {
	return strings.Contains(cgroupPath, ".slice")
}

// convertSystemdPathToFs converts systemd cgroup path to filesystem path
func (p *Plugin) convertSystemdPathToFs(mountPoint, cgroupPath string) string {
	// systemd paths like /system.slice/containerd.service/kubepods.slice/...
	// are already in the correct format for filesystem access
	return filepath.Join(mountPoint, strings.TrimPrefix(cgroupPath, "/"))
}

// findMemoryMountPoint finds the memory subsystem mount point for cgroup v1
func (p *Plugin) findMemoryMountPoint() (string, error) {
	// Common memory subsystem mount points
	commonPaths := []string{
		"/sys/fs/cgroup/memory",
		"/sys/fs/cgroup/unified/memory",
	}

	// Try common paths first
	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// Fallback: parse /proc/mounts to find memory cgroup mount
	return p.parseMemoryMountFromProc()
}

// parseMemoryMountFromProc parses /proc/mounts to find memory cgroup mount point
func (p *Plugin) parseMemoryMountFromProc() (string, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return "", fmt.Errorf("failed to read /proc/mounts: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[2] == "cgroup" {
			// Check if this mount contains memory subsystem
			if strings.Contains(fields[3], "memory") {
				return fields[1], nil
			}
		}
	}

	return "", fmt.Errorf("memory cgroup mount point not found in /proc/mounts")
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
