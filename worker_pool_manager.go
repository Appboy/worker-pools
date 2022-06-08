package pool

import (
	"context"
	"sync"
	"time"

	"github.com/jellydator/ttlcache/v3"
)

// WorkerPoolManager - Self-expiring, lazily constructed map of fixed-size worker pools safe for concurrent use
type WorkerPoolManager struct {
	workerPoolCache     *ttlcache.Cache[string, WorkerPool]
	workerPoolMaxSize   int
	poolReservationLock *sync.Mutex
	stalePoolExpiration time.Duration
	maxPoolLifetime     time.Duration
}

// NewWorkerPoolManager factory constructor
//
// * poolSize - The max number of workers for each key
// * stalePoolExpiration - how long to cache unused pools for
// * maxPoolLifetime - max time to allow pools to live
func NewWorkerPoolManager(
	poolSize int, stalePoolExpiration time.Duration, maxPoolLifetime time.Duration,
) *WorkerPoolManager {
	workerPoolCache := ttlcache.New(
		ttlcache.WithTTL[string, WorkerPool](stalePoolExpiration),
	)
	workerPoolCache.OnEviction(func(context context.Context, reason ttlcache.EvictionReason, item *ttlcache.Item[string, WorkerPool]) {
		item.Value().Dispose()
	})
	go workerPoolCache.Start()

	return &WorkerPoolManager{
		workerPoolCache:     workerPoolCache,
		workerPoolMaxSize:   poolSize,
		poolReservationLock: &sync.Mutex{},
		stalePoolExpiration: stalePoolExpiration,
		maxPoolLifetime:     maxPoolLifetime,
	}
}

// GetPool returns the WorkerPool for this key, building a BaseWorkerPool and caching it if necessary.
// Spawns sendSize workers, up to a max of the manager's poolSize.
//
// This returns the pool in an "unexpirable" state - the caller should signal the returned done channel when it
// no longer requires the returned bundle.
func (m *WorkerPoolManager) GetPool(key string, sendSize int) (WorkerPool, chan<- bool) {
	// The default factory, NewWorkerPool, cannot return an error
	pool, doneUsing, _ := m.GetPoolWithFactory(key, sendSize, NewWorkerPool)
	return pool, doneUsing
}

// GetPoolWithFactory returns the WorkerPool for this key, allowing you to specify a custom pool.Factory
// if you want to build a custom WorkerPool implementation which embeds a BaseWorkerPool and attaches
// supplimentary shared data for the pool.
func (m *WorkerPoolManager) GetPoolWithFactory(
	key string, sendSize int, factory Factory,
) (WorkerPool, chan<- bool, error) {
	var pool WorkerPool
	var err error

	m.poolReservationLock.Lock()

	cachedPoolItem := m.workerPoolCache.Get(key)
	if cachedPoolItem != nil {
		pool = cachedPoolItem.Value()
	} else {
		pool, err = factory(m.workerPoolMaxSize)
		if err != nil {
			m.poolReservationLock.Unlock()
			return nil, nil, err
		}
		m.workerPoolCache.Set(key, pool, ttlcache.DefaultTTL)
	}

	// Prevent this from being deleted until we're done using it - if reserve returns false, it was
	// closed before we obtained control - otherwise we have a read lock and we know it won't be closed
	// until we're done with it
	goodForUse := pool.reserve()
	if !goodForUse {
		m.poolReservationLock.Unlock()
		return m.GetPoolWithFactory(key, sendSize, factory)
	}

	pool.spawnWorkers(sendSize)

	// If the item is older than maxClientBundleExpiration, remove it from the cache and schedule it for disposal.
	// Disposal won't actually occur until the caller has released it
	if pool.age() > m.maxPoolLifetime {
		m.workerPoolCache.Delete(key)
		go pool.Dispose()
	}

	doneUsing := make(chan bool)
	go func() {
		<-doneUsing
		pool.release()
	}()

	m.poolReservationLock.Unlock()
	return pool, doneUsing, nil
}
