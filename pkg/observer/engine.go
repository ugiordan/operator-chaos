package observer

import (
	"context"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
)

// Observer watches Kubernetes state and checks steady-state conditions.
type Observer interface {
	CheckSteadyState(ctx context.Context, checks []v1alpha1.SteadyStateCheck, namespace string) (*v1alpha1.CheckResult, error)
}
