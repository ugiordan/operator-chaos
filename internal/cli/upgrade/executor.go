package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ExecutorOptions configures the playbook executor.
type ExecutorOptions struct {
	StateDir    string
	ResumeFrom  string
	ForceResume bool
	DryRun      bool
}

// Executor runs upgrade playbooks step-by-step with state tracking.
type Executor struct {
	registry *StepRegistry
	opts     ExecutorOptions
}

// NewExecutor creates a new playbook executor.
func NewExecutor(registry *StepRegistry, opts ExecutorOptions) *Executor {
	return &Executor{registry: registry, opts: opts}
}

// Run executes the playbook, writing progress to out.
func (e *Executor) Run(ctx context.Context, pb *PlaybookSpec, out io.Writer) error {
	steps := pb.Steps()

	// Print header
	printHeader(pb, out)
	_, _ = fmt.Fprintln(out)

	// Dry run mode
	if e.opts.DryRun {
		_, _ = fmt.Fprintln(out, "Execution plan (dry-run):")
		for i, s := range steps {
			_, _ = fmt.Fprintf(out, "  [%d/%d] %s (type: %s)\n", i+1, len(steps), s.Name, s.Type)
		}
		return nil
	}

	// Load or create state
	state, stateFile, err := e.loadOrCreateState(pb)
	if err != nil {
		return err
	}

	// Handle resume logic
	resumeIndex := 0
	if e.opts.ResumeFrom != "" {
		idx, err := e.resolveResumeIndex(pb, state)
		if err != nil {
			return err
		}
		resumeIndex = idx
		_, _ = fmt.Fprintf(out, "Resuming from step: %s\n\n", e.opts.ResumeFrom)
	} else if state.FailedStep != "" {
		// A previous run failed and no --resume-from was specified
		return fmt.Errorf("previous run failed at step %q (error: %s); use --resume-from %s to continue",
			state.FailedStep, state.FailedError, state.FailedStep)
	}

	// Execute steps
	for i, step := range steps {
		if i < resumeIndex {
			continue
		}

		// Prepare step label for display
		stepLabel := step.Name
		if step.Synthetic {
			stepLabel = "[auto] " + step.Name
		}

		// Skip completed steps
		if _, completed := state.CompletedSteps[step.Name]; completed {
			_, _ = fmt.Fprintf(out, "[%d/%d] %s %s SKIP (already completed)\n", i+1, len(steps), stepLabel, dots(stepLabel))
			continue
		}

		executor, execGetErr := e.registry.Get(step.Type)
		if execGetErr != nil {
			_, _ = fmt.Fprintf(out, "[%d/%d] %s %s FAIL\n", i+1, len(steps), stepLabel, dots(stepLabel))
			return fmt.Errorf("step %q: %w", step.Name, execGetErr)
		}

		start := time.Now()
		var stepOut bytes.Buffer
		multiOut := io.MultiWriter(out, &stepOut)

		_, _ = fmt.Fprintf(out, "[%d/%d] %s %s ", i+1, len(steps), stepLabel, dots(stepLabel))
		_, _ = fmt.Fprintln(out) // newline before step output
		execErr := executor.Execute(ctx, step, pb, state, multiOut)
		elapsed := time.Since(start)

		if execErr != nil && !errors.Is(execErr, ErrStepSkipped) {
			_, _ = fmt.Fprintf(out, "[%d/%d] %s %s FAIL (%s)\n", i+1, len(steps), stepLabel, dots(stepLabel), elapsed.Round(time.Second))

			state.FailedStep = step.Name
			state.FailedAt = time.Now()
			state.FailedError = execErr.Error()
			_ = e.saveState(state, stateFile)

			_, _ = fmt.Fprintf(out, "\nRun with --resume-from %s to continue after fixing the issue.\n", step.Name)
			return fmt.Errorf("step %q failed: %w", step.Name, execErr)
		}

		stepStatus := "completed"
		label := "PASS"
		if errors.Is(execErr, ErrStepSkipped) {
			stepStatus = "skipped"
			label = "SKIP"
		}

		_, _ = fmt.Fprintf(out, "[%d/%d] %s %s %s (%s)\n", i+1, len(steps), stepLabel, dots(stepLabel), label, elapsed.Round(time.Second))

		// Skip synthetic steps in state writes
		if !step.Synthetic {
			state.CompletedSteps[step.Name] = StepResult{
				Status:     stepStatus,
				FinishedAt: time.Now(),
				Output:     stepOut.String(),
			}
			state.FailedStep = ""
			state.FailedError = ""
			_ = e.saveState(state, stateFile)
		}
	}

	// Clean up state file on success
	_ = os.Remove(stateFile)

	if pb.Upgrade != nil {
		_, _ = fmt.Fprintf(out, "\nUpgrade complete: v%s → v%s\n", pb.Upgrade.Source.Version, pb.Upgrade.Target.Version)
	} else {
		_, _ = fmt.Fprintf(out, "\nPlaybook complete: %s\n", pb.Metadata.Name)
	}
	return nil
}

func (e *Executor) loadOrCreateState(pb *PlaybookSpec) (*PlaybookState, string, error) {
	stateFile := e.stateFilePath(pb.Metadata.Name)

	data, err := os.ReadFile(stateFile)
	if err == nil {
		var state PlaybookState
		if err := json.Unmarshal(data, &state); err == nil {
			return &state, stateFile, nil
		}
	}

	state := &PlaybookState{
		PlaybookName:   pb.Metadata.Name,
		StartedAt:      time.Now(),
		CompletedSteps: make(map[string]StepResult),
	}
	return state, stateFile, nil
}

func (e *Executor) resolveResumeIndex(pb *PlaybookSpec, state *PlaybookState) (int, error) {
	idx := -1
	for i, s := range pb.Steps() {
		if s.Name == e.opts.ResumeFrom {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, fmt.Errorf("step %q not found in playbook", e.opts.ResumeFrom)
	}

	stateFile := e.stateFilePath(pb.Metadata.Name)
	if _, err := os.Stat(stateFile); os.IsNotExist(err) && !e.opts.ForceResume {
		return 0, fmt.Errorf("no state file found; use --force-resume to skip arbitrary steps")
	}

	if state.FailedStep != "" && state.FailedStep != e.opts.ResumeFrom && !e.opts.ForceResume {
		return 0, fmt.Errorf("state file shows failure at %q, not %q; use --force-resume to override", state.FailedStep, e.opts.ResumeFrom)
	}

	return idx, nil
}

func (e *Executor) saveState(state *PlaybookState, path string) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	// Write atomically: temp file + rename to prevent corruption on crash
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// MakeStateSaver returns a StateSaver function bound to this executor's state directory.
// Pass this to OLMExecutor so it can persist per-hop progress.
func (e *Executor) MakeStateSaver(playbookName string) StateSaver {
	return func(state *PlaybookState) error {
		return e.saveState(state, e.stateFilePath(playbookName))
	}
}

func (e *Executor) stateFilePath(playbookName string) string {
	return filepath.Join(e.opts.StateDir, playbookName+".state.json")
}

func dots(name string) string {
	const maxWidth = 40
	pad := maxWidth - len(name)
	if pad < 2 {
		pad = 2
	}
	return strings.Repeat(".", pad)
}

// printHeader prints a kind-appropriate header for the playbook.
func printHeader(pb *PlaybookSpec, out io.Writer) {
	switch pb.Kind {
	case "ChaosPlaybook":
		_, _ = fmt.Fprintf(out, "ChaosPlaybook: %s\n", pb.Metadata.Name)
		if pb.Chaos != nil {
			_, _ = fmt.Fprintf(out, "Knowledge: %s\n", pb.Chaos.KnowledgeDir)
		}
	default:
		// UpgradePlaybook or empty kind
		_, _ = fmt.Fprintf(out, "Upgrade Playbook: %s\n", pb.Metadata.Name)
		if pb.Upgrade != nil {
			_, _ = fmt.Fprintf(out, "Source: %s v%s | Target: %s v%s\n",
				pb.Upgrade.Source.KnowledgeDir, pb.Upgrade.Source.Version,
				pb.Upgrade.Target.KnowledgeDir, pb.Upgrade.Target.Version)
			if len(pb.Upgrade.Paths) > 0 {
				var channels []string
				for _, p := range pb.Upgrade.Paths {
					for _, h := range p.Hops {
						channels = append(channels, h.Channel)
					}
				}
				_, _ = fmt.Fprintf(out, "Path: %s\n", strings.Join(channels, " → "))
			}
		}
	}
}
