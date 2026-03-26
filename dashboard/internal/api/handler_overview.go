package api

import "net/http"

func (s *Server) handleOverviewStats(w http.ResponseWriter, r *http.Request) {
	since := parseSince(r)

	stats, err := s.store.OverviewStats(since)
	if err != nil {
		internalError(w, "overview stats", err)
		return
	}

	avgs, err := s.store.AvgRecoveryByType(since)
	if err != nil {
		internalError(w, "avg recovery", err)
		return
	}

	avgMap := make(map[string]int64, len(avgs))
	for _, a := range avgs {
		avgMap[a.InjectionType] = a.AvgMs
	}

	trends, err := s.store.Trends(since)
	if err != nil {
		internalError(w, "trends", err)
		return
	}

	timeline, err := s.store.VerdictTimeline(30)
	if err != nil {
		internalError(w, "verdict timeline", err)
		return
	}

	running, err := s.store.ListRunning()
	if err != nil {
		internalError(w, "list running", err)
		return
	}

	type runningSummary struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Phase     string `json:"phase"`
		Component string `json:"component"`
		Type      string `json:"type"`
	}
	summaries := make([]runningSummary, 0, len(running))
	for _, e := range running {
		summaries = append(summaries, runningSummary{
			Name:      e.Name,
			Namespace: e.Namespace,
			Phase:     e.Phase,
			Component: e.Component,
			Type:      e.InjectionType,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total":              stats.Total,
		"resilient":          stats.Resilient,
		"degraded":           stats.Degraded,
		"failed":             stats.Failed,
		"inconclusive":       stats.Inconclusive,
		"running":            stats.Running,
		"trends":             trends,
		"verdictTimeline":    timeline,
		"avgRecoveryByType":  avgMap,
		"runningExperiments": summaries,
	})
}
