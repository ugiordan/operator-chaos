package api

import (
	"encoding/json"
	"net/http"

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
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func pathSegment(r *http.Request, name string) string {
	return r.PathValue(name)
}
