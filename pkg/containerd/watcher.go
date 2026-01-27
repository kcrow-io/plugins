package containerd

import (
	"context"
	"sync"
	"time"

	"github.com/containerd/containerd/v2/client"
	"github.com/kcrow-io/plugins/pkg/pool"
	"github.com/sirupsen/logrus"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ContainerSubscriber is a function that processes a container
// It returns true to continue iteration, false to stop for this subscriber
type ContainerSubscriber func(context.Context, client.Container) bool

// CRIContainerSubscriber is a function that processes a CRI container
// It returns true to continue iteration, false to stop for this subscriber
type CRIContainerSubscriber func(context.Context, *runtimeapi.Container) bool

// Watcher watches container stats and applies io limits
type Watcher struct {
	client      *Cntrd
	criClient   *CRIClient
	config      *Config
	stopCh      chan struct{}
	subscribers []subscriberEntry
	mu          sync.RWMutex
	pool        *pool.WorkerPool
}

type subscriberEntry struct {
	id         string
	fn         ContainerSubscriber
	criFn      CRIContainerSubscriber
	useCRI     bool
	shouldStop bool // tracks if this subscriber wants to stop
}

type Config struct {
	// Interval is the interval at which to poll container stats
	WatchInterval time.Duration

	// CRISocket is the path to the CRI socket (optional)
	CRISocket string

	// WorkerPoolSize is the number of workers for parallel processing (default: 4)
	WorkerPoolSize int
}

// NewWatcher creates a new stats watcher
func NewWatcher(client *Cntrd, config *Config) (*Watcher, error) {
	// Set default worker pool size
	if config.WorkerPoolSize <= 0 {
		config.WorkerPoolSize = 4
	}

	w := &Watcher{
		client:      client,
		config:      config,
		stopCh:      make(chan struct{}),
		subscribers: make([]subscriberEntry, 0),
		pool:        pool.New(config.WorkerPoolSize),
	}

	// Initialize CRI client if socket is provided
	if config.CRISocket != "" {
		criClient, err := NewCRIClient(config.CRISocket)
		if err != nil {
			return nil, err
		}
		w.criClient = criClient
	}

	// Start worker pool
	w.pool.Start()

	return w, nil
}

// Subscribe adds a subscriber for containerd containers
// The subscriber function returns true to continue iteration, false to stop
func (w *Watcher) Subscribe(id string, fn ContainerSubscriber) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.subscribers = append(w.subscribers, subscriberEntry{
		id:     id,
		fn:     fn,
		useCRI: false,
	})
}

// SubscribeCRI adds a subscriber for CRI containers
// The subscriber function returns true to continue iteration, false to stop
func (w *Watcher) SubscribeCRI(id string, fn CRIContainerSubscriber) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.subscribers = append(w.subscribers, subscriberEntry{
		id:     id,
		criFn:  fn,
		useCRI: true,
	})
}

// Unsubscribe removes a subscriber by ID
func (w *Watcher) Unsubscribe(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i, sub := range w.subscribers {
		if sub.id == id {
			w.subscribers = append(w.subscribers[:i], w.subscribers[i+1:]...)
			return
		}
	}
}

// Start starts the watcher
func (w *Watcher) Start() {
	logrus.Infof("Starting watcher (interval: %s, workers: %d)", w.config.WatchInterval, w.config.WorkerPoolSize)

	go w.watch()
}

// Stop stops the watcher
func (w *Watcher) Stop() {
	close(w.stopCh)
	if w.pool != nil {
		w.pool.Stop()
	}
	if w.criClient != nil {
		w.criClient.Close() // nolint
	}
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

// hasActiveSubscribers checks if there are any active subscribers
func (w *Watcher) hasActiveSubscribers(subscribers []subscriberEntry) bool {
	for _, sub := range subscribers {
		if !sub.shouldStop {
			return true
		}
	}
	return false
}

// checkContainers checks all containers and notifies subscribers
func (w *Watcher) checkContainers() {
	ctx := context.Background()

	w.mu.RLock()
	subscribers := make([]subscriberEntry, len(w.subscribers))
	copy(subscribers, w.subscribers)
	w.mu.RUnlock()

	// Early exit if no active subscribers
	if !w.hasActiveSubscribers(subscribers) {
		return
	}

	// Separate subscribers by type
	var containerdSubs []int
	var criSubs []int

	for i, sub := range subscribers {
		if sub.shouldStop {
			continue
		}
		if sub.useCRI {
			criSubs = append(criSubs, i)
		} else {
			containerdSubs = append(containerdSubs, i)
		}
	}

	// Process containerd containers
	if len(containerdSubs) > 0 {
		containers, err := w.client.Containers(ctx)
		if err != nil {
			logrus.Errorf("Failed to list containers: %v", err)
		} else {
			w.processContainerdContainers(ctx, containers, subscribers, containerdSubs)
		}
	}

	// Process CRI containers
	if len(criSubs) > 0 && w.criClient != nil {
		containers, err := w.criClient.ListContainers(ctx, nil)
		if err != nil {
			logrus.Errorf("Failed to list CRI containers: %v", err)
		} else {
			w.processCRIContainers(ctx, containers, subscribers, criSubs)
		}
	}
}

// processContainerdContainers processes containerd containers in parallel
func (w *Watcher) processContainerdContainers(ctx context.Context, containers []client.Container, subscribers []subscriberEntry, subIndices []int) {
	var wg sync.WaitGroup

	for _, container := range containers {
		// Check if any subscriber is still active
		hasActive := false
		for _, idx := range subIndices {
			if !subscribers[idx].shouldStop {
				hasActive = true
				break
			}
		}

		// Early exit if all subscribers stopped
		if !hasActive {
			break
		}

		container := container // Capture for goroutine
		wg.Add(1)

		w.pool.Submit(func(poolCtx context.Context) {
			defer wg.Done()

			for _, idx := range subIndices {
				if subscribers[idx].shouldStop {
					continue
				}

				shouldContinue := subscribers[idx].fn(ctx, container)
				if !shouldContinue {
					w.mu.Lock()
					// Find and mark this subscriber as stopped
					for i := range w.subscribers {
						if w.subscribers[i].id == subscribers[idx].id {
							w.subscribers[i].shouldStop = true
							break
						}
					}
					w.mu.Unlock()
				}
			}
		})
	}

	wg.Wait()
}

// processCRIContainers processes CRI containers in parallel
func (w *Watcher) processCRIContainers(ctx context.Context, containers []*runtimeapi.Container, subscribers []subscriberEntry, subIndices []int) {
	var wg sync.WaitGroup

	for _, container := range containers {
		// Check if any subscriber is still active
		hasActive := false
		for _, idx := range subIndices {
			if !subscribers[idx].shouldStop {
				hasActive = true
				break
			}
		}

		// Early exit if all subscribers stopped
		if !hasActive {
			break
		}

		container := container // Capture for goroutine
		wg.Add(1)

		w.pool.Submit(func(poolCtx context.Context) {
			defer wg.Done()

			for _, idx := range subIndices {
				if subscribers[idx].shouldStop {
					continue
				}

				shouldContinue := subscribers[idx].criFn(ctx, container)
				if !shouldContinue {
					w.mu.Lock()
					// Find and mark this subscriber as stopped
					for i := range w.subscribers {
						if w.subscribers[i].id == subscribers[idx].id {
							w.subscribers[i].shouldStop = true
							break
						}
					}
					w.mu.Unlock()
				}
			}
		})
	}

	wg.Wait()
}
