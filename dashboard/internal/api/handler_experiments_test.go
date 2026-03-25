package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-chaos/dashboard/internal/store"
)

type mockStore struct {
	experiments []store.Experiment
}

func (m *mockStore) List(f store.ListFilter) (store.ListResult, error) {
	var filtered []store.Experiment
	for _, e := range m.experiments {
		if f.Operator != "" && e.Operator != f.Operator { continue }
		if f.Verdict != "" && e.Verdict != f.Verdict { continue }
		filtered = append(filtered, e)
	}
	return store.ListResult{Items: filtered, TotalCount: len(filtered)}, nil
}

func (m *mockStore) Get(namespace, name string) (*store.Experiment, error) {
	for _, e := range m.experiments {
		if e.Namespace == namespace && e.Name == name { return &e, nil }
	}
	return nil, nil
}

func (m *mockStore) Upsert(exp store.Experiment) error { return nil }
func (m *mockStore) ListRunning() ([]store.Experiment, error) { return nil, nil }
func (m *mockStore) OverviewStats(since *time.Time) (store.OverviewStats, error) { return store.OverviewStats{}, nil }
func (m *mockStore) AvgRecoveryByType(since *time.Time) ([]store.RecoveryAvg, error) { return nil, nil }
func (m *mockStore) ListOperators(since *time.Time) ([]string, error) { return nil, nil }
func (m *mockStore) ListByOperator(op string, since *time.Time) ([]store.Experiment, error) { return nil, nil }
func (m *mockStore) ListSuiteRuns() ([]store.SuiteRun, error) { return nil, nil }
func (m *mockStore) ListBySuiteRunID(id string) ([]store.Experiment, error) { return nil, nil }
func (m *mockStore) CompareSuiteRuns(sn, a, b string) ([]store.Experiment, []store.Experiment, error) { return nil, nil, nil }
func (m *mockStore) Close() error { return nil }

func TestHandleListExperiments(t *testing.T) {
	ms := &mockStore{experiments: []store.Experiment{
		{Name: "e1", Namespace: "ns", Operator: "op1", Verdict: "Resilient", SpecJSON: "{}", StatusJSON: "{}"},
		{Name: "e2", Namespace: "ns", Operator: "op2", Verdict: "Failed", SpecJSON: "{}", StatusJSON: "{}"},
	}}

	srv := NewServer(ms, nil, nil)
	req := httptest.NewRequest("GET", "/api/v1/experiments?operator=op1", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var result struct {
		Items      []json.RawMessage `json:"items"`
		TotalCount int               `json:"totalCount"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Equal(t, 1, result.TotalCount)
}

func TestHandleGetExperiment(t *testing.T) {
	ms := &mockStore{experiments: []store.Experiment{
		{Name: "e1", Namespace: "ns", Operator: "op1", SpecJSON: "{}", StatusJSON: "{}"},
	}}

	srv := NewServer(ms, nil, nil)
	req := httptest.NewRequest("GET", "/api/v1/experiments/ns/e1", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetExperiment_NotFound(t *testing.T) {
	ms := &mockStore{}
	srv := NewServer(ms, nil, nil)
	req := httptest.NewRequest("GET", "/api/v1/experiments/ns/missing", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSSERoute_DoesNotShadowExperimentGet(t *testing.T) {
	ms := &mockStore{experiments: []store.Experiment{
		{Name: "live", Namespace: "experiments", SpecJSON: "{}", StatusJSON: "{}"},
	}}
	broker := NewSSEBroker()
	go broker.Run()
	defer broker.Stop()

	srv := NewServer(ms, broker, nil)
	req := httptest.NewRequest("GET", "/api/v1/experiments/live", nil)
	rec := httptest.NewRecorder()
	go srv.Handler().ServeHTTP(rec, req)
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
}
