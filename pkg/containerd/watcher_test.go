package containerd

import (
	"context"
	"testing"
	"time"

	"github.com/containerd/containerd/v2/client"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func TestWatcherSubscribe(t *testing.T) {
	// This is a unit test to verify the subscribe pattern works correctly
	// Note: This test doesn't require actual containerd/CRI connections

	config := &Config{
		WatchInterval: 1 * time.Second,
	}

	// Create a mock Cntrd (we won't actually use it in this test)
	mockCntrd := &Cntrd{}

	watcher, err := NewWatcher(mockCntrd, config)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	// Test Subscribe
	watcher.Subscribe("test1", func(ctx context.Context, c client.Container) bool {
		return true // continue
	})

	if len(watcher.subscribers) != 1 {
		t.Errorf("Expected 1 subscriber, got %d", len(watcher.subscribers))
	}

	// Test Unsubscribe
	watcher.Unsubscribe("test1")
	if len(watcher.subscribers) != 0 {
		t.Errorf("Expected 0 subscribers after unsubscribe, got %d", len(watcher.subscribers))
	}
}

func TestWatcherSubscribeCRI(t *testing.T) {
	config := &Config{
		WatchInterval: 1 * time.Second,
	}

	mockCntrd := &Cntrd{}

	watcher, err := NewWatcher(mockCntrd, config)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	// Test SubscribeCRI
	watcher.SubscribeCRI("test-cri", func(ctx context.Context, c *runtimeapi.Container) bool {
		return true // continue
	})

	if len(watcher.subscribers) != 1 {
		t.Errorf("Expected 1 subscriber, got %d", len(watcher.subscribers))
	}

	if !watcher.subscribers[0].useCRI {
		t.Error("Expected subscriber to be marked as CRI subscriber")
	}
}

func TestMultipleSubscribers(t *testing.T) {
	config := &Config{
		WatchInterval: 1 * time.Second,
	}

	mockCntrd := &Cntrd{}

	watcher, err := NewWatcher(mockCntrd, config)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	// Add multiple subscribers
	watcher.Subscribe("sub1", func(ctx context.Context, c client.Container) bool {
		return true
	})

	watcher.Subscribe("sub2", func(ctx context.Context, c client.Container) bool {
		return false // this subscriber wants to stop
	})

	watcher.SubscribeCRI("sub3", func(ctx context.Context, c *runtimeapi.Container) bool {
		return true
	})

	if len(watcher.subscribers) != 3 {
		t.Errorf("Expected 3 subscribers, got %d", len(watcher.subscribers))
	}

	// Verify we have both types
	containerdCount := 0
	criCount := 0
	for _, sub := range watcher.subscribers {
		if sub.useCRI {
			criCount++
		} else {
			containerdCount++
		}
	}

	if containerdCount != 2 {
		t.Errorf("Expected 2 containerd subscribers, got %d", containerdCount)
	}

	if criCount != 1 {
		t.Errorf("Expected 1 CRI subscriber, got %d", criCount)
	}
}
