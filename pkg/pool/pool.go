package pool

import (
	"context"
	"sync"
	"sync/atomic"
)

// Task represents a unit of work to be executed
type Task func(context.Context)

// WorkerPool manages a pool of worker goroutines
type WorkerPool struct {
	workers   int
	taskQueue chan Task
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	stopped   atomic.Bool
}

// New creates a new worker pool with the specified number of workers
func New(workers int) *WorkerPool {
	if workers <= 0 {
		workers = 1
	}

	ctx, cancel := context.WithCancel(context.Background())

	pool := &WorkerPool{
		workers:   workers,
		taskQueue: make(chan Task, workers*2), // Buffer size = 2x workers
		ctx:       ctx,
		cancel:    cancel,
	}

	return pool
}

// Start starts the worker pool
func (p *WorkerPool) Start() {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

// worker is the worker goroutine that processes tasks
func (p *WorkerPool) worker() {
	defer p.wg.Done()

	for {
		select {
		case task, ok := <-p.taskQueue:
			if !ok {
				return
			}
			task(p.ctx)
		case <-p.ctx.Done():
			return
		}
	}
}

// Submit submits a task to the pool
// Returns false if the pool is closed or context is cancelled
func (p *WorkerPool) Submit(task Task) bool {
	if p.stopped.Load() {
		return false
	}

	select {
	case p.taskQueue <- task:
		return true
	case <-p.ctx.Done():
		return false
	}
}

// SubmitWait submits a task and waits for it to complete
func (p *WorkerPool) SubmitWait(task Task) {
	done := make(chan struct{})
	wrappedTask := func(ctx context.Context) {
		defer close(done)
		task(ctx)
	}

	if p.Submit(wrappedTask) {
		<-done
	}
}

// Stop stops the worker pool and waits for all workers to finish
func (p *WorkerPool) Stop() {
	if !p.stopped.CompareAndSwap(false, true) {
		return // Already stopped
	}

	p.cancel() // Cancel context first to prevent new submissions
	close(p.taskQueue)
	p.wg.Wait()
}

// StopGracefully stops accepting new tasks and waits for existing tasks to complete
func (p *WorkerPool) StopGracefully() {
	if !p.stopped.CompareAndSwap(false, true) {
		return // Already stopped
	}

	p.cancel() // Cancel context first
	close(p.taskQueue)
	p.wg.Wait()
}

// Size returns the number of workers in the pool
func (p *WorkerPool) Size() int {
	return p.workers
}

// QueueLength returns the current number of tasks in the queue
func (p *WorkerPool) QueueLength() int {
	return len(p.taskQueue)
}
