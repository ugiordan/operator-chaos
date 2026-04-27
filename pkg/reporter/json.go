package reporter

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	v1alpha1 "github.com/opendatahub-io/operator-chaos/api/v1alpha1"
	"github.com/opendatahub-io/operator-chaos/pkg/evaluator"
	"github.com/opendatahub-io/operator-chaos/pkg/observer"
)

// ExperimentReport is the top-level report for a single chaos experiment.
type ExperimentReport struct {
	Experiment     string                        `json:"experiment"`
	Timestamp      time.Time                     `json:"timestamp"`
	Tier           int32                         `json:"tier,omitempty"`
	Target         TargetReport                  `json:"target"`
	Injection      InjectionReport               `json:"injection"`
	Evaluation     evaluator.EvaluationResult    `json:"evaluation"`
	SteadyState    SteadyStateReport             `json:"steadyState,omitempty"`
	Reconciliation *observer.ReconciliationResult `json:"reconciliation,omitempty"`
	CleanupError   string                        `json:"cleanupError,omitempty"`
	Collateral     []CollateralFinding              `json:"collateral,omitempty"`
}

// TargetReport describes the target of a chaos experiment.
type TargetReport struct {
	Operator  string `json:"operator"`
	Component string `json:"component"`
	Resource  string `json:"resource,omitempty"`
}

// InjectionReport describes the fault injection performed.
type InjectionReport struct {
	Type      string            `json:"type"`
	Targets   []string          `json:"targets,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Details   map[string]string `json:"details,omitempty"`
}

// SteadyStateReport captures pre- and post-injection steady state.
type SteadyStateReport struct {
	Pre  any `json:"pre,omitempty"`
	Post any `json:"post,omitempty"`
}

// CollateralFinding represents a collateral damage check result for an operator/component.
type CollateralFinding struct {
	Operator  string                `json:"operator"`
	Component string                `json:"component"`
	Passed    bool                  `json:"passed"`
	Checks    *v1alpha1.CheckResult `json:"checks,omitempty"`
}

// JSONReporter writes experiment reports as JSON.
type JSONReporter struct {
	writer io.Writer
}

// NewJSONReporter creates a JSONReporter that writes to the given writer.
func NewJSONReporter(w io.Writer) *JSONReporter {
	return &JSONReporter{writer: w}
}

// NewJSONFileReporter creates a JSONReporter that writes to a file at the given path.
func NewJSONFileReporter(path string) (*JSONReporter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("creating report file: %w", err)
	}
	return &JSONReporter{writer: f}, nil
}

// Write serializes the report as pretty-printed JSON and writes it.
func (r *JSONReporter) Write(report ExperimentReport) error {
	encoder := json.NewEncoder(r.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// Close closes the underlying writer if it implements io.Closer.
func (r *JSONReporter) Close() error {
	if closer, ok := r.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
