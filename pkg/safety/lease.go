package safety

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultLeaseDurationSeconds is the auto-expiry duration for experiment locks.
// If a process crashes and fails to release the lock, the lease will be
// considered expired after this many seconds (15 minutes).
const DefaultLeaseDurationSeconds = int32(900)

// LeaseExperimentLock implements ExperimentLock using Kubernetes Lease objects
// for distributed safety. Only one experiment per operator can run across
// all instances of odh-chaos in the cluster.
type LeaseExperimentLock struct {
	client    client.Client
	namespace string
	logger    *slog.Logger
}

// NewLeaseExperimentLock creates a distributed lock backed by Kubernetes Lease objects.
func NewLeaseExperimentLock(c client.Client, namespace string) *LeaseExperimentLock {
	return &LeaseExperimentLock{
		client:    c,
		namespace: namespace,
		logger:    slog.Default(),
	}
}

// Acquire attempts to create a Lease for the given operator. If a Lease already
// exists, it checks whether it has expired. Expired leases are reclaimed via
// an in-place Update (using Kubernetes optimistic concurrency / resourceVersion)
// to avoid TOCTOU races. If the lease is still valid, an error is returned
// containing the holding experiment name.
func (l *LeaseExperimentLock) Acquire(ctx context.Context, operator string, experimentName string) error {
	leaseName := fmt.Sprintf("odh-chaos-lock-%s", operator)
	leaseDuration := DefaultLeaseDurationSeconds
	now := metav1.NewMicroTime(time.Now())

	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: l.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "odh-chaos",
			},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &experimentName,
			LeaseDurationSeconds: &leaseDuration,
			AcquireTime:          &now,
		},
	}

	err := l.client.Create(ctx, lease)
	if errors.IsAlreadyExists(err) {
		existing := &coordinationv1.Lease{}
		if getErr := l.client.Get(ctx, client.ObjectKeyFromObject(lease), existing); getErr != nil {
			return fmt.Errorf("checking existing lease: %w", getErr)
		}

		// If the existing lease has expired, reclaim it via Update (optimistic
		// concurrency via resourceVersion). This avoids a TOCTOU race where
		// two callers both delete then create.
		if l.isExpired(existing) {
			holder := ""
			if existing.Spec.HolderIdentity != nil {
				holder = *existing.Spec.HolderIdentity
			}
			l.logger.Info("reclaiming expired lease",
				"lease", leaseName,
				"holder", holder,
				"operator", operator,
			)

			// Update the existing lease in-place. The resourceVersion on
			// the existing object provides optimistic concurrency -- if
			// another caller updates first, this one gets a conflict error.
			reclaimNow := metav1.NewMicroTime(time.Now())
			existing.Spec.HolderIdentity = &experimentName
			existing.Spec.LeaseDurationSeconds = &leaseDuration
			existing.Spec.AcquireTime = &reclaimNow

			return l.client.Update(ctx, existing)
		}

		holder := ""
		if existing.Spec.HolderIdentity != nil {
			holder = *existing.Spec.HolderIdentity
		}
		return fmt.Errorf("operator %s is locked by experiment %q", operator, holder)
	}
	return err
}

// Release deletes the Lease for the given operator, allowing other experiments
// to acquire the lock.
func (l *LeaseExperimentLock) Release(operator string) {
	leaseName := fmt.Sprintf("odh-chaos-lock-%s", operator)
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: l.namespace,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := l.client.Delete(ctx, lease); err != nil {
		l.logger.Error("failed to release lease",
			"lease", leaseName,
			"operator", operator,
			"error", err,
		)
	}
}

// isExpired returns true if the lease's acquire time plus its duration is in
// the past. Leases without an acquire time or duration are considered expired
// to avoid permanent locks from incomplete state.
func (l *LeaseExperimentLock) isExpired(lease *coordinationv1.Lease) bool {
	if lease.Spec.AcquireTime == nil || lease.Spec.LeaseDurationSeconds == nil {
		return true
	}
	expiry := lease.Spec.AcquireTime.Time.Add(
		time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second,
	)
	return time.Now().After(expiry)
}

// Compile-time interface check.
var _ ExperimentLock = (*LeaseExperimentLock)(nil)
