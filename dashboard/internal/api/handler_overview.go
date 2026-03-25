package api

import (
	"net/http"
	"time"
)

func (s *Server) handleOverviewStats(w http.ResponseWriter, r *http.Request) {
	var since *time.Time
	if v := r.URL.Query().Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			since = &t
		} else if d, err := time.ParseDuration(v); err == nil {
			t := time.Now().Add(-d)
			since = &t
		}
	}

	stats, err := s.store.OverviewStats(since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	avgs, err := s.store.AvgRecoveryByType(since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	avgMap := make(map[string]int64, len(avgs))
	for _, a := range avgs {
		avgMap[a.InjectionType] = a.AvgMs
	}

	running, _ := s.store.ListRunning()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total":              stats.Total,
		"resilient":          stats.Resilient,
		"degraded":           stats.Degraded,
		"failed":             stats.Failed,
		"inconclusive":       stats.Inconclusive,
		"running":            stats.Running,
		"avgRecoveryByType":  avgMap,
		"runningExperiments": running,
	})
}
