package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-chaos/dashboard/internal/store"
)

type suitesMockStore struct {
	mockStore
	suiteRuns []store.SuiteRun
}

func (m *suitesMockStore) ListSuiteRuns() ([]store.SuiteRun, error) {
	return m.suiteRuns, nil
}

func (m *suitesMockStore) ListBySuiteRunID(id string) ([]store.Experiment, error) {
	var result []store.Experiment
	for _, e := range m.experiments {
		if e.SuiteRunID == id {
			result = append(result, e)
		}
	}
	return result, nil
}

func TestHandleListSuiteRuns(t *testing.T) {
	ms := &suitesMockStore{
		mockStore: mockStore{experiments: []store.Experiment{
			{Name: "e1", SuiteRunID: "run-1", SuiteName: "suite-a", Verdict: "Resilient", SpecJSON: "{}", StatusJSON: "{}"},
		}},
		suiteRuns: []store.SuiteRun{
			{SuiteName: "suite-a", SuiteRunID: "run-1", OperatorVersion: "v1.0", Total: 2, Resilient: 1, Failed: 1},
		},
	}

	srv := NewServer(ms, nil, nil)
	req := httptest.NewRequest("GET", "/api/v1/suites", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var result []store.SuiteRun
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Len(t, result, 1)
	assert.Equal(t, "suite-a", result[0].SuiteName)
}

func TestHandleGetSuiteRun(t *testing.T) {
	ms := &suitesMockStore{mockStore: mockStore{experiments: []store.Experiment{
		{Name: "e1", SuiteRunID: "run-1", SuiteName: "suite-a", Verdict: "Resilient", SpecJSON: "{}", StatusJSON: "{}"},
		{Name: "e2", SuiteRunID: "run-1", SuiteName: "suite-a", Verdict: "Failed", SpecJSON: "{}", StatusJSON: "{}"},
		{Name: "e3", SuiteRunID: "run-2", SuiteName: "suite-a", Verdict: "Resilient", SpecJSON: "{}", StatusJSON: "{}"},
	}}}

	srv := NewServer(ms, nil, nil)
	req := httptest.NewRequest("GET", "/api/v1/suites/run-1", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var result []json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Len(t, result, 2)
}

func TestHandleGetSuiteRun_NotFound(t *testing.T) {
	ms := &suitesMockStore{}
	srv := NewServer(ms, nil, nil)
	req := httptest.NewRequest("GET", "/api/v1/suites/missing", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleCompareSuiteRuns(t *testing.T) {
	ms := &suitesMockStore{mockStore: mockStore{experiments: []store.Experiment{
		{Name: "e1", SuiteRunID: "run-1", SuiteName: "suite-a", Verdict: "Resilient", SpecJSON: "{}", StatusJSON: "{}"},
		{Name: "e2", SuiteRunID: "run-2", SuiteName: "suite-a", Verdict: "Failed", SpecJSON: "{}", StatusJSON: "{}"},
	}}}

	srv := NewServer(ms, nil, nil)
	req := httptest.NewRequest("GET", "/api/v1/suites/compare?suite=suite-a&runA=run-1&runB=run-2", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Contains(t, result, "runA")
	assert.Contains(t, result, "runB")
}

func TestHandleCompareSuiteRuns_MissingParams(t *testing.T) {
	ms := &suitesMockStore{}
	srv := NewServer(ms, nil, nil)
	req := httptest.NewRequest("GET", "/api/v1/suites/compare?suite=suite-a", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
