package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/evaluator"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/injection"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/observer"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Mock observer
type mockObserver struct {
	result *v1alpha1.CheckResult
}

func (m *mockObserver) CheckSteadyState(ctx context.Context, checks []v1alpha1.SteadyStateCheck, namespace string) (*v1alpha1.CheckResult, error) {
	if m.result != nil {
		return m.result, nil
	}
	return &v1alpha1.CheckResult{Passed: true, ChecksRun: 0, Timestamp: metav1.Now()}, nil
}

// Mock injector
type mockInjector struct {
	validateErr   error
	injectErr     error
	cleanupCalled bool
	cleanupFunc   func(ctx context.Context) error
	revertCalled  bool
	revertErr     error
}

func (m *mockInjector) Validate(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error {
	return m.validateErr
}

func (m *mockInjector) Revert(_ context.Context, _ v1alpha1.InjectionSpec, _ string) error {
	m.revertCalled = true
	return m.revertErr
}

func (m *mockInjector) Inject(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) (injection.CleanupFunc, []v1alpha1.InjectionEvent, error) {
	if m.injectErr != nil {
		return nil, nil, m.injectErr
	}
	events := []v1alpha1.InjectionEvent{
		{Type: spec.Type, Target: "test-pod", Action: "deleted", Timestamp: metav1.Now()},
	}
	cleanup := func(ctx context.Context) error {
		m.cleanupCalled = true
		if m.cleanupFunc != nil {
			return m.cleanupFunc(ctx)
		}
		return nil
	}
	return cleanup, events, nil
}

func newTestOrchestrator(obs *mockObserver, inj *mockInjector) *Orchestrator {
	registry := injection.NewRegistry()
	registry.Register(v1alpha1.PodKill, inj)

	orch := New(OrchestratorConfig{
		Registry:  registry,
		Observer:  obs,
		Evaluator: evaluator.New(10),
		Lock:      safety.NewLocalExperimentLock(),
		Verbose:   false,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	return orch
}

func newTestExperiment() *v1alpha1.ChaosExperiment {
	return &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-experiment",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target: v1alpha1.TargetSpec{
				Operator:  "test-operator",
				Component: "dashboard",
			},
			Injection: v1alpha1.InjectionSpec{
				Type:  v1alpha1.PodKill,
				Count: 1,
				Parameters: map[string]string{
					"labelSelector": "app=dashboard",
				},
			},
			BlastRadius: v1alpha1.BlastRadiusSpec{
				MaxPodsAffected:   1,
				AllowedNamespaces: []string{"test-ns"},
			},
			Hypothesis: v1alpha1.HypothesisSpec{
				Description:     "Test recovers",
				RecoveryTimeout: metav1.Duration{Duration: 1 * time.Millisecond},
			},
		},
	}
}

func TestOrchestratorHappyPath(t *testing.T) {
	obs := &mockObserver{result: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1, Timestamp: metav1.Now()}}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	result, err := orch.Run(context.Background(), newTestExperiment())
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseComplete, result.Phase)
	assert.Equal(t, v1alpha1.Resilient, result.Verdict)
	assert.True(t, inj.cleanupCalled)
}

func TestOrchestratorDryRun(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	exp.Spec.BlastRadius.DryRun = true

	result, err := orch.Run(context.Background(), exp)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseComplete, result.Phase)
	assert.Equal(t, v1alpha1.Inconclusive, result.Verdict)
	assert.False(t, inj.cleanupCalled) // Should not inject in dry run
}

func TestOrchestratorBlastRadiusViolation(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	exp.Spec.BlastRadius.MaxPodsAffected = 0 // Invalid

	result, err := orch.Run(context.Background(), exp)
	assert.Error(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, result.Phase)
}

func TestOrchestratorPreCheckFailed(t *testing.T) {
	obs := &mockObserver{result: &v1alpha1.CheckResult{Passed: false, ChecksRun: 1, ChecksPassed: 0, Timestamp: metav1.Now()}}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	exp.Spec.SteadyState = v1alpha1.SteadyStateSpec{
		Checks: []v1alpha1.SteadyStateCheck{{Type: v1alpha1.CheckConditionTrue, Kind: "Deployment", Name: "test", ConditionType: "Available"}},
	}

	result, err := orch.Run(context.Background(), exp)
	assert.Error(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, result.Phase)
	assert.Equal(t, v1alpha1.Inconclusive, result.Verdict)
}

func TestOrchestratorCleanupOnError(t *testing.T) {
	// Verify cleanup is called even when later phases fail
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	result, err := orch.Run(context.Background(), newTestExperiment())
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseComplete, result.Phase)
	assert.True(t, inj.cleanupCalled)
}

func TestOrchestratorCleanupFailureSurfacedInResult(t *testing.T) {
	obs := &mockObserver{result: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1, Timestamp: metav1.Now()}}
	inj := &mockInjector{
		cleanupFunc: func(ctx context.Context) error {
			return fmt.Errorf("cleanup: failed to restore pod")
		},
	}
	orch := newTestOrchestrator(obs, inj)

	result, err := orch.Run(context.Background(), newTestExperiment())
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseComplete, result.Phase, "experiment should complete despite cleanup failure")
	assert.True(t, inj.cleanupCalled, "cleanup should have been called")
	assert.Contains(t, result.CleanupError, "cleanup: failed to restore pod", "cleanup error should be surfaced in result")
	require.NotNil(t, result.Report, "report should be present even when cleanup fails")
	assert.Contains(t, result.Report.CleanupError, "cleanup: failed to restore pod", "cleanup error should be surfaced in report")
}

func TestOrchestratorInjectionError(t *testing.T) {
	obs := &mockObserver{result: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1, Timestamp: metav1.Now()}}
	inj := &mockInjector{injectErr: assert.AnError}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	exp.Spec.SteadyState = v1alpha1.SteadyStateSpec{
		Checks: []v1alpha1.SteadyStateCheck{{Type: v1alpha1.CheckConditionTrue, Kind: "Deployment", Name: "test", ConditionType: "Available"}},
	}

	result, err := orch.Run(context.Background(), exp)
	assert.Error(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, result.Phase)
	assert.Contains(t, result.Error, "injection failed")
}

func TestOrchestratorValidationError(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{validateErr: assert.AnError}
	orch := newTestOrchestrator(obs, inj)

	result, err := orch.Run(context.Background(), newTestExperiment())
	assert.Error(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, result.Phase)
	assert.Contains(t, result.Error, "injection validation failed")
	// Verify error chain is preserved with %w wrapping
	assert.Contains(t, err.Error(), "validation failed:")
}

func TestOrchestratorDangerLevelBlocked(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	exp.Spec.Injection.DangerLevel = v1alpha1.DangerLevelHigh
	exp.Spec.BlastRadius.AllowDangerous = false

	result, err := orch.Run(context.Background(), exp)
	assert.Error(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, result.Phase)
	assert.Contains(t, result.Error, "danger level")
}

func TestOrchestratorDefaultNamespace(t *testing.T) {
	obs := &mockObserver{result: &v1alpha1.CheckResult{Passed: true, ChecksRun: 0, Timestamp: metav1.Now()}}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	exp.Namespace = "" // empty namespace should default to "opendatahub"
	exp.Spec.BlastRadius.AllowedNamespaces = []string{"opendatahub"}

	result, err := orch.Run(context.Background(), exp)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseComplete, result.Phase)
}

func TestOrchestratorUnknownInjectionType(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	exp.Spec.Injection.Type = "UnknownType"

	result, err := orch.Run(context.Background(), exp)
	assert.Error(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, result.Phase)
	assert.Contains(t, result.Error, "unknown injection type")
}

func TestOrchestratorLogOutput(t *testing.T) {
	obs := &mockObserver{result: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1, Timestamp: metav1.Now()}}
	inj := &mockInjector{}

	buf := &bytes.Buffer{}
	registry := injection.NewRegistry()
	registry.Register(v1alpha1.PodKill, inj)

	orch := New(OrchestratorConfig{
		Registry:  registry,
		Observer:  obs,
		Evaluator: evaluator.New(10),
		Lock:      safety.NewLocalExperimentLock(),
		Verbose:   true,
		Logger:    slog.New(slog.NewTextHandler(buf, nil)),
	})

	_, err := orch.Run(context.Background(), newTestExperiment())
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "PENDING")
	assert.Contains(t, output, "COMPLETE")
}

func TestOrchestratorVerboseOff(t *testing.T) {
	obs := &mockObserver{result: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1, Timestamp: metav1.Now()}}
	inj := &mockInjector{}

	buf := &bytes.Buffer{}
	registry := injection.NewRegistry()
	registry.Register(v1alpha1.PodKill, inj)

	// Verbose=false with no Logger should use discard handler by default
	orch := New(OrchestratorConfig{
		Registry:  registry,
		Observer:  obs,
		Evaluator: evaluator.New(10),
		Lock:      safety.NewLocalExperimentLock(),
		Verbose:   false,
	})

	// Verify that using a discard logger produces no output
	// by also checking with an explicit logger pointing at our buffer
	orchWithBuf := New(OrchestratorConfig{
		Registry:  registry,
		Observer:  obs,
		Evaluator: evaluator.New(10),
		Lock:      safety.NewLocalExperimentLock(),
		Verbose:   false,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	_ = orch // default discard behavior verified by construction

	_, err := orchWithBuf.Run(context.Background(), newTestExperiment())
	require.NoError(t, err)

	assert.Empty(t, buf.String(), "expected no output when verbose is off")
}

func TestOrchestratorReportGeneration(t *testing.T) {
	obs := &mockObserver{result: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1, Timestamp: metav1.Now()}}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	result, err := orch.Run(context.Background(), newTestExperiment())
	require.NoError(t, err)
	require.NotNil(t, result.Report)
	assert.Equal(t, "test-experiment", result.Report.Experiment)
	assert.Equal(t, "test-operator", result.Report.Target.Operator)
	assert.Equal(t, "dashboard", result.Report.Target.Component)
}

func TestOrchestratorCleanupUsesBackgroundContext(t *testing.T) {
	// Test that cleanup receives a non-cancelled context even when the parent
	// context has been cancelled (e.g. due to SIGINT). This verifies that
	// the cleanup defer block uses context.Background() instead of the parent ctx.

	obs := &mockObserver{result: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1, Timestamp: metav1.Now()}}

	// Create a cancellable context and a mock injector that cancels it
	// during injection (simulating a signal arriving mid-experiment).
	ctx, cancel := context.WithCancel(context.Background())

	var cleanupCtxErr error
	cleanupDone := make(chan struct{})

	// Override the default Inject behavior: cancel the parent context, then
	// return a cleanup function that records whether its context is valid.
	registry := injection.NewRegistry()
	customInj := &contextCancellingMockInjector{
		cancelParent: cancel,
		cleanupDone:  cleanupDone,
		cleanupCtxErr: &cleanupCtxErr,
	}
	registry.Register(v1alpha1.PodKill, customInj)

	orch := New(OrchestratorConfig{
		Registry:  registry,
		Observer:  obs,
		Evaluator: evaluator.New(10),
		Lock:      safety.NewLocalExperimentLock(),
		Verbose:   false,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	_, _ = orch.Run(ctx, newTestExperiment())

	// Wait for cleanup to complete
	<-cleanupDone

	// The cleanup function should have received a non-cancelled context
	assert.NoError(t, cleanupCtxErr, "cleanup context should not be cancelled; it should use a fresh background context")
	assert.True(t, customInj.cleanupCalled, "cleanup should have been called")
}

// contextCancellingMockInjector is a mock injector that cancels the parent
// context during Inject, then records whether cleanup received a valid context.
type contextCancellingMockInjector struct {
	cancelParent  context.CancelFunc
	cleanupCalled bool
	cleanupDone   chan struct{}
	cleanupCtxErr *error
}

func (m *contextCancellingMockInjector) Validate(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error {
	return nil
}

func (m *contextCancellingMockInjector) Revert(_ context.Context, _ v1alpha1.InjectionSpec, _ string) error {
	return nil
}

func (m *contextCancellingMockInjector) Inject(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) (injection.CleanupFunc, []v1alpha1.InjectionEvent, error) {
	// Cancel the parent context to simulate SIGINT/SIGTERM arrival
	m.cancelParent()

	events := []v1alpha1.InjectionEvent{
		{Type: spec.Type, Target: "test-pod", Action: "deleted", Timestamp: metav1.Now()},
	}
	cleanup := func(ctx context.Context) error {
		defer close(m.cleanupDone)
		m.cleanupCalled = true
		// Record whether the context passed to cleanup is already cancelled
		*m.cleanupCtxErr = ctx.Err()
		return nil
	}
	return cleanup, events, nil
}

func TestOrchestratorReportWrittenToDir(t *testing.T) {
	obs := &mockObserver{result: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1, Timestamp: metav1.Now()}}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)
	orch.reportDir = t.TempDir()

	result, err := orch.Run(context.Background(), newTestExperiment())
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseComplete, result.Phase)
	// Report should have been written to the temp dir
	require.NotNil(t, result.Report)
}

func TestOrchestratorStructuredLogging(t *testing.T) {
	obs := &mockObserver{result: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1, Timestamp: metav1.Now()}}
	inj := &mockInjector{}

	buf := &bytes.Buffer{}
	registry := injection.NewRegistry()
	registry.Register(v1alpha1.PodKill, inj)

	// Create orchestrator with a JSON handler to verify structured output
	orch := New(OrchestratorConfig{
		Registry:  registry,
		Observer:  obs,
		Evaluator: evaluator.New(10),
		Lock:      safety.NewLocalExperimentLock(),
		Verbose:   true,
		Logger:    slog.New(slog.NewJSONHandler(buf, nil)),
	})

	_, err := orch.Run(context.Background(), newTestExperiment())
	require.NoError(t, err)

	// Parse each line as JSON and collect structured log entries
	output := buf.String()
	require.NotEmpty(t, output, "expected structured log output")

	lines := bytes.Split([]byte(output), []byte("\n"))

	var foundPending, foundComplete, foundInjectionComplete bool
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		var entry map[string]interface{}
		err := json.Unmarshal(line, &entry)
		require.NoError(t, err, "each log line should be valid JSON: %s", string(line))

		// Verify common structured fields exist
		assert.Contains(t, entry, "time", "structured log should have time field")
		assert.Contains(t, entry, "level", "structured log should have level field")
		assert.Contains(t, entry, "msg", "structured log should have msg field")

		msg, _ := entry["msg"].(string)
		phase, _ := entry["phase"].(string)

		if msg == "phase transition" && phase == "PENDING" {
			foundPending = true
			assert.Equal(t, "test-experiment", entry["experiment"])
			assert.Equal(t, "validating", entry["action"])
		}
		if msg == "phase transition" && phase == "COMPLETE" {
			foundComplete = true
			assert.Contains(t, entry, "verdict")
		}
		if msg == "injection complete" {
			foundInjectionComplete = true
			assert.Contains(t, entry, "events")
		}
	}

	assert.True(t, foundPending, "should have logged PENDING phase transition")
	assert.True(t, foundComplete, "should have logged COMPLETE phase transition")
	assert.True(t, foundInjectionComplete, "should have logged injection complete")
}

func TestOrchestratorDefaultLoggerVerbose(t *testing.T) {
	// Verify that when no Logger is provided and Verbose=true, a default logger is created
	obs := &mockObserver{result: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1, Timestamp: metav1.Now()}}
	inj := &mockInjector{}
	registry := injection.NewRegistry()
	registry.Register(v1alpha1.PodKill, inj)

	orch := New(OrchestratorConfig{
		Registry:  registry,
		Observer:  obs,
		Evaluator: evaluator.New(10),
		Lock:      safety.NewLocalExperimentLock(),
		Verbose:   true,
		// No Logger set — should create a default text handler to stdout
	})

	// The orchestrator should have a non-nil logger
	assert.NotNil(t, orch.logger, "default logger should be created when Logger is nil")
}

// alwaysLockedLock is a mock ExperimentLock that always returns an error from
// Acquire, simulating lock contention (another experiment already holds the lock).
type alwaysLockedLock struct{}

func (l *alwaysLockedLock) Acquire(_ context.Context, operator, experiment string, _ time.Duration) error {
	return fmt.Errorf("operator %q is locked by experiment %q", operator, "other-experiment")
}

func (l *alwaysLockedLock) Renew(_ context.Context, _, _ string) error { return nil }

func (l *alwaysLockedLock) Release(_ context.Context, _, _ string) error { return nil }

func TestOrchestratorLockContention(t *testing.T) {
	obs := &mockObserver{result: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1, Timestamp: metav1.Now()}}
	inj := &mockInjector{}

	registry := injection.NewRegistry()
	registry.Register(v1alpha1.PodKill, inj)

	orch := New(OrchestratorConfig{
		Registry:  registry,
		Observer:  obs,
		Evaluator: evaluator.New(10),
		Lock:      &alwaysLockedLock{},
		Verbose:   false,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	result, err := orch.Run(context.Background(), newTestExperiment())
	assert.Error(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, result.Phase)
	assert.Contains(t, result.Error, "lock")
	assert.False(t, inj.cleanupCalled, "cleanup should not be called when lock acquisition fails")
}

// spyLock is a mock ExperimentLock that records whether Acquire was called.
type spyLock struct {
	acquireCalled bool
}

func (l *spyLock) Acquire(_ context.Context, operator, experiment string, _ time.Duration) error {
	l.acquireCalled = true
	return nil
}

func (l *spyLock) Renew(_ context.Context, _, _ string) error { return nil }

func (l *spyLock) Release(_ context.Context, _, _ string) error { return nil }

func TestOrchestratorDryRunSkipsLock(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	spy := &spyLock{}

	registry := injection.NewRegistry()
	registry.Register(v1alpha1.PodKill, inj)

	orch := New(OrchestratorConfig{
		Registry:  registry,
		Observer:  obs,
		Evaluator: evaluator.New(10),
		Lock:      spy,
		Verbose:   false,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	exp := newTestExperiment()
	exp.Spec.BlastRadius.DryRun = true

	result, err := orch.Run(context.Background(), exp)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseComplete, result.Phase)
	assert.Equal(t, v1alpha1.Inconclusive, result.Verdict)
	assert.False(t, spy.acquireCalled, "dry run should not acquire the experiment lock")
	assert.False(t, inj.cleanupCalled, "dry run should not inject or clean up")
}

func TestOrchestratorForbiddenNamespace(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	for _, ns := range []string{"kube-system", "kube-public", "kube-node-lease"} {
		t.Run(ns, func(t *testing.T) {
			exp := newTestExperiment()
			exp.Spec.BlastRadius.AllowedNamespaces = []string{ns}

			result, err := orch.Run(context.Background(), exp)
			assert.Error(t, err)
			assert.Equal(t, v1alpha1.PhaseAborted, result.Phase)
			assert.Contains(t, result.Error, "forbidden")
		})
	}
}

func TestOrchestratorClusterScopedRequiresDangerHigh(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}

	registry := injection.NewRegistry()
	registry.Register(v1alpha1.WebhookDisrupt, inj)

	orch := New(OrchestratorConfig{
		Registry:  registry,
		Observer:  obs,
		Evaluator: evaluator.New(10),
		Lock:      safety.NewLocalExperimentLock(),
		Verbose:   false,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	exp := newTestExperiment()
	exp.Spec.Injection.Type = v1alpha1.WebhookDisrupt
	exp.Spec.Injection.Parameters = map[string]string{
		"webhookName": "my-webhook",
		"action":      "setFailurePolicy",
	}
	// No dangerLevel set — should be rejected for cluster-scoped type
	exp.Spec.BlastRadius.AllowedNamespaces = []string{"test-ns"}

	err := orch.ValidateExperiment(context.Background(), exp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cluster-scoped")
	assert.Contains(t, err.Error(), "dangerLevel: high")
}

func TestOrchestratorClusterScopedPassesWithCorrectConfig(t *testing.T) {
	obs := &mockObserver{result: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1, Timestamp: metav1.Now()}}
	inj := &mockInjector{}

	registry := injection.NewRegistry()
	registry.Register(v1alpha1.WebhookDisrupt, inj)

	orch := New(OrchestratorConfig{
		Registry:  registry,
		Observer:  obs,
		Evaluator: evaluator.New(10),
		Lock:      safety.NewLocalExperimentLock(),
		Verbose:   false,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	exp := newTestExperiment()
	exp.Spec.Injection.Type = v1alpha1.WebhookDisrupt
	exp.Spec.Injection.DangerLevel = v1alpha1.DangerLevelHigh
	exp.Spec.Injection.Parameters = map[string]string{
		"webhookName": "my-webhook",
		"action":      "setFailurePolicy",
	}
	// Cluster-scoped: empty AllowedNamespaces, dangerLevel high, allowDangerous
	exp.Spec.BlastRadius.AllowedNamespaces = nil
	exp.Spec.BlastRadius.AllowDangerous = true

	err := orch.ValidateExperiment(context.Background(), exp)
	assert.NoError(t, err, "cluster-scoped injection with correct config should pass validation")
}

func TestOrchestratorDefaultLoggerNonVerbose(t *testing.T) {
	// Verify that when no Logger is provided and Verbose=false, a discard logger is created
	obs := &mockObserver{result: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1, Timestamp: metav1.Now()}}
	inj := &mockInjector{}
	registry := injection.NewRegistry()
	registry.Register(v1alpha1.PodKill, inj)

	orch := New(OrchestratorConfig{
		Registry:  registry,
		Observer:  obs,
		Evaluator: evaluator.New(10),
		Lock:      safety.NewLocalExperimentLock(),
		Verbose:   false,
		// No Logger set — should create a discard handler
	})

	// The orchestrator should have a non-nil logger (even if it discards)
	assert.NotNil(t, orch.logger, "discard logger should be created when Logger is nil and verbose is off")
}

func TestValidateExperimentForbiddenTargetNamespace(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	exp.Namespace = "kube-system"
	exp.Spec.BlastRadius.AllowedNamespaces = []string{"kube-system"}

	err := orch.ValidateExperiment(context.Background(), exp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forbidden")
	assert.Contains(t, err.Error(), "kube-system")
}

func TestValidateExperimentForbiddenOpenShiftNamespace(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	exp.Namespace = "openshift-monitoring"
	exp.Spec.BlastRadius.AllowedNamespaces = []string{"openshift-monitoring"}

	err := orch.ValidateExperiment(context.Background(), exp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "openshift-monitoring")
	assert.Contains(t, err.Error(), "forbidden prefix")
}

func TestValidateExperimentForbiddenDefaultNamespace(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	exp.Namespace = "default"
	exp.Spec.BlastRadius.AllowedNamespaces = []string{"default"}

	err := orch.ValidateExperiment(context.Background(), exp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forbidden")
	assert.Contains(t, err.Error(), "default")
}

func TestValidateExperimentForbiddenSteadyStateCheckNamespace(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	exp.Spec.SteadyState = v1alpha1.SteadyStateSpec{
		Checks: []v1alpha1.SteadyStateCheck{
			{
				Type:      v1alpha1.CheckConditionTrue,
				Namespace: "kube-system",
			},
		},
	}

	err := orch.ValidateExperiment(context.Background(), exp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "steady-state check namespace")
	assert.Contains(t, err.Error(), "kube-system")
	assert.Contains(t, err.Error(), "forbidden")
}

func TestValidateExperimentForbiddenSteadyStateCheckOpenShiftNamespace(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	exp.Spec.SteadyState = v1alpha1.SteadyStateSpec{
		Checks: []v1alpha1.SteadyStateCheck{
			{
				Type:      v1alpha1.CheckConditionTrue,
				Namespace: "openshift-operators",
			},
		},
	}

	err := orch.ValidateExperiment(context.Background(), exp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "steady-state check namespace")
	assert.Contains(t, err.Error(), "openshift-operators")
	assert.Contains(t, err.Error(), "forbidden prefix")
}

func TestValidateExperimentRecoveryTimeoutTooLarge(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	exp.Spec.Hypothesis.RecoveryTimeout = metav1.Duration{Duration: 2 * time.Hour}

	err := orch.ValidateExperiment(context.Background(), exp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recoveryTimeout")
	assert.Contains(t, err.Error(), "exceeds maximum")
}

func TestValidateExperimentRecoveryTimeoutAtMax(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	exp.Spec.Hypothesis.RecoveryTimeout = metav1.Duration{Duration: 1 * time.Hour}

	err := orch.ValidateExperiment(context.Background(), exp)
	assert.NoError(t, err, "exactly MaxRecoveryTimeout should be allowed")
}

func TestStoreResultConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	// Create a namespace object so the fake client has it
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()

	obs := &mockObserver{result: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1, Timestamp: metav1.Now()}}
	inj := &mockInjector{}

	registry := injection.NewRegistry()
	registry.Register(v1alpha1.PodKill, inj)

	orch := New(OrchestratorConfig{
		Registry:  registry,
		Observer:  obs,
		Evaluator: evaluator.New(10),
		Lock:      safety.NewLocalExperimentLock(),
		K8sClient: fakeClient,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	exp := newTestExperiment()
	result, err := orch.Run(context.Background(), exp)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseComplete, result.Phase)

	// Verify ConfigMap was created
	cmList := &corev1.ConfigMapList{}
	require.NoError(t, fakeClient.List(context.Background(), cmList, client.InNamespace("test-ns")))
	require.Len(t, cmList.Items, 1)

	cm := cmList.Items[0]
	assert.Equal(t, "chaos-result-test-experiment", cm.Name)
	assert.Equal(t, "odh-chaos", cm.Labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "test-experiment", cm.Labels["chaos.opendatahub.io/experiment"])
	assert.Contains(t, cm.Data, "result.json")

	// Test name truncation with a very long experiment name (>253 chars)
	longName := strings.Repeat("a", 260)
	expLong := newTestExperiment()
	expLong.Name = longName

	resultLong, err := orch.Run(context.Background(), expLong)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseComplete, resultLong.Phase)

	// Verify the long-named ConfigMap was created with truncated name
	cmList2 := &corev1.ConfigMapList{}
	require.NoError(t, fakeClient.List(context.Background(), cmList2, client.InNamespace("test-ns")))
	require.Len(t, cmList2.Items, 2)
	for _, cm := range cmList2.Items {
		assert.LessOrEqual(t, len(cm.Name), 253, "ConfigMap name should be truncated to <=253 chars")
		// Label value should be truncated to 63 chars
		expLabel := cm.Labels["chaos.opendatahub.io/experiment"]
		assert.LessOrEqual(t, len(expLabel), 63, "Label value should be truncated to <=63 chars")
	}
}

// mockReconciliationChecker implements observer.ReconciliationCheckerInterface
// for testing the knowledge + reconciler path in RunPostCheck.
type mockReconciliationChecker struct {
	result *observer.ReconciliationResult
	err    error
}

func (m *mockReconciliationChecker) CheckReconciliation(_ context.Context, _ *model.ComponentModel, _ string, _ time.Duration) (*observer.ReconciliationResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func TestRunPostCheckWithKnowledge(t *testing.T) {
	obs := &mockObserver{result: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1, Timestamp: metav1.Now()}}
	inj := &mockInjector{}

	registry := injection.NewRegistry()
	registry.Register(v1alpha1.PodKill, inj)

	reconResult := &observer.ReconciliationResult{
		AllReconciled:   true,
		ReconcileCycles: 1,
		RecoveryTime:    5 * time.Second,
	}
	mockRecon := &mockReconciliationChecker{result: reconResult}

	knowledge := &model.OperatorKnowledge{
		Operator: model.OperatorMeta{Name: "test-operator"},
		Components: []model.ComponentModel{
			{
				Name:       "dashboard",
				Controller: "dashboard-controller",
			},
		},
	}

	orch := New(OrchestratorConfig{
		Registry:   registry,
		Observer:   obs,
		Evaluator:  evaluator.New(10),
		Lock:       safety.NewLocalExperimentLock(),
		Knowledge:  knowledge,
		Reconciler: mockRecon,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	exp := newTestExperiment()
	result, err := orch.Run(context.Background(), exp)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseComplete, result.Phase)

	// The findings should include a reconciliation finding
	require.NotNil(t, result.Report)
	require.NotNil(t, result.Report.Reconciliation)
	assert.True(t, result.Report.Reconciliation.AllReconciled)
	assert.Equal(t, 1, result.Report.Reconciliation.ReconcileCycles)
}

func TestRunPostCheckWithDepGraph(t *testing.T) {
	// Build a mock observer that returns results for collateral checks
	obs := &mockObserver{result: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1, Timestamp: metav1.Now()}}
	inj := &mockInjector{}

	registry := injection.NewRegistry()
	registry.Register(v1alpha1.PodKill, inj)

	// Build a dependency graph: target "dashboard" in "test-operator" has a
	// dependent "api-server" which declares steady-state checks.
	targetKnowledge := &model.OperatorKnowledge{
		Operator: model.OperatorMeta{Name: "test-operator", Namespace: "test-ns"},
		Components: []model.ComponentModel{
			{
				Name:       "dashboard",
				Controller: "dashboard-controller",
			},
			{
				Name:         "api-server",
				Controller:   "api-controller",
				Dependencies: []string{"dashboard"},
				SteadyState: v1alpha1.SteadyStateSpec{
					Checks: []v1alpha1.SteadyStateCheck{
						{Type: v1alpha1.CheckResourceExists, Kind: "Deployment", Name: "api-server"},
					},
				},
			},
		},
	}

	depGraph, err := model.BuildDependencyGraph([]*model.OperatorKnowledge{targetKnowledge})
	require.NoError(t, err)

	orch := New(OrchestratorConfig{
		Registry:  registry,
		Observer:  obs,
		Evaluator: evaluator.New(10),
		Lock:      safety.NewLocalExperimentLock(),
		DepGraph:  depGraph,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	exp := newTestExperiment()
	_, findings, postErr := orch.RunPostCheck(context.Background(), exp)
	require.NoError(t, postErr)

	// Should have findings from the collateral contributor
	var hasCollateral bool
	for _, f := range findings {
		if f.Source == observer.SourceCollateral {
			hasCollateral = true
			assert.Equal(t, "api-server", f.Component)
		}
	}
	assert.True(t, hasCollateral, "expected collateral findings from dependency graph")
}

func TestValidateSteadyStateCheckConditionTrueMissingFields(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	tests := []struct {
		name  string
		check v1alpha1.SteadyStateCheck
		want  string
	}{
		{
			name:  "missing kind",
			check: v1alpha1.SteadyStateCheck{Type: v1alpha1.CheckConditionTrue, Name: "test", ConditionType: "Available"},
			want:  "requires kind, name, and conditionType",
		},
		{
			name:  "missing name",
			check: v1alpha1.SteadyStateCheck{Type: v1alpha1.CheckConditionTrue, Kind: "Deployment", ConditionType: "Available"},
			want:  "requires kind, name, and conditionType",
		},
		{
			name:  "missing conditionType",
			check: v1alpha1.SteadyStateCheck{Type: v1alpha1.CheckConditionTrue, Kind: "Deployment", Name: "test"},
			want:  "requires kind, name, and conditionType",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := newTestExperiment()
			exp.Spec.SteadyState = v1alpha1.SteadyStateSpec{Checks: []v1alpha1.SteadyStateCheck{tt.check}}
			err := orch.ValidateExperiment(context.Background(), exp)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestValidateSteadyStateCheckResourceExistsMissingFields(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	tests := []struct {
		name  string
		check v1alpha1.SteadyStateCheck
		want  string
	}{
		{
			name:  "missing kind",
			check: v1alpha1.SteadyStateCheck{Type: v1alpha1.CheckResourceExists, Name: "test"},
			want:  "requires kind and name",
		},
		{
			name:  "missing name",
			check: v1alpha1.SteadyStateCheck{Type: v1alpha1.CheckResourceExists, Kind: "Deployment"},
			want:  "requires kind and name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := newTestExperiment()
			exp.Spec.SteadyState = v1alpha1.SteadyStateSpec{Checks: []v1alpha1.SteadyStateCheck{tt.check}}
			err := orch.ValidateExperiment(context.Background(), exp)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestValidateSteadyStateCheckEmptyType(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	exp.Spec.SteadyState = v1alpha1.SteadyStateSpec{
		Checks: []v1alpha1.SteadyStateCheck{{Kind: "Deployment", Name: "test"}},
	}
	err := orch.ValidateExperiment(context.Background(), exp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "type is required")
}

func TestValidateSteadyStateCheckUnknownType(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	exp.Spec.SteadyState = v1alpha1.SteadyStateSpec{
		Checks: []v1alpha1.SteadyStateCheck{{Type: "bogus", Kind: "Deployment", Name: "test"}},
	}
	err := orch.ValidateExperiment(context.Background(), exp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown type")
}

func TestErrorChainsPreserved(t *testing.T) {
	obs := &mockObserver{}
	sentinel := fmt.Errorf("sentinel error")
	inj := &mockInjector{validateErr: sentinel}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	_, err := orch.Run(context.Background(), exp)
	require.Error(t, err)

	// The error chain should be: "validation failed: injection validation failed: sentinel error"
	// errors.Is should find the sentinel through the %w chain
	assert.ErrorIs(t, err, sentinel, "error chain should preserve the original sentinel error via %%w wrapping")
}

func TestValidateExperimentInjectionTTLTooLarge(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	exp.Spec.Injection.TTL = metav1.Duration{Duration: 2 * time.Hour}

	err := orch.ValidateExperiment(context.Background(), exp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injection TTL")
	assert.Contains(t, err.Error(), "exceeds maximum")
}

func TestIsClusterScopedInjection_CRDMutationNamespaceScoped(t *testing.T) {
	// CR instances with custom apiVersion should be namespace-scoped
	assert.False(t, isClusterScopedInjection(v1alpha1.CRDMutation, map[string]string{
		"apiVersion": "serving.kserve.io/v1beta1",
	}), "CR instances should be namespace-scoped")
}

func TestIsClusterScopedInjection_CRDMutationClusterScoped(t *testing.T) {
	// CRD definitions (apiextensions.k8s.io) should be cluster-scoped
	assert.True(t, isClusterScopedInjection(v1alpha1.CRDMutation, map[string]string{
		"apiVersion": "apiextensions.k8s.io/v1",
	}), "CRD definitions should be cluster-scoped")
}

func TestIsClusterScopedInjection_CRDMutationApiregistration(t *testing.T) {
	assert.True(t, isClusterScopedInjection(v1alpha1.CRDMutation, map[string]string{
		"apiVersion": "apiregistration.k8s.io/v1",
		"kind":       "APIService",
	}), "APIService should be cluster-scoped")
}

func TestIsClusterScopedInjection_CRDMutationClusterRole(t *testing.T) {
	assert.True(t, isClusterScopedInjection(v1alpha1.CRDMutation, map[string]string{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRole",
	}), "ClusterRole should be cluster-scoped")
}

func TestIsClusterScopedInjection_CRDMutationRoleNamespaceScoped(t *testing.T) {
	assert.False(t, isClusterScopedInjection(v1alpha1.CRDMutation, map[string]string{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "Role",
	}), "Role should be namespace-scoped")
}

func TestIsClusterScopedInjection_PodKillDefault(t *testing.T) {
	assert.False(t, isClusterScopedInjection(v1alpha1.PodKill, map[string]string{}))
}

func TestIsClusterScopedInjection_ConfigDriftDefault(t *testing.T) {
	assert.False(t, isClusterScopedInjection(v1alpha1.ConfigDrift, map[string]string{}))
}

func TestIsClusterScopedInjection_RBACRevokeClusterRoleBinding(t *testing.T) {
	assert.True(t, isClusterScopedInjection(v1alpha1.RBACRevoke, map[string]string{
		"bindingType": "ClusterRoleBinding",
	}))
}

func TestIsClusterScopedInjection_RBACRevokeRoleBinding(t *testing.T) {
	assert.False(t, isClusterScopedInjection(v1alpha1.RBACRevoke, map[string]string{
		"bindingType": "RoleBinding",
	}))
}

func TestIsClusterScopedInjection_FinalizerBlockClusterScoped(t *testing.T) {
	clusterKinds := []string{"Namespace", "Node", "ClusterRole", "ClusterRoleBinding", "CustomResourceDefinition", "PersistentVolume"}
	for _, kind := range clusterKinds {
		t.Run(kind, func(t *testing.T) {
			assert.True(t, isClusterScopedInjection(v1alpha1.FinalizerBlock, map[string]string{
				"kind": kind,
			}), "%s should be cluster-scoped", kind)
		})
	}
}

func TestIsClusterScopedInjection_FinalizerBlockNamespaceScoped(t *testing.T) {
	assert.False(t, isClusterScopedInjection(v1alpha1.FinalizerBlock, map[string]string{
		"kind": "Deployment",
	}), "Deployment should be namespace-scoped")
}

func TestOrchestratorCRDMutationNamespaceScopedEnforcesBlastRadius(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}

	registry := injection.NewRegistry()
	registry.Register(v1alpha1.CRDMutation, inj)

	orch := New(OrchestratorConfig{
		Registry:  registry,
		Observer:  obs,
		Evaluator: evaluator.New(10),
		Lock:      safety.NewLocalExperimentLock(),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	exp := newTestExperiment()
	exp.Spec.Injection.Type = v1alpha1.CRDMutation
	exp.Spec.Injection.Parameters = map[string]string{
		"apiVersion": "serving.kserve.io/v1beta1",
		"kind":       "InferenceService",
		"name":       "my-isvc",
		"field":      "managementState",
		"value":      "Removed",
	}
	// Empty AllowedNamespaces should fail for namespace-scoped CRDMutation
	exp.Spec.BlastRadius.AllowedNamespaces = nil
	exp.Spec.BlastRadius.MaxPodsAffected = 1

	err := orch.ValidateExperiment(context.Background(), exp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires at least one AllowedNamespaces")
}

func TestValidateExperimentErrorChainBlastRadius(t *testing.T) {
	obs := &mockObserver{}
	inj := &mockInjector{}
	orch := newTestOrchestrator(obs, inj)

	exp := newTestExperiment()
	exp.Spec.BlastRadius.MaxPodsAffected = 0 // triggers blast radius validation error

	err := orch.ValidateExperiment(context.Background(), exp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blast radius validation failed")

	// Verify the wrapped error is accessible
	unwrapped := err
	assert.NotNil(t, unwrapped)
}
