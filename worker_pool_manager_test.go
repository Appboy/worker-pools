package pool

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/fortytw2/leaktest"
	"github.com/stretchr/testify/assert"
)

func TestGetPoolCannotCreateLargerPoolsThanExpected(t *testing.T) {
	poolSize := 1050
	pm := NewWorkerPoolManager(poolSize, 1*time.Second, 5*time.Second)
	_, doneUsing := pm.GetPool("key", 0)
	close(doneUsing)

	defer leaktest.Check(t)()

	var wg sync.WaitGroup

	n := 23
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			_, doneUsing := pm.GetPool("key", 500)
			time.Sleep(1 * time.Nanosecond)
			close(doneUsing)
			wg.Done()
		}()
	}

	wg.Wait()

	bundle, _ := pm.workerPoolCache.Get("key")
	assert.Equal(t, poolSize, bundle.(*BaseWorkerPool).workerCount)
	bundle.(WorkerPool).Dispose()
}

func TestManagerStoresWorkerPoolInCache(t *testing.T) {
	pm := NewWorkerPoolManager(100, 1*time.Second, 5*time.Second)
	redHerring, _ := NewWorkerPool(1)
	pm.workerPoolCache.Set("red herring key", redHerring)

	pool, doneUsing := pm.GetPool("key", 0)
	assert.NotNil(t, pool)
	close(doneUsing)

	cachedObject, cacheExists := pm.workerPoolCache.Get("key")
	cachedPool, success := cachedObject.(WorkerPool)
	if !success {
		t.Error("Expected GetPool to store the WorkerPool in the cache")
	}

	if reflect.DeepEqual(cachedPool, redHerring) {
		t.Error("Got back the red herring!")
	}

	if !reflect.DeepEqual(cachedPool, pool) || !cacheExists {
		t.Errorf("Expected GetPool to store the pool in the cache, wanted %+v, got %+v", pool, cachedPool)
	}
}

func TestManagerUsesStoredWorkerPoolFromCache(t *testing.T) {
	pm := NewWorkerPoolManager(100, 1*time.Second, 5*time.Second)
	redHerring, _ := NewWorkerPool(1)
	pm.workerPoolCache.Set("red herring key", redHerring)
	actualPool, _ := NewWorkerPool(1)
	pm.workerPoolCache.Set("key", actualPool)

	pool, doneUsing := pm.GetPool("key", 0)
	assert.NotNil(t, pool)

	if reflect.DeepEqual(pool, redHerring) {
		t.Error("Got back the red herring!")
	}

	if !reflect.DeepEqual(pool, actualPool) {
		t.Errorf("Expected GetPool to return the pool from the cache, wanted %+v, got %+v", actualPool, pool)
	}
	close(doneUsing)
}

func TestWorkerPoolExpiry(t *testing.T) {
	staleClientBundleExpiration := 100 * time.Millisecond
	maxClientBundleExpiration := 6 * time.Hour
	pm := NewWorkerPoolManager(100, staleClientBundleExpiration, maxClientBundleExpiration)

	defer leaktest.Check(t)()

	var wg sync.WaitGroup

	doWork := func() {
		pool, doneUsing := pm.GetPool("key", 1)
		pool.Submit(func() {
			wg.Done()
		})
		close(doneUsing)
	}

	wg.Add(1)
	go doWork()

	// Wait around just long enough to ensure that the pool to got made and cached
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, 1, pm.workerPoolCache.Count())

	time.Sleep(staleClientBundleExpiration)
	assert.Equal(t, 0, pm.workerPoolCache.Count())

	wg.Add(1)
	go doWork()
	// Wait around just long enough to ensure that the pool to got made and cached
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, 1, pm.workerPoolCache.Count())
	wg.Add(1)
	go doWork()
	time.Sleep(staleClientBundleExpiration / 2)
	assert.Equal(t, 1, pm.workerPoolCache.Count())
	wg.Add(1)
	go doWork()
	time.Sleep(staleClientBundleExpiration / 2)
	assert.Equal(t, 1, pm.workerPoolCache.Count())
	wg.Add(1)
	go doWork()
	time.Sleep(staleClientBundleExpiration / 2)
	assert.Equal(t, 1, pm.workerPoolCache.Count())

	time.Sleep(staleClientBundleExpiration)
	assert.Equal(t, 0, pm.workerPoolCache.Count())

	wg.Wait()

	// Just to make sure dispose has had time to run before leaktest checks
	time.Sleep(staleClientBundleExpiration)
}

func TestWorkerPoolMaxExpiry(t *testing.T) {
	staleClientBundleExpiration := 50 * time.Millisecond
	maxClientBundleExpiration := 100 * time.Millisecond
	pm := NewWorkerPoolManager(20, staleClientBundleExpiration, maxClientBundleExpiration)

	defer leaktest.Check(t)()

	var doWorkWaitGroup sync.WaitGroup

	doWork := func(i int) {
		var poolWg sync.WaitGroup
		pool, doneUsing := pm.GetPool("key", i)
		for j := 0; j < i; j++ {
			poolWg.Add(1)
			pool.Submit(func() {
				poolWg.Done()
			})
		}
		poolWg.Wait()
		close(doneUsing)
		doWorkWaitGroup.Done()
	}

	doWorkWaitGroup.Add(1)
	go doWork(1)
	// Wait around just long enough to ensure that the pool to got made and cached
	time.Sleep(10 * time.Millisecond)
	bundle, exists := pm.workerPoolCache.Get("key")
	assert.True(t, exists)
	for i := 0; i < 10; i++ {
		doWorkWaitGroup.Add(1)
		go doWork(i + 1)
		time.Sleep(staleClientBundleExpiration / 2)
	}
	doWorkWaitGroup.Wait()

	newBundle, _ := pm.workerPoolCache.Get("key")
	assert.NotEqual(t, bundle, newBundle)
}

type MockWorkerPool struct {
	WorkerPool

	value int
}

func TestManagerCanUseWorkerPoolFromCustomFactory(t *testing.T) {
	pm := NewWorkerPoolManager(100, 10*time.Millisecond, 1*time.Second)
	value := 13
	var factory Factory = func(maxSize int) (WorkerPool, error) {
		basePool, _ := NewWorkerPool(maxSize)
		return &MockWorkerPool{
			WorkerPool: basePool,
			value:      value,
		}, nil
	}
	pool, doneUsing, err := pm.GetPoolWithFactory("key", 1, factory)
	assert.Nil(t, err)
	assert.NotNil(t, doneUsing)
	assert.Equal(t, value, pool.(*MockWorkerPool).value)
	close(doneUsing)
}

func TestManagerReturnsErrorFromCustomFactory(t *testing.T) {
	pm := NewWorkerPoolManager(100, 1*time.Second, 5*time.Second)
	errorMessage := "SOMETHING BROKE O NOEZ"
	var factory Factory = func(maxSize int) (WorkerPool, error) {
		return nil, errors.New(errorMessage)
	}
	pool, doneUsing, err := pm.GetPoolWithFactory("key", 1, factory)
	assert.NotNil(t, err)
	assert.Equal(t, errorMessage, err.Error())
	assert.Nil(t, doneUsing)
	assert.Nil(t, pool)
}

func TestInterleavedUseAndExpiryDoesNotLeak(t *testing.T) {
	staleClientBundleExpiration := 150 * time.Millisecond
	maxClientBundleExpiration := 300 * time.Millisecond

	pm := NewWorkerPoolManager(10, staleClientBundleExpiration, maxClientBundleExpiration)

	var testWg sync.WaitGroup

	defer leaktest.Check(t)()

	// make n large enough that with the sleep, we pass a maxClientBundleExpiration
	n := 35
	for i := 0; i < n; i++ {
		time.Sleep(10 * time.Millisecond)

		testWg.Add(1)
		go func(i int) {
			var poolWg sync.WaitGroup
			pool, doneUsing := pm.GetPool(string(i%3), 1)
			poolWg.Add(1)
			pool.Submit(func() {
				poolWg.Done()
			})
			poolWg.Wait()
			close(doneUsing)
			testWg.Done()
		}(i)
	}
	testWg.Wait()

	time.Sleep(staleClientBundleExpiration + 5*time.Millisecond)
	assert.Equal(t, 0, pm.workerPoolCache.Count())
}
