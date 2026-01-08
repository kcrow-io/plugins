package containerd

import (
	"context"
	"time"

	"github.com/containerd/containerd/v2/client"
	"github.com/sirupsen/logrus"
)

// Watcher watches container stats and applies io limits
type Watcher struct {
	client *Cntrd
	config *Config
	stopCh chan struct{}
}

type Config struct {
	// Interval is the interval at which to poll container stats
	WatchInterval time.Duration

	// Containerfn is the function to call when iter container
	Containerfn func(context.Context, client.Container)
}

// NewWatcher creates a new stats watcher
func NewWatcher(client *Cntrd, config *Config) *Watcher {
	return &Watcher{
		client: client,
		config: config,
		stopCh: make(chan struct{}),
	}
}

// Start starts the watcher
func (w *Watcher) Start() {
	logrus.Infof("Starting watcher (interval: %s)", w.config.WatchInterval)

	go w.watch()
}

// Stop stops the watcher
func (w *Watcher) Stop() {
	close(w.stopCh)
	logrus.Info("Watcher stopped")
}

// watch is the main watch loop
func (w *Watcher) watch() {
	for {
		select {
		case <-time.After(w.config.WatchInterval):
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
		w.config.Containerfn(ctx, container)
	}
}
