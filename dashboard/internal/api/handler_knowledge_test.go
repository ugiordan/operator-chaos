package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
)

func TestHandleKnowledge(t *testing.T) {
	knowledge := []model.OperatorKnowledge{{
		Operator: model.OperatorMeta{Name: "opendatahub-operator"},
		Components: []model.ComponentModel{{
			Name: "odh-model-controller",
			ManagedResources: []model.ManagedResource{
				{Kind: "Deployment", Name: "odh-model-controller"},
			},
		}},
	}}

	srv := NewServer(&mockStore{}, nil, knowledge)
	req := httptest.NewRequest("GET", "/api/v1/knowledge/opendatahub-operator/odh-model-controller", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Equal(t, "odh-model-controller", result["name"])
}

func TestHandleKnowledge_NotFound(t *testing.T) {
	srv := NewServer(&mockStore{}, nil, nil)
	req := httptest.NewRequest("GET", "/api/v1/knowledge/unknown/unknown", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
