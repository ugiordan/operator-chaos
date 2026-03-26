package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func sampleExperiment(name string) Experiment {
	now := time.Now().UTC().Truncate(time.Millisecond)
	recoveryMs := int64(32000)
	return Experiment{
		ID:            "opendatahub/" + name + "/" + now.Format(time.RFC3339),
		Name:          name,
		Namespace:     "opendatahub",
		Operator:      "opendatahub-operator",
		Component:     "odh-model-controller",
		InjectionType: "PodKill",
		Phase:         "Complete",
		Verdict:       "Resilient",
		RecoveryMs:    &recoveryMs,
		StartTime:     &now,
		SpecJSON:      `{"target":{}}`,
		StatusJSON:    `{"phase":"Complete"}`,
	}
}

func TestSQLiteStore_UpsertAndGet(t *testing.T) {
	s := newTestStore(t)
	exp := sampleExperiment("omc-podkill")
	require.NoError(t, s.Upsert(exp))
	got, err := s.Get("opendatahub", "omc-podkill")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "omc-podkill", got.Name)
	assert.Equal(t, "Resilient", got.Verdict)
	assert.Equal(t, int64(32000), *got.RecoveryMs)
}

func TestSQLiteStore_UpsertUpdatesExisting(t *testing.T) {
	s := newTestStore(t)
	exp := sampleExperiment("omc-podkill")
	require.NoError(t, s.Upsert(exp))
	exp.Phase = "Aborted"
	exp.Verdict = "Inconclusive"
	require.NoError(t, s.Upsert(exp))
	got, err := s.Get("opendatahub", "omc-podkill")
	require.NoError(t, err)
	assert.Equal(t, "Aborted", got.Phase)
	assert.Equal(t, "Inconclusive", got.Verdict)
}

func TestSQLiteStore_GetReturnsNilForMissing(t *testing.T) {
	s := newTestStore(t)
	got, err := s.Get("opendatahub", "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSQLiteStore_ListWithFilters(t *testing.T) {
	s := newTestStore(t)
	exp1 := sampleExperiment("omc-podkill")
	exp2 := sampleExperiment("omc-configdrift")
	exp2.ID = "opendatahub/omc-configdrift/" + time.Now().Format(time.RFC3339)
	exp2.InjectionType = "ConfigDrift"
	exp2.Verdict = "Degraded"
	require.NoError(t, s.Upsert(exp1))
	require.NoError(t, s.Upsert(exp2))
	result, err := s.List(ListFilter{Verdict: "Resilient", Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalCount)
	assert.Equal(t, "omc-podkill", result.Items[0].Name)
}

func TestSQLiteStore_ListWithSearch(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.Upsert(sampleExperiment("omc-podkill")))
	exp2 := sampleExperiment("kserve-podkill")
	exp2.ID = "opendatahub/kserve-podkill/" + time.Now().Format(time.RFC3339)
	exp2.Operator = "kserve"
	require.NoError(t, s.Upsert(exp2))
	result, err := s.List(ListFilter{Search: "kserve", Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalCount)
	assert.Equal(t, "kserve-podkill", result.Items[0].Name)
}

func TestSQLiteStore_OverviewStats(t *testing.T) {
	s := newTestStore(t)
	exp1 := sampleExperiment("e1")
	exp1.Verdict = "Resilient"
	exp2 := sampleExperiment("e2")
	exp2.ID = "opendatahub/e2/" + time.Now().Format(time.RFC3339)
	exp2.Verdict = "Degraded"
	exp3 := sampleExperiment("e3")
	exp3.ID = "opendatahub/e3/" + time.Now().Format(time.RFC3339)
	exp3.Phase = "Observing"
	exp3.Verdict = ""
	require.NoError(t, s.Upsert(exp1))
	require.NoError(t, s.Upsert(exp2))
	require.NoError(t, s.Upsert(exp3))
	stats, err := s.OverviewStats(nil)
	require.NoError(t, err)
	assert.Equal(t, 3, stats.Total)
	assert.Equal(t, 1, stats.Resilient)
	assert.Equal(t, 1, stats.Degraded)
	assert.Equal(t, 1, stats.Running)
}
