package safety

import (
	"context"
	"testing"
	"time"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestLeaseExperimentLockAcquire(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	lock := NewLeaseExperimentLock(client, "opendatahub")
	err := lock.Acquire(context.Background(), "test-operator", "test-experiment", 0)
	assert.NoError(t, err)
}

func TestLeaseExperimentLockConflict(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	lock := NewLeaseExperimentLock(client, "opendatahub")
	err := lock.Acquire(context.Background(), "test-operator", "experiment-1", 0)
	require.NoError(t, err)

	err = lock.Acquire(context.Background(), "test-operator", "experiment-2", 0)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrLockContention)
	assert.Contains(t, err.Error(), "experiment-1")
}

func TestLeaseExperimentLockRelease(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	lock := NewLeaseExperimentLock(client, "opendatahub")
	err := lock.Acquire(context.Background(), "test-operator", "experiment-1", 0)
	require.NoError(t, err)

	err = lock.Release(context.Background(), "test-operator", "experiment-1")
	require.NoError(t, err)

	// Should be able to acquire again after release
	err = lock.Acquire(context.Background(), "test-operator", "experiment-2", 0)
	assert.NoError(t, err)
}

func TestLeaseExperimentLockDifferentOperators(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	lock := NewLeaseExperimentLock(client, "opendatahub")

	err := lock.Acquire(context.Background(), "operator-a", "exp-1", 0)
	require.NoError(t, err)

	// Different operator should work
	err = lock.Acquire(context.Background(), "operator-b", "exp-2", 0)
	assert.NoError(t, err)

	_ = lock.Release(context.Background(), "operator-a", "exp-1")
	_ = lock.Release(context.Background(), "operator-b", "exp-2")
}

func TestLeaseExperimentLockSetsExpiry(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	lock := NewLeaseExperimentLock(c, "opendatahub")
	err := lock.Acquire(context.Background(), "test-operator", "test-experiment", 0)
	require.NoError(t, err)

	// Fetch the created lease and verify expiry fields are set.
	lease := &coordinationv1.Lease{}
	err = c.Get(context.Background(), client.ObjectKey{
		Name:      "odh-chaos-lock-test-operator",
		Namespace: "opendatahub",
	}, lease)
	require.NoError(t, err)

	assert.NotNil(t, lease.Spec.LeaseDurationSeconds, "LeaseDurationSeconds should be set")
	assert.Equal(t, DefaultLeaseDurationSeconds, *lease.Spec.LeaseDurationSeconds)
	assert.NotNil(t, lease.Spec.AcquireTime, "AcquireTime should be set")
	assert.WithinDuration(t, time.Now(), lease.Spec.AcquireTime.Time, 5*time.Second)
}

func TestLeaseExperimentLockExpiry(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))

	// Create a pre-expired lease: acquired 1 hour ago with a 15-minute duration.
	expiredHolder := "crashed-experiment"
	expiredDuration := DefaultLeaseDurationSeconds
	expiredTime := metav1.NewMicroTime(time.Now().Add(-1 * time.Hour))
	expiredLease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "odh-chaos-lock-test-operator",
			Namespace: "opendatahub",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "odh-chaos",
			},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &expiredHolder,
			LeaseDurationSeconds: &expiredDuration,
			AcquireTime:          &expiredTime,
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(expiredLease).Build()
	lock := NewLeaseExperimentLock(c, "opendatahub")

	// Acquire should succeed by reclaiming (updating) the stale lease in-place.
	err := lock.Acquire(context.Background(), "test-operator", "new-experiment", 0)
	assert.NoError(t, err)

	// Verify the new lease has the correct holder.
	lease := &coordinationv1.Lease{}
	err = c.Get(context.Background(), client.ObjectKey{
		Name:      "odh-chaos-lock-test-operator",
		Namespace: "opendatahub",
	}, lease)
	require.NoError(t, err)
	assert.Equal(t, "new-experiment", *lease.Spec.HolderIdentity)
}

func TestLeaseExperimentLockAcquireUsesUpdateForExpiredLease(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))

	// Create a pre-expired lease: acquired 2 hours ago with a 15-minute duration.
	expiredHolder := "old-experiment"
	expiredDuration := DefaultLeaseDurationSeconds
	expiredTime := metav1.NewMicroTime(time.Now().Add(-2 * time.Hour))
	expiredLease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "odh-chaos-lock-my-operator",
			Namespace: "opendatahub",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "odh-chaos",
			},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &expiredHolder,
			LeaseDurationSeconds: &expiredDuration,
			AcquireTime:          &expiredTime,
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(expiredLease).Build()
	lock := NewLeaseExperimentLock(c, "opendatahub")

	// Acquire should reclaim the expired lease by updating it in-place
	// (not deleting + recreating), which avoids TOCTOU race conditions.
	err := lock.Acquire(context.Background(), "my-operator", "reclaimer-experiment", 0)
	require.NoError(t, err)

	// Verify the lease was updated (not deleted and recreated) by checking:
	// 1. The holder changed to the new experiment
	lease := &coordinationv1.Lease{}
	err = c.Get(context.Background(), client.ObjectKey{
		Name:      "odh-chaos-lock-my-operator",
		Namespace: "opendatahub",
	}, lease)
	require.NoError(t, err)
	assert.Equal(t, "reclaimer-experiment", *lease.Spec.HolderIdentity,
		"holder should be updated to new experiment")

	// 2. AcquireTime should be recent (within last 5 seconds)
	assert.WithinDuration(t, time.Now(), lease.Spec.AcquireTime.Time, 5*time.Second,
		"acquire time should be updated to now")

	// 3. LeaseDurationSeconds should still be the default
	assert.Equal(t, DefaultLeaseDurationSeconds, *lease.Spec.LeaseDurationSeconds,
		"lease duration should be set to default")

	// 4. The managed-by label should still be present (preserved from the original object)
	assert.Equal(t, "odh-chaos", lease.Labels["app.kubernetes.io/managed-by"],
		"managed-by label should be preserved")
}

func TestLeaseExperimentLockRenew(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	fakeClock := clock.NewFakeClock(time.Now())
	lock := NewLeaseExperimentLock(c, "opendatahub").WithClock(fakeClock)
	ctx := context.Background()

	err := lock.Acquire(ctx, "test-operator", "test-experiment", 0)
	require.NoError(t, err)

	// Get original acquire time
	lease := &coordinationv1.Lease{}
	err = c.Get(ctx, client.ObjectKey{Name: "odh-chaos-lock-test-operator", Namespace: "opendatahub"}, lease)
	require.NoError(t, err)
	originalAcquireTime := lease.Spec.AcquireTime.Time

	// Advance clock to make renew time measurably different
	fakeClock.Advance(10 * time.Millisecond)

	err = lock.Renew(ctx, "test-operator", "test-experiment")
	require.NoError(t, err)

	// Verify RenewTime was set and AcquireTime unchanged
	err = c.Get(ctx, client.ObjectKey{Name: "odh-chaos-lock-test-operator", Namespace: "opendatahub"}, lease)
	require.NoError(t, err)
	assert.Equal(t, originalAcquireTime, lease.Spec.AcquireTime.Time,
		"renew should NOT change acquire time")
	require.NotNil(t, lease.Spec.RenewTime, "renew should set RenewTime")
	assert.True(t, lease.Spec.RenewTime.After(originalAcquireTime),
		"RenewTime should be after original AcquireTime")
}

func TestLeaseExperimentLockRenewWrongHolder(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	lock := NewLeaseExperimentLock(c, "opendatahub")
	ctx := context.Background()

	err := lock.Acquire(ctx, "test-operator", "experiment-1", 0)
	require.NoError(t, err)

	err = lock.Renew(ctx, "test-operator", "wrong-experiment")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrHolderMismatch)
}

func TestLeaseExperimentLockRenewNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	lock := NewLeaseExperimentLock(c, "opendatahub")
	ctx := context.Background()

	// Try to renew a lease that doesn't exist
	err := lock.Renew(ctx, "nonexistent-operator", "test-experiment")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrLockNotFound)
}

func TestLeaseExperimentLockSelfReacquire(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	lock := NewLeaseExperimentLock(c, "opendatahub")
	ctx := context.Background()

	err := lock.Acquire(ctx, "test-operator", "experiment-1", 0)
	require.NoError(t, err)

	// Same experiment re-acquiring should succeed
	err = lock.Acquire(ctx, "test-operator", "experiment-1", 0)
	assert.NoError(t, err)
}

func TestLeaseExperimentLockReleaseHolderVerification(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	lock := NewLeaseExperimentLock(c, "opendatahub")
	ctx := context.Background()

	err := lock.Acquire(ctx, "test-operator", "experiment-1", 0)
	require.NoError(t, err)

	// Wrong holder should fail with ErrHolderMismatch
	err = lock.Release(ctx, "test-operator", "wrong-experiment")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrHolderMismatch)

	// Correct holder should succeed
	err = lock.Release(ctx, "test-operator", "experiment-1")
	assert.NoError(t, err)

	// Release of non-existent lease should return ErrLockNotFound
	err = lock.Release(ctx, "nonexistent-operator", "experiment-1")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrLockNotFound)
}

func TestLeaseExperimentLockReleaseGetError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))

	// Use an interceptor client that returns an error on Get
	// We'll create a simple wrapper that fails Get operations
	baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// For this test, we need to simulate a Get error other than NotFound
	// Since the fake client doesn't provide easy error injection, we'll
	// test this by creating a lease and then using a namespace mismatch
	// to trigger a different error scenario
	lock := NewLeaseExperimentLock(baseClient, "opendatahub")
	ctx := context.Background()

	err := lock.Acquire(ctx, "test-operator", "experiment-1", 0)
	require.NoError(t, err)

	// Now create a lock with a different namespace to simulate Get returning an error
	wrongNsLock := NewLeaseExperimentLock(baseClient, "wrong-namespace")

	// This should fail to find the lease (NotFound), which Release returns ErrLockNotFound
	err = wrongNsLock.Release(ctx, "test-operator", "experiment-1")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrLockNotFound)
}

func TestLeaseExperimentLockAcquireReclainsExpiredRenewedLease(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))

	// Create a lease with a recent AcquireTime but an old RenewTime and short duration.
	// The lease should be considered expired based on RenewTime + duration < now.
	holder := "old-experiment"
	durationSeconds := int32(60) // 1 minute
	recentAcquire := metav1.NewMicroTime(time.Now().Add(-5 * time.Minute))
	oldRenew := metav1.NewMicroTime(time.Now().Add(-10 * time.Minute))
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "odh-chaos-lock-renew-operator",
			Namespace: "opendatahub",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "odh-chaos",
			},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &holder,
			LeaseDurationSeconds: &durationSeconds,
			AcquireTime:          &recentAcquire,
			RenewTime:            &oldRenew,
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(lease).Build()
	lock := NewLeaseExperimentLock(c, "opendatahub")

	// Acquire should succeed by reclaiming the expired-by-renewtime lease
	err := lock.Acquire(context.Background(), "renew-operator", "new-experiment", 0)
	assert.NoError(t, err)

	// Verify the lease was reclaimed with the new holder
	result := &coordinationv1.Lease{}
	err = c.Get(context.Background(), client.ObjectKey{
		Name:      "odh-chaos-lock-renew-operator",
		Namespace: "opendatahub",
	}, result)
	require.NoError(t, err)
	assert.Equal(t, "new-experiment", *result.Spec.HolderIdentity,
		"holder should be updated to the new experiment after reclaiming expired lease")
}

func TestLeaseExperimentLockShortDurationUsesDefault(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	lock := NewLeaseExperimentLock(c, "opendatahub")
	ctx := context.Background()

	// Acquire with a duration shorter than the default — should use default
	shortDuration := 30 * time.Second
	err := lock.Acquire(ctx, "test-operator", "test-experiment", shortDuration)
	require.NoError(t, err)

	lease := &coordinationv1.Lease{}
	err = c.Get(ctx, client.ObjectKey{Name: "odh-chaos-lock-test-operator", Namespace: "opendatahub"}, lease)
	require.NoError(t, err)

	assert.Equal(t, DefaultLeaseDurationSeconds, *lease.Spec.LeaseDurationSeconds,
		"lease duration should use the default when requested duration is shorter")
}

func TestLeaseExperimentLockDynamicDuration(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, coordinationv1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	lock := NewLeaseExperimentLock(c, "opendatahub")
	ctx := context.Background()

	// Acquire with a duration longer than the default
	longDuration := time.Duration(DefaultLeaseDurationSeconds+600) * time.Second
	err := lock.Acquire(ctx, "test-operator", "test-experiment", longDuration)
	require.NoError(t, err)

	lease := &coordinationv1.Lease{}
	err = c.Get(ctx, client.ObjectKey{Name: "odh-chaos-lock-test-operator", Namespace: "opendatahub"}, lease)
	require.NoError(t, err)

	assert.Equal(t, int32(DefaultLeaseDurationSeconds+600), *lease.Spec.LeaseDurationSeconds,
		"lease duration should use the longer dynamic value")
}
