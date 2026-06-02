package escape

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"syscall"
	"time"

	"github.com/containerd/nri/pkg/api"
	"github.com/containerd/nri/pkg/stub"
	"github.com/kcrow-io/plugins/pkg/cgroup"
	"github.com/kcrow-io/plugins/pkg/containerd"
	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/kcrow-io/plugins/pkg/plugins"
	"github.com/opencontainers/cgroups"
	"github.com/sirupsen/logrus"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var (
	_ plugins.Pluginer                 = (*escape)(nil)
	_ stub.PostStartContainerInterface = (*escape)(nil)
)

const (
	name = "escape"

	// Default CRI socket path
	defaultCRISocket = "unix:///run/containerd/containerd.sock"

	// Default cleanup interval for escaped containers
	defaultCleanupInterval = 30 * time.Second
)

// Config represents the escape plugin configuration
type Config struct {
	// LogPath is the path to the log file
	LogPath string `json:"log_path,omitempty"`
	// CRISocket is the path to the CRI socket
	CRISocket string `json:"cri_socket,omitempty"`
	// CleanupInterval is the interval in seconds for checking escaped containers
	CleanupInterval int `json:"cleanup_interval,omitempty"`
}

type escape struct {
	log    *logrus.Entry
	config *Config

	// iscgv2 indicates whether the system is using cgroup v2
	iscgv2 bool

	// escapedContainers tracks container ID to escape info
	// key: container ID, value: escapeInfo
	escapedContainers map[string]*escapeInfo
	mu                sync.RWMutex

	// criClient is the CRI client for getting container status
	criClient *containerd.CRIClient
}

// escapeInfo contains information about an escaped container
type escapeInfo struct {
	// initPid is the PID of the container's init process
	initPid int
	// mountNs is the mount namespace inode
	mountNs uint64
}

func (e *escape) Name() string {
	return name
}

func (e *escape) Default() plugins.Configer {
	return &plugins.NopConfig{}
}

func (e *escape) Configure(ctx context.Context, config, runtime, version string) (api.EventMask, error) {
	var (
		mask api.EventMask
	)

	e.config = &Config{
		CRISocket:       defaultCRISocket,
		CleanupInterval: int(defaultCleanupInterval.Seconds()),
	}

	// Parse config if provided
	if config != "" {
		if err := json.Unmarshal([]byte(config), e.config); err != nil {
			return 0, fmt.Errorf("failed to parse config: %w", err)
		}
	}

	// Validate cleanup interval
	if e.config.CleanupInterval <= 0 {
		e.config.CleanupInterval = int(defaultCleanupInterval.Seconds())
	}

	// Setup file logging if log_path is configured
	if e.config.LogPath != "" {
		if err := log.SetupFileLogging(e.config.LogPath); err != nil {
			return 0, fmt.Errorf("failed to setup logging to %s: %w", e.config.LogPath, err)
		}
	}

	e.log = log.G(ctx).WithField(plugins.FieldName, name)

	// Detect cgroup version
	e.iscgv2 = cgroups.IsCgroup2UnifiedMode()
	e.log.WithField("cgroupv2", e.iscgv2).Info("Detected cgroup version")

	// Only initialize CRI client and sync on cgroupv2
	if e.iscgv2 {
		criClient, err := containerd.NewCRIClient(e.config.CRISocket)
		if err != nil {
			e.log.WithError(err).Warnf("Failed to create CRI client, escape detection will be limited")
		} else {
			e.criClient = criClient
			e.log.Infof("CRI client connected to %s", e.config.CRISocket)

			// Do initial sync
			go e.syncContainerStatus(context.Background())
		}
	}

	// Subscribe events
	mask.Set(api.Event_POST_START_CONTAINER)

	e.log.WithFields(logrus.Fields{
		"runtime":          runtime,
		"version":          version,
		"cgroupv2":         e.iscgv2,
		"cleanup_interval": e.config.CleanupInterval,
	}).Infof("Configure plugin, handler event: %s", mask.PrettyString())

	return mask, nil
}

// PostStartContainer handles container post-start events and updates escaped containers mapping
func (e *escape) PostStartContainer(ctx context.Context, pod *api.PodSandbox, container *api.Container) error {
	// If not cgroupv2, skip
	if !e.iscgv2 {
		return nil
	}

	logger := e.log.WithFields(logrus.Fields{
		"container_name": container.Name,
		"pod":            pod.Name,
		"namespace":      pod.Namespace,
		"pid":            container.Pid,
	})

	// Get container init PID
	initPid, err := e.getContainerInitPid(container)
	if err != nil {
		logger.WithError(err).Debug("Failed to get container init PID")
		return nil
	}

	// Get expected cgroup path from container spec
	expectedCgroupPath := ""
	if container.Linux != nil {
		expectedCgroupPath = container.Linux.CgroupsPath
	}

	escaped := false

	// Check using expected cgroup path if available
	if expectedCgroupPath != "" {
		escaped, err = cgroup.IsEscapedFromCgroup(initPid, expectedCgroupPath)
		if err != nil {
			logger.WithError(err).Debug("Failed to check cgroup escape")
			return nil
		}
	} else {
		// Fallback: check if in root cgroup
		escaped, err = cgroup.IsInRootCgroup(initPid)
		if err != nil {
			logger.WithError(err).Debug("Failed to check cgroup")
			return nil
		}
	}

	if !escaped {
		return nil
	}

	// Get mount namespace of container init process
	initMntNs, err := cgroup.GetMountNsInode(initPid)
	if err != nil {
		logger.WithError(err).Debug("Failed to get init process mount namespace")
		return nil
	}

	// Store container ID to escape info
	e.mu.Lock()
	e.escapedContainers[container.Id] = &escapeInfo{
		initPid: initPid,
		mountNs: initMntNs,
	}
	e.mu.Unlock()

	// Get actual cgroup path for logging
	actualCgroup, _ := cgroup.GetCgroupPath(initPid)
	logger.WithFields(logrus.Fields{
		"init_pid":        initPid,
		"mount_ns":        initMntNs,
		"expected_cgroup": expectedCgroupPath,
		"actual_cgroup":   actualCgroup,
	}).Warn("Container escaped from expected cgroup")

	return nil
}

// syncContainerStatus syncs container status from CRI and checks for escape
func (e *escape) syncContainerStatus(ctx context.Context) {
	if e.criClient == nil {
		return
	}

	// Retry ListContainers until success or context cancelled
	var containers []*runtimeapi.Container
	retryInterval := 5 * time.Second
	for {
		var err error
		containers, err = e.criClient.ListContainers(ctx, nil)
		if err == nil {
			break
		}
		e.log.WithError(err).Warn("Failed to list containers, retrying...")
		select {
		case <-ctx.Done():
			e.log.Info("Context cancelled while waiting to list containers")
			return
		case <-time.After(retryInterval):
			continue
		}
	}

	e.log.Debugf("Syncing %d containers", len(containers))

	for _, container := range containers {
		// Get container status response
		resp, err := e.criClient.GetContainerStatus(ctx, container.Id)
		if err != nil {
			e.log.WithError(err).WithField("container_id", container.Id).Debug("Failed to get container status")
			continue
		}

		// Skip if container is not running
		if resp.Status.State != runtimeapi.ContainerState_CONTAINER_RUNNING {
			continue
		}

		// Get PID from response info
		pid, err := e.getContainerPidFromResponse(resp)
		if err != nil {
			e.log.WithError(err).WithField("container_id", container.Id).Debug("Failed to get container PID")
			continue
		}

		// Check if container has escaped
		escaped, err := cgroup.IsInRootCgroup(int(pid))
		if err != nil {
			e.log.WithError(err).WithField("container_id", container.Id).Debug("Failed to check cgroup")
			continue
		}

		if !escaped {
			continue
		}

		// Get mount namespace of container init process
		initMntNs, err := cgroup.GetMountNsInode(int(pid))
		if err != nil {
			e.log.WithError(err).WithField("container_id", container.Id).Debug("Failed to get init process mount namespace")
			continue
		}

		// Store container ID to escape info
		e.mu.Lock()
		e.escapedContainers[container.Id] = &escapeInfo{
			initPid: int(pid),
			mountNs: initMntNs,
		}
		e.mu.Unlock()

		// Log escape detection
		actualCgroup, _ := cgroup.GetCgroupPath(int(pid))
		e.log.WithFields(logrus.Fields{
			"container_id":  container.Id,
			"init_pid":      pid,
			"mount_ns":      initMntNs,
			"actual_cgroup": actualCgroup,
		}).Warn("Container escaped from expected cgroup")
	}

	go e.cleanupEscapedContainers(ctx)
}

// getContainerPidFromResponse extracts the PID from container status response
func (e *escape) getContainerPidFromResponse(resp *runtimeapi.ContainerStatusResponse) (uint32, error) {
	// Try to get PID from info map
	if resp.Info != nil {
		// containerd stores PID in "pid" field
		if pidStr, ok := resp.Info["pid"]; ok {
			var pid uint32
			if _, err := fmt.Sscanf(pidStr, "%d", &pid); err == nil {
				return pid, nil
			}
		}
	}

	// Fallback: try to get from sandbox info if available
	// This is a simplified approach - may need enhancement based on actual CRI response format
	return 0, fmt.Errorf("PID not found in container status info")
}

// cleanupEscapedContainers periodically checks and cleans up escaped containers
func (e *escape) cleanupEscapedContainers(ctx context.Context) {
	interval := time.Duration(e.config.CleanupInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	e.log.WithField("interval", interval).Info("Starting periodic cleanup goroutine for escaped containers")

	for {
		select {
		case <-ctx.Done():
			e.log.Info("Stopping periodic cleanup goroutine")
			return
		case <-ticker.C:
			e.doCleanup(ctx)
		}
	}
}

// doCleanup performs one cleanup cycle for escaped containers
func (e *escape) doCleanup(ctx context.Context) {
	// Copy escaped containers map to avoid holding lock during cleanup
	e.mu.RLock()
	containersToCleanup := make(map[string]*escapeInfo, len(e.escapedContainers))
	for k, v := range e.escapedContainers {
		containersToCleanup[k] = v
	}
	e.mu.RUnlock()

	if len(containersToCleanup) == 0 {
		return
	}

	e.log.Debugf("Checking %d escaped containers", len(containersToCleanup))

	for containerID, info := range containersToCleanup {
		logger := e.log.WithFields(logrus.Fields{
			"container_id": containerID,
			"init_pid":     info.initPid,
			"mount_ns":     info.mountNs,
		})
		// Check if container still exists via CRI
		if e.criClient != nil {
			resp, err := e.criClient.GetContainerStatus(ctx, containerID)
			if err == nil && resp.Status.State == runtimeapi.ContainerState_CONTAINER_RUNNING {
				logger.Debug("Container still running in CRI, skipping cleanup")
				continue
			}
		}

		logger.Info("Container gone, cleaning up mount namespace processes")

		// Find processes in the same mount namespace
		pids, err := cgroup.FindProcessesWithMountNs(info.mountNs)
		if err != nil {
			logger.WithError(err).Error("Failed to find processes with mount namespace")
			continue
		}

		if len(pids) == 0 {
			// No processes found, remove from map
			e.mu.Lock()
			delete(e.escapedContainers, containerID)
			e.mu.Unlock()
			logger.Info("No processes found in mount namespace, removed from tracking")
			continue
		}

		logger.WithField("pids", pids).Infof("Found %d processes to cleanup", len(pids))

		// Cleanup processes
		cleaned := e.cleanupProcesses(logger, pids)

		// Remove from escaped containers map
		e.mu.Lock()
		delete(e.escapedContainers, containerID)
		e.mu.Unlock()

		logger.WithField("cleaned", cleaned).Info("Container cleanup completed")
	}
}

// processExists checks if a process with the given PID exists
func (e *escape) processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil
}

// cleanupProcesses sends SIGTERM and SIGKILL to processes
func (e *escape) cleanupProcesses(logger *logrus.Entry, pids []int) int {
	cleaned := 0

	for _, pid := range pids {
		procName := cgroup.GetProcessName(pid)
		processLogger := logger.WithFields(logrus.Fields{
			"pid":          pid,
			"process_name": procName,
		})

		// Send SIGTERM
		processLogger.Info("Sending SIGTERM")
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			processLogger.WithError(err).Warn("Failed to send SIGTERM")
			continue
		}

		// Wait a bit for graceful shutdown
		time.Sleep(3 * time.Second)

		// Check if process still exists
		if !e.processExists(pid) {
			processLogger.Info("Process terminated after SIGTERM")
			cleaned++
			continue
		}

		// Process still running, send SIGKILL
		processLogger.Info("Process still running, sending SIGKILL")
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
			processLogger.WithError(err).Warn("Failed to send SIGKILL")
			continue
		}

		processLogger.Info("Process killed with SIGKILL")
		cleaned++
	}

	return cleaned
}

func (e *escape) getContainerInitPid(container *api.Container) (int, error) {
	if container.Pid == 0 {
		return 0, fmt.Errorf("container PID is 0")
	}
	return int(container.Pid), nil
}

func New() plugins.Pluginer {
	return &escape{
		config: &Config{
			CRISocket: defaultCRISocket,
		},
		escapedContainers: make(map[string]*escapeInfo),
	}
}
