package observer

import (
	"context"
	"testing"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type collateralMockObserver struct {
	result *v1alpha1.CheckResult
	err    error
}

func (m *collateralMockObserver) CheckSteadyState(
	ctx context.Context,
	checks []v1alpha1.SteadyStateCheck,
	namespace string,
) (*v1alpha1.CheckResult, error) {
	return m.result, m.err
}

func TestCollateralContributor_AllPass(t *testing.T) {
	board := NewObservationBoard()
	obs := &collateralMockObserver{
		result: &v1alpha1.CheckResult{
			Passed:       true,
			ChecksRun:    1,
			ChecksPassed: 1,
			Timestamp:    metav1.Now(),
		},
	}

	dependents := []*model.ResolvedComponent{
		{
			Ref: model.ComponentRef{Operator: "op-a", Component: "comp-1"},
			Component: &model.ComponentModel{
				Name: "comp-1",
				SteadyState: v1alpha1.SteadyStateSpec{
					Checks: []v1alpha1.SteadyStateCheck{
						{Type: v1alpha1.CheckConditionTrue, Name: "comp-1"},
					},
				},
			},
			Namespace: "ns-a",
		},
	}

	contrib := NewCollateralContributor(obs, dependents)
	err := contrib.Observe(context.Background(), board)
	require.NoError(t, err)

	findings := board.FindingsBySource(SourceCollateral)
	require.Len(t, findings, 1)
	assert.Equal(t, SourceCollateral, findings[0].Source)
	assert.Equal(t, "comp-1", findings[0].Component)
	assert.Equal(t, "op-a", findings[0].Operator)
	assert.True(t, findings[0].Passed)
	assert.NotNil(t, findings[0].Checks)
}

func TestCollateralContributor_OneFails(t *testing.T) {
	board := NewObservationBoard()
	obs := &collateralMockObserver{
		result: &v1alpha1.CheckResult{
			Passed:       false,
			ChecksRun:    1,
			ChecksPassed: 0,
			Timestamp:    metav1.Now(),
		},
	}

	dependents := []*model.ResolvedComponent{
		{
			Ref: model.ComponentRef{Operator: "op-b", Component: "comp-2"},
			Component: &model.ComponentModel{
				Name: "comp-2",
				SteadyState: v1alpha1.SteadyStateSpec{
					Checks: []v1alpha1.SteadyStateCheck{
						{Type: v1alpha1.CheckResourceExists, Name: "comp-2"},
					},
				},
			},
			Namespace: "ns-b",
		},
	}

	contrib := NewCollateralContributor(obs, dependents)
	err := contrib.Observe(context.Background(), board)
	require.NoError(t, err)

	findings := board.FindingsBySource(SourceCollateral)
	require.Len(t, findings, 1)
	assert.False(t, findings[0].Passed)
}

func TestCollateralContributor_SkipsNoChecks(t *testing.T) {
	board := NewObservationBoard()
	obs := &collateralMockObserver{
		result: &v1alpha1.CheckResult{
			Passed:    true,
			Timestamp: metav1.Now(),
		},
	}

	dependents := []*model.ResolvedComponent{
		{
			Ref: model.ComponentRef{Operator: "op-c", Component: "comp-3"},
			Component: &model.ComponentModel{
				Name: "comp-3",
				SteadyState: v1alpha1.SteadyStateSpec{
					Checks: []v1alpha1.SteadyStateCheck{}, // Empty checks
				},
			},
			Namespace: "ns-c",
		},
	}

	contrib := NewCollateralContributor(obs, dependents)
	err := contrib.Observe(context.Background(), board)
	require.NoError(t, err)

	findings := board.FindingsBySource(SourceCollateral)
	require.Len(t, findings, 0) // No findings should be written
}

func TestCollateralContributor_MultipleDependents(t *testing.T) {
	board := NewObservationBoard()
	obs := &collateralMockObserver{
		result: &v1alpha1.CheckResult{
			Passed:       true,
			ChecksRun:    1,
			ChecksPassed: 1,
			Timestamp:    metav1.Now(),
		},
	}

	dependents := []*model.ResolvedComponent{
		{
			Ref: model.ComponentRef{Operator: "op-d", Component: "comp-4"},
			Component: &model.ComponentModel{
				Name: "comp-4",
				SteadyState: v1alpha1.SteadyStateSpec{
					Checks: []v1alpha1.SteadyStateCheck{
						{Type: v1alpha1.CheckConditionTrue, Name: "comp-4"},
					},
				},
			},
			Namespace: "ns-d",
		},
		{
			Ref: model.ComponentRef{Operator: "op-e", Component: "comp-5"},
			Component: &model.ComponentModel{
				Name: "comp-5",
				SteadyState: v1alpha1.SteadyStateSpec{
					Checks: []v1alpha1.SteadyStateCheck{
						{Type: v1alpha1.CheckResourceExists, Name: "comp-5"},
					},
				},
			},
			Namespace: "ns-e",
		},
	}

	contrib := NewCollateralContributor(obs, dependents)
	err := contrib.Observe(context.Background(), board)
	require.NoError(t, err)

	findings := board.FindingsBySource(SourceCollateral)
	require.Len(t, findings, 2)

	// Verify both findings are present and correct
	assert.Equal(t, SourceCollateral, findings[0].Source)
	assert.Equal(t, "comp-4", findings[0].Component)
	assert.Equal(t, "op-d", findings[0].Operator)
	assert.True(t, findings[0].Passed)

	assert.Equal(t, SourceCollateral, findings[1].Source)
	assert.Equal(t, "comp-5", findings[1].Component)
	assert.Equal(t, "op-e", findings[1].Operator)
	assert.True(t, findings[1].Passed)
}
