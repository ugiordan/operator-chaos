package safety

import (
	"context"
	"fmt"

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LeaseExperimentLock implements ExperimentLock using Kubernetes Lease objects
// for distributed safety. Only one experiment per operator can run across
// all instances of odh-chaos in the cluster.
type LeaseExperimentLock struct {
	client    client.Client
	namespace string
}

// NewLeaseExperimentLock creates a distributed lock backed by Kubernetes Lease objects.
func NewLeaseExperimentLock(c client.Client, namespace string) *LeaseExperimentLock {
	return &LeaseExperimentLock{client: c, namespace: namespace}
}

// Acquire attempts to create a Lease for the given operator. If a Lease already
// exists, it means another experiment holds the lock and we return an error
// containing the holding experiment name.
func (l *LeaseExperimentLock) Acquire(ctx context.Context, operator string, experimentName string) error {
	leaseName := fmt.Sprintf("odh-chaos-lock-%s", operator)
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: l.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "odh-chaos",
			},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity: &experimentName,
		},
	}

	err := l.client.Create(ctx, lease)
	if errors.IsAlreadyExists(err) {
		existing := &coordinationv1.Lease{}
		if getErr := l.client.Get(ctx, client.ObjectKeyFromObject(lease), existing); getErr != nil {
			return fmt.Errorf("checking existing lease: %w", getErr)
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
	_ = l.client.Delete(context.Background(), lease)
}

// Compile-time interface check.
var _ ExperimentLock = (*LeaseExperimentLock)(nil)
