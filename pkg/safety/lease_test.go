package safety

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestLeaseExperimentLockAcquire(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	lock := NewLeaseExperimentLock(client, "opendatahub")
	err := lock.Acquire(context.Background(), "test-operator", "test-experiment")
	assert.NoError(t, err)
}

func TestLeaseExperimentLockConflict(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	lock := NewLeaseExperimentLock(client, "opendatahub")
	err := lock.Acquire(context.Background(), "test-operator", "experiment-1")
	require.NoError(t, err)

	err = lock.Acquire(context.Background(), "test-operator", "experiment-2")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "experiment-1")
}

func TestLeaseExperimentLockRelease(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	lock := NewLeaseExperimentLock(client, "opendatahub")
	err := lock.Acquire(context.Background(), "test-operator", "experiment-1")
	require.NoError(t, err)

	lock.Release("test-operator")

	// Should be able to acquire again after release
	err = lock.Acquire(context.Background(), "test-operator", "experiment-2")
	assert.NoError(t, err)
}

func TestLeaseExperimentLockDifferentOperators(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	lock := NewLeaseExperimentLock(client, "opendatahub")

	err := lock.Acquire(context.Background(), "operator-a", "exp-1")
	require.NoError(t, err)

	// Different operator should work
	err = lock.Acquire(context.Background(), "operator-b", "exp-2")
	assert.NoError(t, err)

	lock.Release("operator-a")
	lock.Release("operator-b")
}
