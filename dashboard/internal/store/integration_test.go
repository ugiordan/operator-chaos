package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_FullLifecycle(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC().Truncate(time.Millisecond)
	recovery1 := int64(32000)
	recovery2 := int64(58000)

	exps := []Experiment{
		{ID: "ns/e1/" + now.Format(time.RFC3339), Name: "e1", Namespace: "ns", Operator: "op1", Component: "comp1", InjectionType: "PodKill", Phase: "Complete", Verdict: "Resilient", RecoveryMs: &recovery1, StartTime: &now, SuiteRunID: "run-1", SuiteName: "suite-a", OperatorVersion: "v1.0", SpecJSON: "{}", StatusJSON: "{}"},
		{ID: "ns/e2/" + now.Format(time.RFC3339), Name: "e2", Namespace: "ns", Operator: "op1", Component: "comp1", InjectionType: "ConfigDrift", Phase: "Complete", Verdict: "Degraded", RecoveryMs: &recovery2, StartTime: &now, SuiteRunID: "run-1", SuiteName: "suite-a", OperatorVersion: "v1.0", SpecJSON: "{}", StatusJSON: "{}"},
		{ID: "ns/e3/" + now.Format(time.RFC3339), Name: "e3", Namespace: "ns", Operator: "op1", Component: "comp1", InjectionType: "PodKill", Phase: "Observing", Verdict: "", StartTime: &now, SpecJSON: "{}", StatusJSON: "{}"},
	}

	for _, e := range exps {
		require.NoError(t, s.Upsert(e))
	}

	// Test overview stats
	stats, err := s.OverviewStats(nil)
	require.NoError(t, err)
	assert.Equal(t, 3, stats.Total)
	assert.Equal(t, 1, stats.Resilient)
	assert.Equal(t, 1, stats.Degraded)
	assert.Equal(t, 1, stats.Running)

	// Test avg recovery
	avgs, err := s.AvgRecoveryByType(nil)
	require.NoError(t, err)
	require.Len(t, avgs, 2)

	// Test list by suite
	suiteExps, err := s.ListBySuiteRunID("run-1")
	require.NoError(t, err)
	assert.Len(t, suiteExps, 2)

	// Test list operators
	ops, err := s.ListOperators(nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"op1"}, ops)

	// Test filtering
	result, err := s.List(ListFilter{Phase: "Observing", Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalCount)
	assert.Equal(t, "e3", result.Items[0].Name)

	// Test ListRunning
	running, err := s.ListRunning()
	require.NoError(t, err)
	assert.Len(t, running, 1)
	assert.Equal(t, "e3", running[0].Name)

	// Test ListSuiteRuns
	suiteRuns, err := s.ListSuiteRuns()
	require.NoError(t, err)
	assert.Len(t, suiteRuns, 1)
	assert.Equal(t, "suite-a", suiteRuns[0].SuiteName)
}
