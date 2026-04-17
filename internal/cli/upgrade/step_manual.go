package upgrade

import (
	"bufio"
	"context"
	"fmt"
	"io"
)

// ManualExecutor pauses for user confirmation or runs autoCheck in CI mode.
type ManualExecutor struct {
	input      io.Reader
	skipManual bool
	runCommand CommandRunner
}

func NewManualExecutor(input io.Reader, skipManual bool, runner CommandRunner) *ManualExecutor {
	if runner == nil {
		runner = DefaultCommandRunner
	}
	return &ManualExecutor{input: input, skipManual: skipManual, runCommand: runner}
}

func (e *ManualExecutor) Execute(ctx context.Context, step PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, out io.Writer) error {
	if e.skipManual {
		if step.AutoCheck != "" {
			_, _ = fmt.Fprintf(out, "  running autoCheck: %s\n", step.AutoCheck)
			_, stderr, err := e.runCommand(ctx, step.AutoCheck)
			if err != nil {
				if stderr != "" {
					_, _ = fmt.Fprintf(out, "  stderr: %s", stderr)
				}
				return fmt.Errorf("autoCheck failed: %w", err)
			}
			_, _ = fmt.Fprintf(out, "  autoCheck passed\n")
			return nil
		}
		_, _ = fmt.Fprintf(out, "  WARNING: skipped (no autoCheck defined)\n")
		return ErrStepSkipped
	}

	_, _ = fmt.Fprintf(out, "  %s\n", step.Description)
	_, _ = fmt.Fprintf(out, "  Press Enter to continue...")

	scanner := bufio.NewScanner(e.input)
	scanner.Scan()

	return nil
}
