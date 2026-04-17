package upgrade

import (
	"context"
	"fmt"
	"io"
)

// ExperimentRunner is a function that runs a chaos experiment file and returns a verdict string.
type ExperimentRunner func(ctx context.Context, experimentPath, knowledgeDir string) (verdict string, err error)

// ChaosExecutor runs chaos experiments using the existing experiment runner.
type ChaosExecutor struct {
	runner ExperimentRunner
}

func NewChaosExecutor(runner ExperimentRunner) *ChaosExecutor {
	return &ChaosExecutor{runner: runner}
}

func (e *ChaosExecutor) Execute(ctx context.Context, step PlaybookStep, pb *PlaybookSpec, _ *PlaybookState, out io.Writer) error {
	knowledgeDir := step.Knowledge
	if knowledgeDir == "" {
		knowledgeDir = ResolveKnowledgeDir(step, pb)
	}

	var failures int
	for _, expPath := range step.Experiments {
		_, _ = fmt.Fprintf(out, "  running: %s\n", expPath)
		verdict, err := e.runner(ctx, expPath, knowledgeDir)
		if err != nil {
			_, _ = fmt.Fprintf(out, "  %s: ERROR: %v\n", expPath, err)
			failures++
			continue
		}
		_, _ = fmt.Fprintf(out, "  %s: %s\n", expPath, verdict)
		if verdict == "Failed" {
			failures++
		}
	}

	if failures > 0 {
		return fmt.Errorf("%d/%d experiments failed", failures, len(step.Experiments))
	}

	return nil
}
