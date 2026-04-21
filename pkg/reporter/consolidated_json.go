package reporter

import (
	"encoding/json"
	"io"
	"time"
)

// ConsolidatedReport wraps all experiment reports with a summary.
type ConsolidatedReport struct {
	Generated   time.Time          `json:"generated"`
	Experiments []ExperimentReport `json:"experiments"`
	Summary     ReportSummary      `json:"summary"`
}

// ConsolidatedJSONReporter produces a single JSON file containing
// all experiment reports and an aggregated summary.
type ConsolidatedJSONReporter struct{}

var _ Reporter = (*ConsolidatedJSONReporter)(nil)

// WriteReport serializes all reports into a ConsolidatedReport and writes it as JSON.
func (c *ConsolidatedJSONReporter) WriteReport(w io.Writer, reports []ExperimentReport) error {
	if reports == nil {
		reports = []ExperimentReport{}
	}
	consolidated := ConsolidatedReport{
		Generated:   time.Now().UTC(),
		Experiments: reports,
		Summary:     ComputeSummary(reports),
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(consolidated)
}
