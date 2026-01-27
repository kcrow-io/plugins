package pool

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerPool(t *testing.T) {
	pool := New(3)
	pool.Start()
	defer pool.Stop()

	var counter int32
	for i := 0; i < 10; i++ {
		pool.Submit(func(ctx context.Context) {
			atomic.AddInt32(&counter, 1)
		})
	}

	// Wait a bit for tasks to complete
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&counter) != 10 {
		t.Errorf("Expected counter to be 10, got %d", counter)
	}
}

func TestWorkerPoolSubmitWait(t *testing.T) {
	pool := New(2)
	pool.Start()
	defer pool.Stop()

	var counter int32
	pool.SubmitWait(func(ctx context.Context) {
		atomic.AddInt32(&counter, 1)
		time.Sleep(50 * time.Millisecond)
	})

	// Should be completed immediately after SubmitWait returns
	if atomic.LoadInt32(&counter) != 1 {
		t.Errorf("Expected counter to be 1, got %d", counter)
	}
}

func TestWorkerPoolStop(t *testing.T) {
	pool := New(2)
	pool.Start()

	var counter int32
	for i := 0; i < 5; i++ {
		pool.Submit(func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&counter, 1)
		})
	}

	pool.Stop()

	// After stop, submit should fail
	submitted := pool.Submit(func(ctx context.Context) {
		atomic.AddInt32(&counter, 1)
	})

	if submitted {
		t.Error("Expected submit to fail after stop")
	}
}

func TestWorkerPoolSize(t *testing.T) {
	pool := New(5)
	if pool.Size() != 5 {
		t.Errorf("Expected pool size to be 5, got %d", pool.Size())
	}

	pool = New(0) // Should default to 1
	if pool.Size() != 1 {
		t.Errorf("Expected pool size to be 1 (default), got %d", pool.Size())
	}
}

func TestWorkerPoolConcurrency(t *testing.T) {
	pool := New(5)
	pool.Start()
	defer pool.Stop()

	var counter int32
	var maxConcurrent int32
	var currentConcurrent int32

	for i := 0; i < 20; i++ {
		pool.Submit(func(ctx context.Context) {
			current := atomic.AddInt32(&currentConcurrent, 1)

			// Track max concurrent
			for {
				max := atomic.LoadInt32(&maxConcurrent)
				if current <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, current) {
					break
				}
			}

			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&counter, 1)
			atomic.AddInt32(&currentConcurrent, -1)
		})
	}

	// Wait for all tasks to complete
	time.Sleep(200 * time.Millisecond)

	if atomic.LoadInt32(&counter) != 20 {
		t.Errorf("Expected counter to be 20, got %d", counter)
	}

	maxConc := atomic.LoadInt32(&maxConcurrent)
	if maxConc > 5 {
		t.Errorf("Expected max concurrent to be <= 5, got %d", maxConc)
	}
}

func BenchmarkWorkerPool(b *testing.B) {
	pool := New(10)
	pool.Start()
	defer pool.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.SubmitWait(func(ctx context.Context) {
			// Simulate some work
			time.Sleep(1 * time.Microsecond)
		})
	}
}
