package iolimit

import (
	"context"
	"time"

	"github.com/containerd/containerd/v2/client"
	"github.com/sirupsen/logrus"
)

// Watcher watches container stats and applies io limits
type Watcher struct {
	client            *client.Client
	config            *Config
	device            *DeviceNumber
	cgroupVersion     CgroupVersion
	containerdRoot    string
	snapshotter       string
	limitedContainers map[string]bool
	stopCh            chan struct{}
	stoppedCh         chan struct{}
}

// NewWatcher creates a new stats watcher
func NewWatcher(client *client.Client, config *Config, device *DeviceNumber, containerdRoot, snapshotter string) *Watcher {
	return &Watcher{
		client:            client,
		config:            config,
		device:            device,
		cgroupVersion:     DetectCgroupVersion(),
		containerdRoot:    containerdRoot,
		snapshotter:       snapshotter,
		limitedContainers: make(map[string]bool),
		stopCh:            make(chan struct{}),
		stoppedCh:         make(chan struct{}),
	}
}

// Start starts the watcher
func (w *Watcher) Start() {
	logrus.Infof("Starting io watcher (cgroup version: v%d, device: %s, threshold: %d bytes, limit: %d bps, interval: %d seconds)",
		w.cgroupVersion, w.device.String(), w.config.MaxDiskBytes, w.config.BpsLimit, w.config.WatchInterval)

	go w.watch()
}

// Stop stops the watcher
func (w *Watcher) Stop() {
	close(w.stopCh)
	<-w.stoppedCh
	logrus.Info("IO watcher stopped")
}

// watch is the main watch loop
func (w *Watcher) watch() {
	defer close(w.stoppedCh)

	ticker := time.NewTicker(time.Duration(w.config.WatchInterval) * time.Second)
	defer ticker.Stop()

	// Do an initial check immediately
	w.checkContainers()

	for {
		select {
		case <-ticker.C:
			w.checkContainers()
		case <-w.stopCh:
			return
		}
	}
}

// checkContainers checks all containers and applies/removes limits
func (w *Watcher) checkContainers() {
	ctx := context.Background()

	containers, err := w.client.Containers(ctx)
	if err != nil {
		logrus.Errorf("Failed to list containers: %v", err)
		return
	}

	for _, container := range containers {
		w.checkContainer(ctx, container)
	}
}

// checkContainer checks a single container and applies/removes limit
func (w *Watcher) checkContainer(ctx context.Context, container client.Container) {
	id := container.ID()

	// Get container info to check snapshotter
	info, err := container.Info(ctx)
	if err != nil {
		logrus.Debugf("Failed to get container info for %s: %v", id, err)
		return
	}

	// Only process containers with overlayfs snapshotter
	if info.Snapshotter != DefaultSnapshotter {
		return
	}
	usage, err := w.client.SnapshotService(info.Snapshotter).Usage(ctx, info.SnapshotKey)
	if err != nil {
		logrus.Debugf("Failed to get snapshot usage for %s: %v", id, err)
		return
	}
	// Get container spec to extract cgroup path
	spec, err := container.Spec(ctx)
	if err != nil {
		logrus.Debugf("Failed to get spec for container %s: %v", id, err)
		return
	}

	// Get cgroup path from spec
	if spec.Linux == nil || spec.Linux.CgroupsPath == "" {
		logrus.Debugf("Container %s has no cgroup path in spec", id)
		return
	}
	cgroupPath := spec.Linux.CgroupsPath

	// Extract disk usage from snapshotter
	diskUsage := usage.Size
	if diskUsage > w.config.MaxDiskBytes {
		// Disk usage exceeds threshold, Apply limit
		if err := ApplyIOLimit(cgroupPath, w.device, w.config.BpsLimit, w.cgroupVersion); err != nil {
			logrus.Errorf("Failed to apply io limit to container %s: %v", id, err)
			return
		}
		logrus.Infof("Applied io limit to container %s (disk usage: %d bytes > threshold: %d bytes)",
			id, diskUsage, w.config.MaxDiskBytes)
	}
}
