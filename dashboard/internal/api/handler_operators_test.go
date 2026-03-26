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

type operatorsMockStore struct {
	mockStore
	operators []string
	exps      []store.Experiment
}

func (m *operatorsMockStore) ListOperators(since *time.Time) ([]string, error) {
	return m.operators, nil
}

func (m *operatorsMockStore) ListByOperator(op string, since *time.Time) ([]store.Experiment, error) {
	var result []store.Experiment
	for _, e := range m.exps {
		if e.Operator == op {
			result = append(result, e)
		}
	}
	return result, nil
}

func TestHandleListOperators(t *testing.T) {
	ms := &operatorsMockStore{operators: []string{"op1", "op2"}}
	srv := NewServer(ms, nil, nil)
	req := httptest.NewRequest("GET", "/api/v1/operators", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	var result []string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Equal(t, []string{"op1", "op2"}, result)
}

func TestHandleListComponents(t *testing.T) {
	ms := &operatorsMockStore{
		exps: []store.Experiment{
			{Operator: "op1", Component: "comp1", SpecJSON: "{}", StatusJSON: "{}"},
			{Operator: "op1", Component: "comp2", SpecJSON: "{}", StatusJSON: "{}"},
			{Operator: "op1", Component: "comp1", SpecJSON: "{}", StatusJSON: "{}"},
			{Operator: "op2", Component: "comp3", SpecJSON: "{}", StatusJSON: "{}"},
		},
	}
	srv := NewServer(ms, nil, nil)
	req := httptest.NewRequest("GET", "/api/v1/operators/op1/components", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	var result []string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Len(t, result, 2)
	assert.Contains(t, result, "comp1")
	assert.Contains(t, result, "comp2")
}
