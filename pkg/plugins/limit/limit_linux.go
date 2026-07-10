package limit

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/nri/pkg/api"
	"github.com/containerd/nri/pkg/stub"
	"github.com/kcrow-io/plugins/pkg/cgroup"
	"github.com/kcrow-io/plugins/pkg/containerd"
	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/kcrow-io/plugins/pkg/plugins"
	"github.com/sirupsen/logrus"
)

var _ plugins.Pluginer = (*Plugin)(nil)

const (
	name = "limit"

	containerdKindKey  = "io.cri-containerd.kind"
	containerKindValue = "container"
)

type Plugin struct {
	log    *logrus.Entry
	config *Config

	device *DeviceNumber

	// limitedContainers tracks which containers have IO limits applied
	// key: container ID, value: true if limited
	limitedContainers map[string]bool
	mu                sync.RWMutex
}

// New creates a new io plugin
func New() *Plugin {
	return &Plugin{
		log:               log.G(context.Background()).WithField("plugin", name),
		config:            NewConfig(),
		limitedContainers: make(map[string]bool),
	}
}

// Name returns the plugin name
func (p *Plugin) Name() string {
	return name
}

// Configure is called by NRI to configure the plugin
func (p *Plugin) Configure(ctx context.Context, config, runtime, version string) (stub.EventMask, error) {
	p.log.Infof("Configuring plugin (runtime: %s, version: %s)", runtime, version)
	var mask api.EventMask
	mask.Set(api.Event_CREATE_CONTAINER)

	err := p.config.Parse([]byte(config))
	if err != nil {
		return 0, fmt.Errorf("failed to parse config: %w", err)
	}
	if p.config.LogPath != "" {
		if err := log.SetupFileLogging(p.config.LogPath); err != nil {
			p.log.WithError(err).Warnf("Failed to setup file logging to %s, continuing with stdout", p.config.LogPath)
		}
	}
	p.log.Infof("plugin config: %+v", p.config)
	log.PrintBuildInfo(ctx)
	if p.config.Disabled {
		p.log.Infof("plugin is disabled, returning")
		return mask, nil
	}
	// Detect device number using containerd root
	device, err := GetDeviceNumberFromPath(p.config.cntrd.Root)
	if err != nil {
		return 0, fmt.Errorf("failed to get device number: %w", err)
	}

	p.log.Infof("Detected device number: %s", device.String())
	p.device = device

	// Create and start watcher with worker pool
	watcher, err := containerd.NewWatcher(p.config.cntrd, &containerd.Config{
		WatchInterval:  time.Duration(p.config.WatchInterval) * time.Second,
		WorkerPoolSize: 4, // Use 4 workers for parallel processing
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create watcher: %w", err)
	}

	// Subscribe to blkio limit checking
	watcher.Subscribe("blkio-limiter", func(ctx context.Context, container client.Container) bool {
		id := container.ID()
		if len(id) > 6 {
			id = id[:6]
		}
		if p.config.Io.MaxDiskBytes == 0 {
			return false
		}
		// Check if this is a container (not sandbox)
		ok, err := isRunningContainer(ctx, container)
		if err != nil {
			p.log.Debugf("Failed to get container for %s: %v", id, err)
			return true // continue
		}
		if !ok {
			return true // continue
		}
		p.limitblkio(ctx, id, container)
		return true // continue to next container
	})

	// Subscribe to cache clearing
	watcher.Subscribe("cache-clearer", func(ctx context.Context, container client.Container) bool {
		id := container.ID()
		if len(id) > 6 {
			id = id[:6]
		}
		// skip if all-usage-percent not set, which mean no limit
		if p.config.Memory.PodsUsagePercent == 0 {
			return false
		}
		// Check if this is a container (not sandbox)
		ok, err := isRunningContainer(ctx, container)
		if err != nil {
			p.log.Debugf("Failed to get container info for %s: %v", id, err)
			return true // continue
		}
		if !ok {
			return true // continue
		}

		// Get container info for labels
		info, err := container.Info(ctx)
		if err != nil {
			p.log.Debugf("Failed to get container info for %s: %v", id, err)
			return true // continue
		}
		containerName := info.Labels["io.cri-containerd.name"]
		podNamespace := info.Labels["io.kubernetes.pod.namespace"]

		// check pods memory usage
		spec, err := container.Spec(ctx)
		if err != nil {
			p.log.Errorf("Failed to get container spec for %s: %v", id, err)
			return true // continue
		}

		cgroupPath := spec.Linux.CgroupsPath
		usage, limit, err := cgroup.GetFirstMemory(cgroupPath)
		if err != nil {
			p.log.Errorf("Failed to get root memory usage: %v", err)
			return true // continue
		}
		// skip if allpods usage not exceeded
		if limit != 0 && uint64(float64(usage)/float64(limit)*100) < p.config.Memory.PodsUsagePercent {
			return false
		}

		p.clearCache(ctx, id, cgroupPath, containerName, podNamespace)
		return true // continue to next container
	})

	watcher.Start()

	p.log.Info("Plugin configured successfully")
	return mask, nil
}

func (p *Plugin) limitblkio(ctx context.Context, id string, container client.Container) {
	// Get container info to check snapshotter
	info, err := container.Info(ctx)
	if err != nil {
		p.log.Debugf("Failed to get container info for %s: %v", id, err)
		return
	}
	// Only process containers with overlayfs snapshotter
	if info.Snapshotter != "overlayfs" {
		return
	}

	// Extract container name and pod namespace from labels
	containerName := info.Labels["io.cri-containerd.name"]
	podNamespace := info.Labels["io.kubernetes.pod.namespace"]

	if len(p.config.Io.ExcludeNamespaces) > 0 && slices.Contains(p.config.Io.ExcludeNamespaces, podNamespace) {
		return
	}

	usage, err := p.config.cntrd.SnapshotService(info.Snapshotter).Usage(ctx, info.SnapshotKey)
	if err != nil {
		p.log.Debugf("Failed to get snapshot usage for %s: %v", id, err)
		return
	}

	spec, err := container.Spec(ctx)
	if err != nil {
		p.log.Errorf("Failed to get container spec for %s: %v", id, err)
		return
	}

	cgroupPath := spec.Linux.CgroupsPath
	diskUsage := usage.Size
	fullID := container.ID()

	// Check current limit status from map
	p.mu.RLock()
	hasLimit := p.limitedContainers[fullID]
	p.mu.RUnlock()

	if diskUsage > int64(p.config.Io.MaxDiskBytes) {
		// Disk usage exceeds threshold
		if !hasLimit {
			// Apply limits (both BPS and IOPS if configured)
			if err := ApplyIOLimit(cgroupPath, p.device, p.config.Io); err != nil {
				p.log.WithField("container_name", containerName).WithField("pod_namespace", podNamespace).
					Errorf("Failed to apply io limit to container %s (cgroup: %s): %v", id, cgroupPath, err)
			} else {
				logMsg := fmt.Sprintf("Applied io limit to container %s (cgroup: %s, disk usage: %d bytes > threshold: %d bytes, bps_limit: %d",
					id, cgroupPath, diskUsage, p.config.Io.MaxDiskBytes, p.config.Io.BpsLimit)
				if p.config.Io.IopsLimit > 0 {
					logMsg += fmt.Sprintf(", iops_limit: %d", p.config.Io.IopsLimit)
				}
				logMsg += ")"
				p.log.WithField("container_name", containerName).WithField("pod_namespace", podNamespace).Info(logMsg)

				// Mark as limited in map
				p.mu.Lock()
				p.limitedContainers[fullID] = true
				p.mu.Unlock()
			}
		}
	} else {
		// Disk usage is below threshold
		if hasLimit {
			// Remove limits
			if err := ApplyIOLimit(cgroupPath, p.device, nil); err != nil {
				p.log.WithField("container_name", containerName).WithField("pod_namespace", podNamespace).
					Errorf("Failed to remove io limit from container %s (cgroup: %s): %v", id, cgroupPath, err)
			} else {
				p.log.WithField("container_name", containerName).WithField("pod_namespace", podNamespace).
					Infof("Removed io limit from container %s (cgroup: %s, disk usage: %d bytes < threshold: %d bytes)",
						id, cgroupPath, diskUsage, p.config.Io.MaxDiskBytes)

				// Remove from map
				p.mu.Lock()
				delete(p.limitedContainers, fullID)
				p.mu.Unlock()
			}
		}
	}
}

// checks a single container and clear cache
func (p *Plugin) clearCache(ctx context.Context, id string, cgroupPath string, containerName, podNamespace string) {
	statpath, err := cgroup.GetCgroupFilePath(cgroupPath, "memory", "memory.stat")
	if err != nil {
		return
	}
	memstat, err := parseStat(ctx, statpath)
	if err != nil {
		return
	}

	if memstat.cache > p.config.Memory.MinCacheBytes && memstat.ratio > p.config.Memory.CacheRssRatio {
		// Memory usage exceeds threshold, Apply limit
		p.log.WithField("container_id", id).WithField("container_name", containerName).WithField("pod_namespace", podNamespace).
			Infof("Memory exceeds, rss(%d), cache(%d), ratio(%.2f)", memstat.rss, memstat.cache, memstat.ratio)
		if err := ApplyCleanCache(cgroupPath, memstat.cache); err != nil {
			p.log.WithField("container_id", id).WithField("container_name", containerName).WithField("pod_namespace", podNamespace).
				WithError(err).Error("Failed to clear cache")
		} else {
			p.log.WithField("container_id", id).WithField("container_name", containerName).WithField("pod_namespace", podNamespace).
				Infof("Successfully cleared cache (%d bytes)", memstat.cache)
		}
	}
}

func (p *Plugin) CreateContainer(ctx context.Context, pod *api.PodSandbox, container *api.Container) (*api.ContainerAdjustment, []*api.ContainerUpdate, error) {
	return &api.ContainerAdjustment{}, nil, nil
}

func isRunningContainer(ctx context.Context, container client.Container) (bool, error) {
	// Check if this is a container (not sandbox)
	info, err := container.Info(ctx)
	if err != nil {
		return false, err
	}
	if len(info.Labels) == 0 || info.Labels[containerdKindKey] != containerKindValue {
		return false, nil
	}
	task, err := container.Task(ctx, nil)
	if err != nil {
		return false, err
	}
	status, err := task.Status(ctx)
	if err != nil {
		return false, err
	}
	return status.Status == client.Running, nil
}
