package safety

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/clock"
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
	clock     clock.Clock
}

// NewLeaseExperimentLock creates a distributed lock backed by Kubernetes Lease objects.
func NewLeaseExperimentLock(c client.Client, namespace string) *LeaseExperimentLock {
	return &LeaseExperimentLock{
		client:    c,
		namespace: namespace,
		logger:    slog.Default(),
		clock:     clock.RealClock{},
	}
}

// WithClock returns a copy of the lock using the given clock (for testing).
func (l *LeaseExperimentLock) WithClock(c clock.Clock) *LeaseExperimentLock {
	copy := *l
	copy.clock = c
	return &copy
}

// leaseName returns the Kubernetes Lease name for the given operator.
func leaseName(operator string) string {
	return fmt.Sprintf("odh-chaos-lock-%s", operator)
}

// holderOf extracts the holder identity from a lease, returning empty string if not set.
func holderOf(lease *coordinationv1.Lease) string {
	if lease.Spec.HolderIdentity == nil {
		return ""
	}
	return *lease.Spec.HolderIdentity
}

// Acquire attempts to create a Lease for the given operator. If a Lease already
// exists, it checks whether it has expired. Expired leases are reclaimed via
// an in-place Update (using Kubernetes optimistic concurrency / resourceVersion)
// to avoid TOCTOU races. If the lease is still valid, an error is returned
// containing the holding experiment name.
func (l *LeaseExperimentLock) Acquire(ctx context.Context, operator string, experimentName string, leaseDuration time.Duration) error {
	name := leaseName(operator)
	leaseDurationSeconds := DefaultLeaseDurationSeconds
	if dynamicSeconds := int32(leaseDuration.Seconds()); dynamicSeconds > leaseDurationSeconds {
		leaseDurationSeconds = dynamicSeconds
	}
	now := metav1.NewMicroTime(l.clock.Now())

	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: l.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "odh-chaos",
			},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &experimentName,
			LeaseDurationSeconds: &leaseDurationSeconds,
			AcquireTime:          &now,
		},
	}

	err := l.client.Create(ctx, lease)
	if errors.IsAlreadyExists(err) {
		existing := &coordinationv1.Lease{}
		if getErr := l.client.Get(ctx, client.ObjectKeyFromObject(lease), existing); getErr != nil {
			return fmt.Errorf("checking existing lease: %w", getErr)
		}

		// Self-re-acquire: if same experiment already holds the lock, allow it.
		if existing.Spec.HolderIdentity != nil && *existing.Spec.HolderIdentity == experimentName {
			return nil
		}

		// If the existing lease has expired, reclaim it via Update (optimistic
		// concurrency via resourceVersion). This avoids a TOCTOU race where
		// two callers both delete then create.
		if l.isExpired(existing) {
			l.logger.Info("reclaiming expired lease",
				"lease", name,
				"holder", holderOf(existing),
				"operator", operator,
			)

			// Update the existing lease in-place. The resourceVersion on
			// the existing object provides optimistic concurrency -- if
			// another caller updates first, this one gets a conflict error.
			reclaimNow := metav1.NewMicroTime(l.clock.Now())
			existing.Spec.HolderIdentity = &experimentName
			existing.Spec.LeaseDurationSeconds = &leaseDurationSeconds
			existing.Spec.AcquireTime = &reclaimNow
			existing.Spec.RenewTime = nil

			return l.client.Update(ctx, existing)
		}

		return fmt.Errorf("operator %s is locked by experiment %q: %w", operator, holderOf(existing), ErrLockContention)
	}
	return err
}

// Release deletes the Lease for the given operator after verifying that the
// holder matches experimentName. Returns an error wrapping ErrLockNotFound if
// the lease does not exist, ErrHolderMismatch if held by a different experiment,
// or a raw error on deletion failure.
func (l *LeaseExperimentLock) Release(ctx context.Context, operator string, experimentName string) error {
	name := leaseName(operator)
	existing := &coordinationv1.Lease{}
	if err := l.client.Get(ctx, client.ObjectKey{Name: name, Namespace: l.namespace}, existing); err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("lease %s: %w", name, ErrLockNotFound)
		}
		return fmt.Errorf("getting lease for release: %w", err)
	}

	holder := holderOf(existing)
	if holder != experimentName {
		return fmt.Errorf("lock held by %q, release requested by %q: %w", holder, experimentName, ErrHolderMismatch)
	}

	return l.client.Delete(ctx, existing, &client.DeleteOptions{
		Preconditions: &metav1.Preconditions{
			ResourceVersion: &existing.ResourceVersion,
		},
	})
}

// Renew updates the RenewTime of an existing lease to extend its validity.
// AcquireTime is left unchanged (it records the original acquisition).
// Returns an error if the lease is not found or the holder does not match.
func (l *LeaseExperimentLock) Renew(ctx context.Context, operator string, experimentName string) error {
	name := leaseName(operator)
	existing := &coordinationv1.Lease{}
	if err := l.client.Get(ctx, client.ObjectKey{Name: name, Namespace: l.namespace}, existing); err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("lease %s: %w", name, ErrLockNotFound)
		}
		return fmt.Errorf("getting lease for renewal: %w", err)
	}

	holder := holderOf(existing)
	if holder != experimentName {
		return fmt.Errorf("lock held by %q, renew requested by %q: %w", holder, experimentName, ErrHolderMismatch)
	}

	now := metav1.NewMicroTime(l.clock.Now())
	existing.Spec.RenewTime = &now

	return l.client.Update(ctx, existing)
}

// isExpired returns true if the lease's last active time plus its duration is in
// the past. RenewTime is preferred when available; otherwise AcquireTime is used.
// Leases without both times or duration are considered expired to avoid permanent
// locks from incomplete state.
func (l *LeaseExperimentLock) isExpired(lease *coordinationv1.Lease) bool {
	if lease.Spec.LeaseDurationSeconds == nil {
		return true
	}
	var lastActive time.Time
	if lease.Spec.RenewTime != nil {
		lastActive = lease.Spec.RenewTime.Time
	} else if lease.Spec.AcquireTime != nil {
		lastActive = lease.Spec.AcquireTime.Time
	} else {
		return true
	}
	expiry := lastActive.Add(time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second)
	return l.clock.Now().After(expiry)
}

// Compile-time interface check.
var _ ExperimentLock = (*LeaseExperimentLock)(nil)
