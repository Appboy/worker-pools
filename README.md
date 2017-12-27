# worker-pools
Go package for managing a set of lazily constructed, self-expiring, concurrency-limited worker pools.  

```
maxConcurrentWorkloads := 500
stalePoolExpiration := 10*time.Minute
maxPoolLifetime := 4*time.Hour
poolManager := pool.NewWorkerPoolManager(
  maxConcurrentWorkloads, stalePoolExpiration, maxPoolLifetime,
)

pool, doneUsing := poolManager.Get("pool 1")
pool.Submit(func() {
  // Do anything here. Only maxConcurrentWorkloads will be allowed to execute concurrently per pool.
  // This is useful for limiting concurrent usage of external resources.
})
close(doneUsing)
```

Each pool instance is constructed when it is required and cached for `stalePoolExpiration` each time it is used, up to a maximum of `maxPoolLifetime` if the pool is receiving constant usage. Multiple goroutines may safely reserve and use pools concurrently. The pool will spin up worker routines lazily as they're required, allowing for large levels of concurrency and a high cardinality of pools in the manager.

If you want to attach shared data or behavior to each pool instance:

```
type myPooledData struct {
	pool.WorkerPool

	// You can put shared data of any type here
	myData string
}
func (p *myPooledData) Dispose() {
	p.WorkerPool.Dispose()
	// Release shared data here.
}

poolManager := pool.NewWorkerPoolManager(500, 10*time.Minute, 4*time.Hour)

var poolFactory pool.Factory = func(maxSize int) (pool.WorkerPool, error) {
	workerPool, _ := pool.NewWorkerPool(maxSize)

	// Build shared resources here, return errors, etc.

	return &myPooledData{
		WorkerPool: workerPool,
		myData: "my shared data"
	}, nil
}

pool, doneUsing, err := s.ClientBundleManager.GetPoolWithFactory("pool 1", sendSize, bundleFactory)
// Any error returned in the Factory function will bubble up here
if err != nil {
  // Handle
}
pool.Submit(func() {
  // Do anything here. Only maxConcurrentWorkloads will be allowed to execute concurrently per pool.
})
close(doneUsing)
```

See [GoDoc](https://godoc.org/github.com/Appboy/worker-pools) for more details.
