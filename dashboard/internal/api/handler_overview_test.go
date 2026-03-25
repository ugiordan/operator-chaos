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

type overviewMockStore struct {
	mockStore
	stats store.OverviewStats
	avgs  []store.RecoveryAvg
}

func (m *overviewMockStore) OverviewStats(since *time.Time) (store.OverviewStats, error) {
	return m.stats, nil
}

func (m *overviewMockStore) AvgRecoveryByType(since *time.Time) ([]store.RecoveryAvg, error) {
	return m.avgs, nil
}

func TestHandleOverviewStats(t *testing.T) {
	ms := &overviewMockStore{
		stats: store.OverviewStats{Total: 10, Resilient: 7, Degraded: 2, Failed: 1},
		avgs:  []store.RecoveryAvg{{InjectionType: "PodKill", AvgMs: 12000}},
	}

	srv := NewServer(ms, nil, nil)
	req := httptest.NewRequest("GET", "/api/v1/overview/stats", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Equal(t, float64(10), result["total"])
	assert.Equal(t, float64(7), result["resilient"])
}
