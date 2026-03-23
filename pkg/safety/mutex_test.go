package safety

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalExperimentLockAcquireAndRelease(t *testing.T) {
	lock := NewLocalExperimentLock()

	// First lock should succeed
	err := lock.Acquire(context.Background(), "opendatahub-operator", "test-exp-1", 0)
	require.NoError(t, err)

	// Second lock on same operator should fail
	err = lock.Acquire(context.Background(), "opendatahub-operator", "test-exp-2", 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "test-exp-1")

	// Release and re-acquire should work
	_ = lock.Release(context.Background(), "opendatahub-operator", "test-exp-1")

	err = lock.Acquire(context.Background(), "opendatahub-operator", "test-exp-2", 0)
	assert.NoError(t, err)

	_ = lock.Release(context.Background(), "opendatahub-operator", "test-exp-2")
}

func TestExperimentLockDifferentOperators(t *testing.T) {
	lock := NewLocalExperimentLock()

	err := lock.Acquire(context.Background(), "operator-a", "exp-1", 0)
	require.NoError(t, err)

	// Different operator should work
	err = lock.Acquire(context.Background(), "operator-b", "exp-2", 0)
	assert.NoError(t, err)

	_ = lock.Release(context.Background(), "operator-a", "exp-1")
	_ = lock.Release(context.Background(), "operator-b", "exp-2")
}

func TestLocalExperimentLockRenewHolderMismatch(t *testing.T) {
	lock := NewLocalExperimentLock()

	err := lock.Acquire(context.Background(), "operator-a", "exp-1", 0)
	require.NoError(t, err)

	// Renew with wrong experiment name should fail.
	err = lock.Renew(context.Background(), "operator-a", "exp-2")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "holder mismatch")

	// Renew with correct experiment name should succeed.
	err = lock.Renew(context.Background(), "operator-a", "exp-1")
	assert.NoError(t, err)

	_ = lock.Release(context.Background(), "operator-a", "exp-1")
}

func TestLocalExperimentLockRenewNoLockHeld(t *testing.T) {
	lock := NewLocalExperimentLock()

	// Try to renew a lock that doesn't exist
	err := lock.Renew(context.Background(), "nonexistent-operator", "exp-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no lock held for operator")
}

func TestLocalExperimentLockReleaseHolderMismatch(t *testing.T) {
	lock := NewLocalExperimentLock()

	err := lock.Acquire(context.Background(), "operator-a", "exp-1", 0)
	require.NoError(t, err)

	// Release with wrong experiment name should fail.
	err = lock.Release(context.Background(), "operator-a", "exp-2")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "holder mismatch")

	// Lock should still be held by exp-1.
	err = lock.Acquire(context.Background(), "operator-a", "exp-3", 0)
	assert.Error(t, err)

	_ = lock.Release(context.Background(), "operator-a", "exp-1")
}

func TestLocalExperimentLockSelfReacquire(t *testing.T) {
	lock := NewLocalExperimentLock()

	err := lock.Acquire(context.Background(), "operator-a", "exp-1", 0)
	require.NoError(t, err)

	// Same experiment re-acquiring should succeed.
	err = lock.Acquire(context.Background(), "operator-a", "exp-1", 0)
	assert.NoError(t, err)

	_ = lock.Release(context.Background(), "operator-a", "exp-1")
}

func TestLocalExperimentLockReleaseNonExistentOperator(t *testing.T) {
	lock := NewLocalExperimentLock()

	// Release for non-existent operator should return ErrLockNotFound
	err := lock.Release(context.Background(), "nonexistent-operator", "exp-1")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrLockNotFound)
	assert.Contains(t, err.Error(), "nonexistent-operator")
}

func TestLocalExperimentLockConcurrentAccess(t *testing.T) {
	lock := NewLocalExperimentLock()

	var wg sync.WaitGroup
	var acquisitions atomic.Int64
	var conflicts atomic.Int64

	// Spawn 20 goroutines contending for the SAME operator.
	// The winner keeps the lock held (no release) so assertions about
	// exactly 1 winner are deterministic.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			expName := fmt.Sprintf("exp-%d", id)
			err := lock.Acquire(context.Background(), "shared-operator", expName, 0)
			if err != nil {
				conflicts.Add(1)
				return
			}
			acquisitions.Add(1)
		}(i)
	}

	wg.Wait()

	t.Logf("acquisitions: %d, conflicts: %d", acquisitions.Load(), conflicts.Load())
	// Exactly one goroutine should win the lock; the rest should conflict.
	assert.Equal(t, int64(1), acquisitions.Load(), "exactly one goroutine should acquire the lock")
	assert.Equal(t, int64(19), conflicts.Load(), "remaining goroutines should get conflicts")
}
