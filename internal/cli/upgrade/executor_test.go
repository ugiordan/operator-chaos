package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestPlaybook() *PlaybookSpec {
	return &PlaybookSpec{
		Metadata: PlaybookMetadata{Name: "test-upgrade", Description: "test"},
		Upgrade: &UpgradeSpec{
			Source: VersionRef{KnowledgeDir: "knowledge/v1/", Version: "1.0"},
			Target: VersionRef{KnowledgeDir: "knowledge/v2/", Version: "2.0"},
			Path:   &UpgradePath{Operator: "test-op", Namespace: "test-ns", Hops: []Hop{{Channel: "stable-2.0"}}},
			Steps: []PlaybookStep{
				{Name: "step-1", Type: "test"},
				{Name: "step-2", Type: "test"},
				{Name: "step-3", Type: "test"},
			},
		},
	}
}

func TestExecutorRunsAllSteps(t *testing.T) {
	pb := makeTestPlaybook()
	reg := NewStepRegistry()
	var executedSteps []string
	reg.Register("test", &funcExecutor{fn: func(_ context.Context, step PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, _ io.Writer) error {
		executedSteps = append(executedSteps, step.Name)
		return nil
	}})

	stateDir := t.TempDir()
	exe := NewExecutor(reg, ExecutorOptions{StateDir: stateDir})

	var out bytes.Buffer
	err := exe.Run(context.Background(), pb, &out)
	require.NoError(t, err)
	assert.Equal(t, []string{"step-1", "step-2", "step-3"}, executedSteps)
	assert.Contains(t, out.String(), "[1/3]")
	assert.Contains(t, out.String(), "[3/3]")
}

func TestExecutorHaltsOnFailure(t *testing.T) {
	pb := makeTestPlaybook()
	reg := NewStepRegistry()
	callCount := 0
	reg.Register("test", &funcExecutor{fn: func(_ context.Context, step PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, _ io.Writer) error {
		callCount++
		if step.Name == "step-2" {
			return assert.AnError
		}
		return nil
	}})

	stateDir := t.TempDir()
	exe := NewExecutor(reg, ExecutorOptions{StateDir: stateDir})

	var out bytes.Buffer
	err := exe.Run(context.Background(), pb, &out)
	assert.Error(t, err)
	assert.Equal(t, 2, callCount)
	assert.Contains(t, out.String(), "FAIL")
	assert.Contains(t, out.String(), "resume-from")

	// Verify state file was written
	stateFile := filepath.Join(stateDir, "test-upgrade.state.json")
	data, err := os.ReadFile(stateFile)
	require.NoError(t, err)
	var state PlaybookState
	require.NoError(t, json.Unmarshal(data, &state))
	assert.Equal(t, "step-2", state.FailedStep)
	assert.Contains(t, state.CompletedSteps, "step-1")
}

func TestExecutorResumeFromStep(t *testing.T) {
	pb := makeTestPlaybook()
	reg := NewStepRegistry()
	var executedSteps []string
	reg.Register("test", &funcExecutor{fn: func(_ context.Context, step PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, _ io.Writer) error {
		executedSteps = append(executedSteps, step.Name)
		return nil
	}})

	// Pre-populate state file with step-1 completed, step-2 failed
	stateDir := t.TempDir()
	state := PlaybookState{
		PlaybookName:   "test-upgrade",
		StartedAt:      time.Now(),
		CompletedSteps: map[string]StepResult{"step-1": {Status: "completed", FinishedAt: time.Now()}},
		FailedStep:     "step-2",
	}
	stateData, _ := json.Marshal(state)
	stateFile := filepath.Join(stateDir, "test-upgrade.state.json")
	require.NoError(t, os.WriteFile(stateFile, stateData, 0644))

	exe := NewExecutor(reg, ExecutorOptions{StateDir: stateDir, ResumeFrom: "step-2"})

	var out bytes.Buffer
	err := exe.Run(context.Background(), pb, &out)
	require.NoError(t, err)
	assert.Equal(t, []string{"step-2", "step-3"}, executedSteps)
}

func TestExecutorResumeFromWithoutStateRequiresForce(t *testing.T) {
	pb := makeTestPlaybook()
	reg := NewStepRegistry()
	reg.Register("test", &funcExecutor{fn: func(_ context.Context, _ PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, _ io.Writer) error {
		return nil
	}})

	stateDir := t.TempDir()
	exe := NewExecutor(reg, ExecutorOptions{StateDir: stateDir, ResumeFrom: "step-2"})

	var out bytes.Buffer
	err := exe.Run(context.Background(), pb, &out)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "force-resume")
}

func TestExecutorForceResume(t *testing.T) {
	pb := makeTestPlaybook()
	reg := NewStepRegistry()
	var executedSteps []string
	reg.Register("test", &funcExecutor{fn: func(_ context.Context, step PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, _ io.Writer) error {
		executedSteps = append(executedSteps, step.Name)
		return nil
	}})

	stateDir := t.TempDir()
	exe := NewExecutor(reg, ExecutorOptions{StateDir: stateDir, ResumeFrom: "step-2", ForceResume: true})

	var out bytes.Buffer
	err := exe.Run(context.Background(), pb, &out)
	require.NoError(t, err)
	assert.Equal(t, []string{"step-2", "step-3"}, executedSteps)
}

func TestExecutorBlocksOnExistingFailedState(t *testing.T) {
	pb := makeTestPlaybook()
	reg := NewStepRegistry()
	reg.Register("test", &funcExecutor{fn: func(_ context.Context, _ PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, _ io.Writer) error {
		t.Fatal("should not execute when previous run has a failed step")
		return nil
	}})

	// Pre-populate state file with a failed step
	stateDir := t.TempDir()
	state := PlaybookState{
		PlaybookName:   "test-upgrade",
		StartedAt:      time.Now(),
		CompletedSteps: map[string]StepResult{"step-1": {Status: "completed", FinishedAt: time.Now()}},
		FailedStep:     "step-2",
		FailedError:    "something broke",
	}
	stateData, _ := json.Marshal(state)
	stateFile := filepath.Join(stateDir, "test-upgrade.state.json")
	require.NoError(t, os.WriteFile(stateFile, stateData, 0644))

	exe := NewExecutor(reg, ExecutorOptions{StateDir: stateDir})

	var out bytes.Buffer
	err := exe.Run(context.Background(), pb, &out)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "previous run failed")
	assert.Contains(t, err.Error(), "step-2")
}

func TestExecutorSkippedStatus(t *testing.T) {
	pb := makeTestPlaybook()
	reg := NewStepRegistry()
	callCount := 0
	reg.Register("test", &funcExecutor{fn: func(_ context.Context, step PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, _ io.Writer) error {
		callCount++
		if step.Name == "step-2" {
			return ErrStepSkipped
		}
		return nil
	}})

	stateDir := t.TempDir()
	exe := NewExecutor(reg, ExecutorOptions{StateDir: stateDir})

	var out bytes.Buffer
	err := exe.Run(context.Background(), pb, &out)
	require.NoError(t, err)
	assert.Equal(t, 3, callCount) // all 3 steps should run
	assert.Contains(t, out.String(), "SKIP")
}

func TestExecutorDryRun(t *testing.T) {
	pb := makeTestPlaybook()
	reg := NewStepRegistry()
	reg.Register("test", &funcExecutor{fn: func(_ context.Context, _ PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, _ io.Writer) error {
		t.Fatal("should not execute in dry-run mode")
		return nil
	}})

	stateDir := t.TempDir()
	exe := NewExecutor(reg, ExecutorOptions{StateDir: stateDir, DryRun: true})

	var out bytes.Buffer
	err := exe.Run(context.Background(), pb, &out)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "step-1")
	assert.Contains(t, out.String(), "step-2")
	assert.Contains(t, out.String(), "step-3")
}

// funcExecutor wraps a function as a StepExecutor for testing.
type funcExecutor struct {
	fn func(ctx context.Context, step PlaybookStep, pb *PlaybookSpec, state *PlaybookState, out io.Writer) error
}

func (f *funcExecutor) Execute(ctx context.Context, step PlaybookStep, pb *PlaybookSpec, state *PlaybookState, out io.Writer) error {
	return f.fn(ctx, step, pb, state, out)
}

func TestExecutorRunsChaosPlaybook(t *testing.T) {
	pb := &PlaybookSpec{
		Kind: "ChaosPlaybook",
		Metadata: PlaybookMetadata{
			Name:        "test-chaos",
			Description: "test chaos playbook",
		},
		Chaos: &ChaosSpec{
			KnowledgeDir: "knowledge/chaos/",
			Steps: []PlaybookStep{
				{Name: "validate-version", Type: "validate-version"},
				{Name: "run-chaos", Type: "chaos"},
			},
		},
	}

	reg := NewStepRegistry()
	var executedSteps []string
	reg.Register("validate-version", &funcExecutor{fn: func(_ context.Context, step PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, _ io.Writer) error {
		executedSteps = append(executedSteps, step.Name)
		return nil
	}})
	reg.Register("chaos", &funcExecutor{fn: func(_ context.Context, step PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, _ io.Writer) error {
		executedSteps = append(executedSteps, step.Name)
		return nil
	}})

	stateDir := t.TempDir()
	exe := NewExecutor(reg, ExecutorOptions{StateDir: stateDir})

	var out bytes.Buffer
	err := exe.Run(context.Background(), pb, &out)
	require.NoError(t, err)

	output := out.String()
	assert.Contains(t, output, "ChaosPlaybook: test-chaos")
	assert.Contains(t, output, "knowledge/chaos/")
	assert.NotContains(t, output, "Source:")
	assert.NotContains(t, output, "Target:")
	assert.Equal(t, []string{"validate-version", "run-chaos"}, executedSteps)
}

func TestExecutorSkipsSyntheticStepsInState(t *testing.T) {
	pb := &PlaybookSpec{
		Metadata: PlaybookMetadata{Name: "test-synthetic", Description: "test"},
		Upgrade: &UpgradeSpec{
			Source: VersionRef{KnowledgeDir: "knowledge/v1/", Version: "1.0"},
			Target: VersionRef{KnowledgeDir: "knowledge/v2/", Version: "2.0"},
			Path:   &UpgradePath{Operator: "test-op", Namespace: "test-ns", Hops: []Hop{{Channel: "stable"}}},
			Steps: []PlaybookStep{
				{Name: "synthetic-step", Type: "test", Synthetic: true},
				{Name: "normal-step", Type: "test"},
			},
		},
	}

	reg := NewStepRegistry()
	var executedSteps []string
	reg.Register("test", &funcExecutor{fn: func(_ context.Context, step PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, _ io.Writer) error {
		executedSteps = append(executedSteps, step.Name)
		return nil
	}})

	stateDir := t.TempDir()
	exe := NewExecutor(reg, ExecutorOptions{StateDir: stateDir})

	var out bytes.Buffer
	err := exe.Run(context.Background(), pb, &out)
	require.NoError(t, err)

	output := out.String()
	assert.Contains(t, output, "[auto] synthetic-step")
	assert.NotContains(t, output, "[auto] normal-step")
	assert.Equal(t, []string{"synthetic-step", "normal-step"}, executedSteps)

	// State file should have been cleaned up on success
	stateFile := filepath.Join(stateDir, "test-synthetic.state.json")
	_, err = os.Stat(stateFile)
	assert.True(t, os.IsNotExist(err), "state file should be removed on success")
}
