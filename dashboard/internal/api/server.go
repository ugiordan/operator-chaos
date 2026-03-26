package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/opendatahub-io/odh-platform-chaos/dashboard/internal/store"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
)

type Server struct {
	store     store.Store
	broker    *SSEBroker
	knowledge []model.OperatorKnowledge
	mux       *http.ServeMux
}

func NewServer(s store.Store, broker *SSEBroker, knowledge []model.OperatorKnowledge) *Server {
	srv := &Server{store: s, broker: broker, knowledge: knowledge}
	srv.mux = http.NewServeMux()
	srv.registerRoutes()
	return srv
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) registerRoutes() {
	// SSE live endpoint registered first. Go 1.22+ ServeMux uses most-specific-match,
	// so the literal "/experiments/live" always wins over the wildcard "{namespace}/{name}".
	if s.broker != nil {
		s.mux.HandleFunc("GET /api/v1/experiments/live", s.broker.ServeHTTP)
	}
	s.mux.HandleFunc("GET /api/v1/experiments", s.handleListExperiments)
	s.mux.HandleFunc("GET /api/v1/experiments/{namespace}/{name}", s.handleGetExperiment)
	s.mux.HandleFunc("GET /api/v1/overview/stats", s.handleOverviewStats)
	s.mux.HandleFunc("GET /api/v1/operators", s.handleListOperators)
	s.mux.HandleFunc("GET /api/v1/operators/{operator}/components", s.handleListComponents)
	s.mux.HandleFunc("GET /api/v1/knowledge/{operator}/{component}", s.handleKnowledge)
	s.mux.HandleFunc("GET /api/v1/suites", s.handleListSuiteRuns)
	s.mux.HandleFunc("GET /api/v1/suites/compare", s.handleCompareSuiteRuns)
	s.mux.HandleFunc("GET /api/v1/suites/{runId}", s.handleGetSuiteRun)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func internalError(w http.ResponseWriter, context string, err error) {
	log.Printf("%s: %v", context, err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

func pathSegment(r *http.Request, name string) string {
	return r.PathValue(name)
}

func parseSince(r *http.Request) *time.Time {
	v := r.URL.Query().Get("since")
	if v == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return &t
	}
	if d, err := time.ParseDuration(v); err == nil {
		t := time.Now().Add(-d)
		return &t
	}
	return nil
}
