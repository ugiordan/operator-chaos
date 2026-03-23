package observer

import (
	"context"
	"testing"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mockObserver struct {
	result *v1alpha1.CheckResult
	err    error
}

func (m *mockObserver) CheckSteadyState(ctx context.Context, checks []v1alpha1.SteadyStateCheck, namespace string) (*v1alpha1.CheckResult, error) {
	return m.result, m.err
}

func TestSteadyStateContributor_Passed(t *testing.T) {
	board := NewObservationBoard()
	obs := &mockObserver{
		result: &v1alpha1.CheckResult{
			Passed: true, ChecksRun: 2, ChecksPassed: 2, Timestamp: metav1.Now(),
		},
	}
	checks := []v1alpha1.SteadyStateCheck{
		{Type: v1alpha1.CheckConditionTrue, Name: "dep1"},
		{Type: v1alpha1.CheckConditionTrue, Name: "dep2"},
	}

	contrib := NewSteadyStateContributor(obs, checks, "test-ns")
	err := contrib.Observe(context.Background(), board)
	require.NoError(t, err)

	findings := board.FindingsBySource(SourceSteadyState)
	require.Len(t, findings, 1)
	assert.True(t, findings[0].Passed)
	assert.NotNil(t, findings[0].Checks)
}

func TestSteadyStateContributor_Failed(t *testing.T) {
	board := NewObservationBoard()
	obs := &mockObserver{
		result: &v1alpha1.CheckResult{Passed: false, ChecksRun: 1, ChecksPassed: 0, Timestamp: metav1.Now()},
	}

	contrib := NewSteadyStateContributor(obs, []v1alpha1.SteadyStateCheck{{Type: v1alpha1.CheckConditionTrue}}, "ns")
	err := contrib.Observe(context.Background(), board)
	require.NoError(t, err)

	findings := board.FindingsBySource(SourceSteadyState)
	require.Len(t, findings, 1)
	assert.False(t, findings[0].Passed)
}
