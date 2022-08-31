package pool

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestSpawnWorkersCannotCreateLargerPoolsThanExpected(t *testing.T) {
	poolSize := 1050
	p, _ := NewWorkerPool(poolSize)

	n := 23
	for i := 0; i < n; i++ {
		p.spawnWorkers((poolSize / n) + 1)
	}

	assert.Equal(t, poolSize, p.(*BaseWorkerPool).workerCount)
	p.Dispose()
}

func TestDisposeClosesWorkerPool(t *testing.T) {
	defer goleak.VerifyNone(t)
	p, _ := NewWorkerPool(1050)

	for i := 0; i < 10; i++ {
		p.spawnWorkers(500)
	}
	p.Dispose()
}
