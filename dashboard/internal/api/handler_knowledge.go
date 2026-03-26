package api

import "net/http"

func (s *Server) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	operator := pathSegment(r, "operator")
	component := pathSegment(r, "component")

	for _, k := range s.knowledge {
		if k.Operator.Name == operator {
			for _, c := range k.Components {
				if c.Name == component {
					writeJSON(w, http.StatusOK, c)
					return
				}
			}
		}
	}
	writeError(w, http.StatusNotFound, "component not found")
}
