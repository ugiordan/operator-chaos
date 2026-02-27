package sdk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminEndpointFaultPoints(t *testing.T) {
	cfg := &FaultConfig{
		Active: true,
		Faults: map[string]FaultSpec{
			"get":    {ErrorRate: 0.5, Error: "timeout"},
			"create": {ErrorRate: 1.0, Error: "forbidden"},
		},
	}

	handler := NewAdminHandler(cfg)
	req := httptest.NewRequest("GET", "/chaos/faultpoints", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "get")
	assert.Contains(t, w.Body.String(), "create")
}

func TestAdminEndpointStatus(t *testing.T) {
	cfg := &FaultConfig{
		Active: true,
		Faults: map[string]FaultSpec{
			"get": {ErrorRate: 0.5, Error: "timeout"},
		},
	}

	handler := NewAdminHandler(cfg)
	req := httptest.NewRequest("GET", "/chaos/status", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var status map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
	assert.Equal(t, true, status["active"])
	assert.Equal(t, float64(1), status["faultCount"])
}

func TestAdminEndpointNilConfig(t *testing.T) {
	handler := NewAdminHandler(nil)

	req := httptest.NewRequest("GET", "/chaos/status", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var status map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
	assert.Equal(t, false, status["active"])
}
