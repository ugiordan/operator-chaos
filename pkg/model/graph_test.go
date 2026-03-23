package model

import (
	"testing"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildDependencyGraph_IntraOperator tests dependency resolution within a single operator.
// When a component declares a dependency on another component in the same operator,
// DirectDependents should return the declaring component.
func TestBuildDependencyGraph_IntraOperator(t *testing.T) {
	models := []*OperatorKnowledge{
		{
			Operator: OperatorMeta{
				Name:      "alpha-operator",
				Namespace: "test-ns",
			},
			Components: []ComponentModel{
				{
					Name:       "primary",
					Controller: "primary-controller",
					SteadyState: v1alpha1.SteadyStateSpec{
						Checks: []v1alpha1.SteadyStateCheck{
							{Type: v1alpha1.CheckResourceExists, Kind: "Deployment", Name: "primary-deploy"},
						},
					},
				},
				{
					Name:         "secondary",
					Controller:   "secondary-controller",
					Dependencies: []string{"primary"},
					SteadyState: v1alpha1.SteadyStateSpec{
						Checks: []v1alpha1.SteadyStateCheck{
							{Type: v1alpha1.CheckConditionTrue, Kind: "CustomResource", Name: "secondary-cr"},
						},
					},
				},
			},
		},
	}

	graph, err := BuildDependencyGraph(models)
	require.NoError(t, err)
	require.NotNil(t, graph)

	primaryRef := ComponentRef{Operator: "alpha-operator", Component: "primary"}
	dependents := graph.DirectDependents(primaryRef)

	require.Len(t, dependents, 1)
	assert.Equal(t, "alpha-operator", dependents[0].Ref.Operator)
	assert.Equal(t, "secondary", dependents[0].Ref.Component)
	assert.Equal(t, "test-ns", dependents[0].Namespace)
	assert.Equal(t, "secondary-controller", dependents[0].Component.Controller)
}

// TestBuildDependencyGraph_CrossOperator tests dependency resolution across operators.
// When a component declares a dependency on an operator name, all components in that
// operator become targets, and DirectDependents should return the declaring component.
func TestBuildDependencyGraph_CrossOperator(t *testing.T) {
	models := []*OperatorKnowledge{
		{
			Operator: OperatorMeta{
				Name:      "alpha-operator",
				Namespace: "alpha-ns",
			},
			Components: []ComponentModel{
				{
					Name:       "alpha-primary",
					Controller: "alpha-controller",
					SteadyState: v1alpha1.SteadyStateSpec{
						Checks: []v1alpha1.SteadyStateCheck{
							{Type: v1alpha1.CheckResourceExists, Kind: "Deployment", Name: "alpha-deploy"},
						},
					},
				},
			},
		},
		{
			Operator: OperatorMeta{
				Name:      "beta-operator",
				Namespace: "beta-ns",
			},
			Components: []ComponentModel{
				{
					Name:         "beta-main",
					Controller:   "beta-controller",
					Dependencies: []string{"alpha-operator"},
					SteadyState: v1alpha1.SteadyStateSpec{
						Checks: []v1alpha1.SteadyStateCheck{
							{Type: v1alpha1.CheckConditionTrue, Kind: "Service", Name: "beta-svc"},
						},
					},
				},
			},
		},
	}

	graph, err := BuildDependencyGraph(models)
	require.NoError(t, err)
	require.NotNil(t, graph)

	alphaPrimaryRef := ComponentRef{Operator: "alpha-operator", Component: "alpha-primary"}
	dependents := graph.DirectDependents(alphaPrimaryRef)

	require.Len(t, dependents, 1)
	assert.Equal(t, "beta-operator", dependents[0].Ref.Operator)
	assert.Equal(t, "beta-main", dependents[0].Ref.Component)
	assert.Equal(t, "beta-ns", dependents[0].Namespace)
}

// TestBuildDependencyGraph_EmptyDeps tests that components with no dependencies
// have no edges pointing to them from their own declaration.
func TestBuildDependencyGraph_EmptyDeps(t *testing.T) {
	models := []*OperatorKnowledge{
		{
			Operator: OperatorMeta{
				Name:      "standalone-operator",
				Namespace: "standalone-ns",
			},
			Components: []ComponentModel{
				{
					Name:       "standalone-comp",
					Controller: "standalone-controller",
					SteadyState: v1alpha1.SteadyStateSpec{
						Checks: []v1alpha1.SteadyStateCheck{
							{Type: v1alpha1.CheckResourceExists, Kind: "Pod", Name: "standalone-pod"},
						},
					},
				},
			},
		},
	}

	graph, err := BuildDependencyGraph(models)
	require.NoError(t, err)
	require.NotNil(t, graph)

	standaloneRef := ComponentRef{Operator: "standalone-operator", Component: "standalone-comp"}
	dependents := graph.DirectDependents(standaloneRef)

	assert.Nil(t, dependents)
}

// TestBuildDependencyGraph_UnresolvableDep tests that unresolvable dependencies
// are logged but do not cause errors or create edges.
func TestBuildDependencyGraph_UnresolvableDep(t *testing.T) {
	models := []*OperatorKnowledge{
		{
			Operator: OperatorMeta{
				Name:      "gamma-operator",
				Namespace: "gamma-ns",
			},
			Components: []ComponentModel{
				{
					Name:         "gamma-comp",
					Controller:   "gamma-controller",
					Dependencies: []string{"nonexistent"},
					SteadyState: v1alpha1.SteadyStateSpec{
						Checks: []v1alpha1.SteadyStateCheck{
							{Type: v1alpha1.CheckResourceExists, Kind: "ConfigMap", Name: "gamma-cm"},
						},
					},
				},
			},
		},
	}

	graph, err := BuildDependencyGraph(models)
	require.NoError(t, err)
	require.NotNil(t, graph)

	// The unresolvable dependency should not create any edges
	gammaRef := ComponentRef{Operator: "gamma-operator", Component: "gamma-comp"}
	dependents := graph.DirectDependents(gammaRef)
	assert.Nil(t, dependents)

	// Query a non-existent reference - should also return nil
	nonExistentRef := ComponentRef{Operator: "nonexistent", Component: "nonexistent"}
	dependents = graph.DirectDependents(nonExistentRef)
	assert.Nil(t, dependents)
}

// TestBuildDependencyGraph_DuplicateDeps tests that duplicate dependencies
// in the same component's dependency list only create a single edge.
func TestBuildDependencyGraph_DuplicateDeps(t *testing.T) {
	models := []*OperatorKnowledge{
		{
			Operator: OperatorMeta{
				Name:      "delta-operator",
				Namespace: "delta-ns",
			},
			Components: []ComponentModel{
				{
					Name:       "primary",
					Controller: "primary-controller",
					SteadyState: v1alpha1.SteadyStateSpec{
						Checks: []v1alpha1.SteadyStateCheck{
							{Type: v1alpha1.CheckResourceExists, Kind: "Deployment", Name: "primary-deploy"},
						},
					},
				},
				{
					Name:         "secondary",
					Controller:   "secondary-controller",
					Dependencies: []string{"primary", "primary"}, // Duplicate dependency
					SteadyState: v1alpha1.SteadyStateSpec{
						Checks: []v1alpha1.SteadyStateCheck{
							{Type: v1alpha1.CheckConditionTrue, Kind: "Service", Name: "secondary-svc"},
						},
					},
				},
			},
		},
	}

	graph, err := BuildDependencyGraph(models)
	require.NoError(t, err)
	require.NotNil(t, graph)

	primaryRef := ComponentRef{Operator: "delta-operator", Component: "primary"}
	dependents := graph.DirectDependents(primaryRef)

	// Should only have one dependent despite duplicate declaration
	require.Len(t, dependents, 1)
	assert.Equal(t, "delta-operator", dependents[0].Ref.Operator)
	assert.Equal(t, "secondary", dependents[0].Ref.Component)
}

// TestBuildDependencyGraph_NoSteadyStateExcluded tests that components without
// steady-state checks are still included in the dependency graph. Edge direction
// is independent of steady-state presence.
func TestBuildDependencyGraph_NoSteadyStateExcluded(t *testing.T) {
	models := []*OperatorKnowledge{
		{
			Operator: OperatorMeta{
				Name:      "alpha-operator",
				Namespace: "alpha-ns",
			},
			Components: []ComponentModel{
				{
					Name:       "comp-no-checks",
					Controller: "alpha-controller",
					// No SteadyState defined
				},
			},
		},
		{
			Operator: OperatorMeta{
				Name:      "beta-operator",
				Namespace: "beta-ns",
			},
			Components: []ComponentModel{
				{
					Name:         "beta-comp",
					Controller:   "beta-controller",
					Dependencies: []string{"alpha-operator"},
					SteadyState: v1alpha1.SteadyStateSpec{
						Checks: []v1alpha1.SteadyStateCheck{
							{Type: v1alpha1.CheckResourceExists, Kind: "Pod", Name: "beta-pod"},
						},
					},
				},
			},
		},
	}

	graph, err := BuildDependencyGraph(models)
	require.NoError(t, err)
	require.NotNil(t, graph)

	// beta-comp depends on alpha-operator, so comp-no-checks should have beta-comp as dependent
	compNoChecksRef := ComponentRef{Operator: "alpha-operator", Component: "comp-no-checks"}
	dependents := graph.DirectDependents(compNoChecksRef)

	require.Len(t, dependents, 1)
	assert.Equal(t, "beta-operator", dependents[0].Ref.Operator)
	assert.Equal(t, "beta-comp", dependents[0].Ref.Component)
	assert.Equal(t, "beta-ns", dependents[0].Namespace)
}

// TestBuildDependencyGraph_DirectDependentsUnknownRef tests that querying
// DirectDependents with an unknown reference returns nil, not an error.
func TestBuildDependencyGraph_DirectDependentsUnknownRef(t *testing.T) {
	// Empty graph
	graph, err := BuildDependencyGraph(nil)
	require.NoError(t, err)
	require.NotNil(t, graph)

	unknownRef := ComponentRef{Operator: "unknown-operator", Component: "unknown-component"}
	dependents := graph.DirectDependents(unknownRef)

	assert.Nil(t, dependents)
}
