package safety

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExperimentLock(t *testing.T) {
	lock := NewLocalExperimentLock()

	// First lock should succeed
	err := lock.Acquire(context.Background(), "opendatahub-operator", "test-exp-1")
	require.NoError(t, err)

	// Second lock on same operator should fail
	err = lock.Acquire(context.Background(), "opendatahub-operator", "test-exp-2")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "test-exp-1")

	// Release and re-acquire should work
	lock.Release("opendatahub-operator")

	err = lock.Acquire(context.Background(), "opendatahub-operator", "test-exp-2")
	assert.NoError(t, err)

	lock.Release("opendatahub-operator")
}

func TestExperimentLockDifferentOperators(t *testing.T) {
	lock := NewLocalExperimentLock()

	err := lock.Acquire(context.Background(), "operator-a", "exp-1")
	require.NoError(t, err)

	// Different operator should work
	err = lock.Acquire(context.Background(), "operator-b", "exp-2")
	assert.NoError(t, err)

	lock.Release("operator-a")
	lock.Release("operator-b")
}

func TestLocalExperimentLockConcurrentAccess(t *testing.T) {
	lock := NewLocalExperimentLock()

	var wg sync.WaitGroup
	var acquisitions atomic.Int64
	var conflicts atomic.Int64

	// Spawn 20 goroutines that each try to Acquire, sleep briefly, then Release
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			operator := fmt.Sprintf("test-op-%d", id)
			err := lock.Acquire(context.Background(), operator, "test-experiment")
			if err != nil {
				conflicts.Add(1)
				return
			}
			acquisitions.Add(1)
			time.Sleep(1 * time.Millisecond)
			lock.Release(operator)
		}(i)
	}

	wg.Wait()

	t.Logf("acquisitions: %d, conflicts: %d", acquisitions.Load(), conflicts.Load())
	assert.True(t, acquisitions.Load() >= 1, "at least one goroutine should have acquired the lock")
}
