package sdk

import (
	"encoding/json"
	"net/http"
)

// NewAdminHandler creates an HTTP handler that exposes chaos status and fault
// points for debugging and administration. It registers two endpoints:
//
//   - GET /chaos/faultpoints — lists all configured fault injection points
//   - GET /chaos/status      — returns whether chaos is active and how many faults are configured
//
// A nil FaultConfig is handled gracefully: faultpoints returns an empty list
// and status reports inactive with zero faults.
func NewAdminHandler(cfg *FaultConfig) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/chaos/faultpoints", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if cfg == nil {
			json.NewEncoder(w).Encode([]string{}) //nolint:errcheck
			return
		}
		cfg.mu.RLock()
		defer cfg.mu.RUnlock()

		type faultPoint struct {
			Name      string  `json:"name"`
			Active    bool    `json:"active"`
			ErrorRate float64 `json:"errorRate"`
			Error     string  `json:"error"`
		}

		points := make([]faultPoint, 0, len(cfg.Faults))
		for name, spec := range cfg.Faults {
			points = append(points, faultPoint{
				Name:      name,
				Active:    cfg.Active,
				ErrorRate: spec.ErrorRate,
				Error:     spec.Error,
			})
		}
		json.NewEncoder(w).Encode(points) //nolint:errcheck
	})

	mux.HandleFunc("/chaos/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		status := map[string]interface{}{
			"active":     cfg != nil && cfg.IsActive(),
			"faultCount": 0,
		}
		if cfg != nil {
			cfg.mu.RLock()
			status["faultCount"] = len(cfg.Faults)
			cfg.mu.RUnlock()
		}
		json.NewEncoder(w).Encode(status) //nolint:errcheck
	})

	return mux
}
