package api

import "net/http"

func (s *Server) handleListOperators(w http.ResponseWriter, r *http.Request) {
	since := parseSince(r)
	ops, err := s.store.ListOperators(since)
	if err != nil {
		internalError(w, "list operators", err)
		return
	}
	writeJSON(w, http.StatusOK, ops)
}

func (s *Server) handleListComponents(w http.ResponseWriter, r *http.Request) {
	operator := pathSegment(r, "operator")
	since := parseSince(r)
	exps, err := s.store.ListByOperator(operator, since)
	if err != nil {
		internalError(w, "list components", err)
		return
	}

	seen := map[string]bool{}
	var components []string
	for _, e := range exps {
		if !seen[e.Component] {
			seen[e.Component] = true
			components = append(components, e.Component)
		}
	}
	writeJSON(w, http.StatusOK, components)
}
