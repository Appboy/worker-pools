package pool

import (
	"sync"
	"time"
)

// Work - a unit of work
type Work func()

// Factory builds a new WorkerPool
type Factory func(maxSize int) (WorkerPool, error)

// WorkerPool is a fixed-size pool of workers.
type WorkerPool interface {
	Submit(w Work)
	Dispose()

	spawnWorkers(sendSize int)
	reserve() bool
	release()
	age() time.Duration
}

// BaseWorkerPool is the base implementation of WorkerPool
type BaseWorkerPool struct {
	workerCount int
	maxSize     int
	sends       chan Work

	// Each active sender takes out a read lock on this, and when we want to destroy this bundle and clean its
	// workers, we take a write lock
	deletionLock *sync.RWMutex

	disposed     chan bool
	creationTime time.Time
}

// NewWorkerPool builds a new BaseWorkerPool and return it as a WorkerPool. This is the default pool factory.
func NewWorkerPool(maxSize int) (WorkerPool, error) {
	return &BaseWorkerPool{
		sends:        make(chan Work, maxSize),
		maxSize:      maxSize,
		deletionLock: &sync.RWMutex{},
		disposed:     make(chan bool),
		workerCount:  0,
		creationTime: time.Now(),
	}, nil
}

func min(x int, y int) int {
	if x < y {
		return x
	}
	return y
}

// Submit an item of Work to be executed.
//
// When all workers are busy, and an additional workerPoolMaxSize of pending work beyond that is also already enqueued,
// this method will block until workers become available.
func (p *BaseWorkerPool) Submit(w Work) {
	p.sends <- w
}

// It's not thread-safe, lock above this
func (p *BaseWorkerPool) spawnWorkers(sendSize int) {
	// Spawn as many new workers as there are messages in this send, up until workerPoolMaxSize total
	// spawned workers. This way, when there are clients that are only ever doing a single unit of work at a time,
	// we only ever spawn a single worker, but when there are clients doing large blasts of work concurrently, we'll
	// spawn workerPoolMaxSize workers.
	newWorkers := min(sendSize, p.maxSize-p.workerCount)
	if newWorkers > 0 {
		p.workerCount += newWorkers
		// Build a fixed-size sender pool for this bundle. Each worker in the sender pool loops indefinitely,
		// processing all the sends for this client, effectively throttling the number of simultaneous sends for a given
		// client.
		for i := 0; i < newWorkers; i++ {
			go func() {
				for {
					select {
					case send := <-p.sends:
						send()
					case <-p.disposed:
						return
					}
				}
			}()
		}
	}
}

func (p *BaseWorkerPool) reserve() bool {
	p.deletionLock.RLock()
	select {
	case <-p.disposed:
		p.deletionLock.RUnlock()
		return false
	default:
		return true
	}
}

func (p *BaseWorkerPool) release() {
	p.deletionLock.RUnlock()
}

func (p *BaseWorkerPool) age() time.Duration {
	return time.Since(p.creationTime)
}

// Dispose the pool, closing down the workers and releasing any shared resources.
func (p *BaseWorkerPool) Dispose() {
	p.deletionLock.Lock()
	defer p.deletionLock.Unlock()

	// On the one hand, I don't expect dispose to be called on a single bundle multiple times.
	// On the other hand, it's so easy and safe to defend against that I might as well.
	select {
	case <-p.disposed:
		return
	default:
		close(p.disposed)
	}
}
