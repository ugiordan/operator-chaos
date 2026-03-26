package api

import "net/http"

func (s *Server) handleListSuiteRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.store.ListSuiteRuns()
	if err != nil {
		internalError(w, "list suite runs", err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) handleGetSuiteRun(w http.ResponseWriter, r *http.Request) {
	runID := pathSegment(r, "runId")
	exps, err := s.store.ListBySuiteRunID(runID)
	if err != nil {
		internalError(w, "get suite run", err)
		return
	}
	if len(exps) == 0 {
		writeError(w, http.StatusNotFound, "suite run not found")
		return
	}
	writeJSON(w, http.StatusOK, exps)
}

func (s *Server) handleCompareSuiteRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	suiteName := q.Get("suite")
	runIDA := q.Get("runA")
	runIDB := q.Get("runB")
	if suiteName == "" || runIDA == "" || runIDB == "" {
		writeError(w, http.StatusBadRequest, "suite, runA, and runB query params required")
		return
	}

	a, b, err := s.store.CompareSuiteRuns(suiteName, runIDA, runIDB)
	if err != nil {
		internalError(w, "compare suite runs", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"runA": a,
		"runB": b,
	})
}
