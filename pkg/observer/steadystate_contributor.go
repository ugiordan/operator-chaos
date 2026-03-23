package observer

import (
	"context"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SteadyStateContributor struct {
	observer  Observer
	checks    []v1alpha1.SteadyStateCheck
	namespace string
}

func NewSteadyStateContributor(obs Observer, checks []v1alpha1.SteadyStateCheck, namespace string) *SteadyStateContributor {
	return &SteadyStateContributor{
		observer:  obs,
		checks:    checks,
		namespace: namespace,
	}
}

func (c *SteadyStateContributor) Observe(ctx context.Context, board *ObservationBoard) error {
	result, err := c.observer.CheckSteadyState(ctx, c.checks, c.namespace)
	if err != nil {
		board.AddFinding(Finding{
			Source:  SourceSteadyState,
			Passed: false,
			Details: err.Error(),
			Checks: &v1alpha1.CheckResult{Passed: false, Timestamp: metav1.Now()},
		})
		return err
	}

	board.AddFinding(Finding{
		Source: SourceSteadyState,
		Passed: result.Passed,
		Checks: result,
	})
	return nil
}
