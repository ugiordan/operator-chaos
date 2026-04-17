package upgrade

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// ErrStepSkipped is returned by step executors to signal that the step was
// intentionally skipped (e.g. manual step in CI mode without autoCheck).
// The executor records this as status "skipped" instead of "completed".
var ErrStepSkipped = errors.New("step skipped")

// StepExecutor executes a single playbook step.
type StepExecutor interface {
	Execute(ctx context.Context, step PlaybookStep, pb *PlaybookSpec, state *PlaybookState, out io.Writer) error
}

// StepRegistry maps step types to their executors.
type StepRegistry struct {
	executors map[string]StepExecutor
}

// NewStepRegistry creates a registry with no executors registered.
func NewStepRegistry() *StepRegistry {
	return &StepRegistry{executors: make(map[string]StepExecutor)}
}

// Register adds an executor for a step type.
func (r *StepRegistry) Register(stepType string, executor StepExecutor) {
	r.executors[stepType] = executor
}

// Get returns the executor for a step type.
func (r *StepRegistry) Get(stepType string) (StepExecutor, error) {
	e, ok := r.executors[stepType]
	if !ok {
		return nil, fmt.Errorf("no executor registered for step type %q", stepType)
	}
	return e, nil
}
