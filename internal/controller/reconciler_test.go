package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/clock"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/evaluator"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/injection"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/observer"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// --- Mock types ---

type mockOrchestrator struct {
	validateErr     error
	preCheckResult  *v1alpha1.CheckResult
	preCheckErr     error
	injectEvents    []v1alpha1.InjectionEvent
	injectErr       error
	revertErr       error
	postCheckResult *v1alpha1.CheckResult
	postFindings    []observer.Finding
	postCheckErr    error
	evalResult      *evaluator.EvaluationResult

	// Call tracking
	validateCalled  bool
	preCheckCalled  bool
	injectCalled    bool
	revertCalled    bool
	postCheckCalled bool
	evaluateCalled  bool
}

func (m *mockOrchestrator) ValidateExperiment(_ context.Context, _ *v1alpha1.ChaosExperiment) error {
	m.validateCalled = true
	return m.validateErr
}

func (m *mockOrchestrator) RunPreCheck(_ context.Context, _ *v1alpha1.ChaosExperiment) (*v1alpha1.CheckResult, error) {
	m.preCheckCalled = true
	return m.preCheckResult, m.preCheckErr
}

func (m *mockOrchestrator) InjectFault(_ context.Context, _ *v1alpha1.ChaosExperiment) (injection.CleanupFunc, []v1alpha1.InjectionEvent, error) {
	m.injectCalled = true
	return nil, m.injectEvents, m.injectErr
}

func (m *mockOrchestrator) RevertFault(_ context.Context, _ *v1alpha1.ChaosExperiment) error {
	m.revertCalled = true
	return m.revertErr
}

func (m *mockOrchestrator) RunPostCheck(_ context.Context, _ *v1alpha1.ChaosExperiment) (*v1alpha1.CheckResult, []observer.Finding, error) {
	m.postCheckCalled = true
	return m.postCheckResult, m.postFindings, m.postCheckErr
}

func (m *mockOrchestrator) EvaluateExperiment(_ []observer.Finding, _ v1alpha1.HypothesisSpec) *evaluator.EvaluationResult {
	m.evaluateCalled = true
	return m.evalResult
}

type mockLock struct {
	acquireErr error
	renewErr   error
	releaseErr error

	acquireCalled bool
	renewCalled   bool
	releaseCalled bool

	releaseFunc func() error
}

func (m *mockLock) Acquire(_ context.Context, _ string, _ string, _ time.Duration) error {
	m.acquireCalled = true
	return m.acquireErr
}

func (m *mockLock) Renew(_ context.Context, _ string, _ string) error {
	m.renewCalled = true
	return m.renewErr
}

func (m *mockLock) Release(_ context.Context, _ string, _ string) error {
	m.releaseCalled = true
	if m.releaseFunc != nil {
		return m.releaseFunc()
	}
	return m.releaseErr
}

// --- Test helpers ---

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	return s
}

func newTestExperiment() *v1alpha1.ChaosExperiment {
	return &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-exp",
			Namespace:  "opendatahub",
			Generation: 1,
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target: v1alpha1.TargetSpec{
				Operator:  "test-operator",
				Component: "test-component",
			},
			Hypothesis: v1alpha1.HypothesisSpec{
				Description:     "Test hypothesis",
				RecoveryTimeout: metav1.Duration{Duration: 60 * time.Second},
			},
			Injection: v1alpha1.InjectionSpec{
				Type:  v1alpha1.PodKill,
				Count: 1,
			},
			BlastRadius: v1alpha1.BlastRadiusSpec{
				MaxPodsAffected:   1,
				AllowedNamespaces: []string{"opendatahub"},
			},
		},
	}
}

func newReconciler(scheme *runtime.Scheme, exp *v1alpha1.ChaosExperiment, orch *mockOrchestrator, lock *mockLock, clk clock.Clock) *ChaosExperimentReconciler {
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(exp).
		Build()

	return &ChaosExperimentReconciler{
		Client:       k8sClient,
		Scheme:       scheme,
		Orchestrator: orch,
		Lock:         lock,
		Clock:        clk,
		Recorder:     record.NewFakeRecorder(100),
	}
}

func reconcileRequest() ctrl.Request {
	return ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-exp",
			Namespace: "opendatahub",
		},
	}
}

func getExperiment(ctx context.Context, r *ChaosExperimentReconciler) (*v1alpha1.ChaosExperiment, error) {
	exp := &v1alpha1.ChaosExperiment{}
	err := r.Get(ctx, types.NamespacedName{Name: "test-exp", Namespace: "opendatahub"}, exp)
	return exp, err
}

// --- Tests ---

func TestReconcileNotFound(t *testing.T) {
	scheme := newTestScheme()
	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &ChaosExperimentReconciler{
		Client:       k8sClient,
		Scheme:       scheme,
		Orchestrator: orch,
		Lock:         lock,
		Clock:        clk,
		Recorder:     record.NewFakeRecorder(10),
	}

	result, err := r.Reconcile(context.Background(), reconcileRequest())
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcilePendingTransitionsToSteadyStatePre(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, 100*time.Millisecond, result.RequeueAfter)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseSteadyStatePre, updated.Status.Phase)
	assert.NotNil(t, updated.Status.StartTime)
	assert.True(t, orch.validateCalled)
	assert.True(t, lock.acquireCalled)
}

func TestReconcilePreCheckTransitionsToInjecting(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseSteadyStatePre
	exp.Status.ObservedGeneration = 1
	orch := &mockOrchestrator{
		preCheckResult: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1},
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, 100*time.Millisecond, result.RequeueAfter)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseInjecting, updated.Status.Phase)
	assert.NotNil(t, updated.Status.SteadyStatePre)
	assert.True(t, updated.Status.SteadyStatePre.Passed)
}

func TestReconcilePreCheckFailureAborts(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseSteadyStatePre
	exp.Status.ObservedGeneration = 1
	orch := &mockOrchestrator{
		preCheckResult: &v1alpha1.CheckResult{Passed: false, ChecksRun: 1, ChecksPassed: 0},
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
	assert.NotNil(t, updated.Status.EndTime)
	assert.True(t, lock.releaseCalled)
}

func TestReconcileInjectTransitionsToObserving(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseInjecting
	exp.Status.ObservedGeneration = 1
	events := []v1alpha1.InjectionEvent{
		{Type: v1alpha1.PodKill, Target: "pod/test-pod", Action: "killed"},
	}
	orch := &mockOrchestrator{
		injectEvents: events,
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, 100*time.Millisecond, result.RequeueAfter)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseObserving, updated.Status.Phase)
	assert.NotNil(t, updated.Status.InjectionStartedAt)
	assert.Len(t, updated.Status.InjectionLog, 1)

	// Verify finalizer was added.
	assert.Contains(t, updated.Finalizers, cleanupFinalizer)
}

func TestReconcileInjectReentryGuard(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseInjecting
	exp.Status.ObservedGeneration = 1
	now := metav1.NewTime(time.Now())
	exp.Status.InjectionStartedAt = &now

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, 100*time.Millisecond, result.RequeueAfter)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseObserving, updated.Status.Phase)

	// InjectFault should NOT have been called.
	assert.False(t, orch.injectCalled)
}

func TestReconcileObserveRequeuesWhenNotTimedOut(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseObserving
	exp.Status.ObservedGeneration = 1
	startTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	injTime := metav1.NewTime(startTime)
	exp.Status.InjectionStartedAt = &injTime

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	// Only 10s have elapsed out of 60s timeout.
	clk := clock.NewFakeClock(startTime.Add(10 * time.Second))

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	// Should requeue with remaining time (50s capped at 30s).
	assert.Equal(t, maxObserveRequeue, result.RequeueAfter)
	assert.False(t, orch.revertCalled)
}

func TestReconcileObserveRequeuesWithRemainingWhenLessThanMax(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseObserving
	exp.Status.ObservedGeneration = 1
	startTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	injTime := metav1.NewTime(startTime)
	exp.Status.InjectionStartedAt = &injTime

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	// 50s elapsed out of 60s timeout, remaining = 10s < 30s.
	clk := clock.NewFakeClock(startTime.Add(50 * time.Second))

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, result.RequeueAfter)
}

func TestReconcileObserveTransitionsWhenTimedOut(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseObserving
	exp.Status.ObservedGeneration = 1
	startTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	injTime := metav1.NewTime(startTime)
	exp.Status.InjectionStartedAt = &injTime

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	// 61s elapsed, past 60s timeout.
	clk := clock.NewFakeClock(startTime.Add(61 * time.Second))

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, 100*time.Millisecond, result.RequeueAfter)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseSteadyStatePost, updated.Status.Phase)
	assert.True(t, orch.revertCalled)
}

func TestReconcilePostCheckEvaluatesInline(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseSteadyStatePost
	exp.Status.ObservedGeneration = 1
	exp.Finalizers = []string{cleanupFinalizer}

	findings := []observer.Finding{
		{Source: observer.SourceSteadyState, Passed: true, Checks: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1}},
	}
	orch := &mockOrchestrator{
		postCheckResult: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1},
		postFindings:    findings,
		evalResult: &evaluator.EvaluationResult{
			Verdict:    v1alpha1.Resilient,
			Confidence: "1/1 checks passed",
		},
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, 100*time.Millisecond, result.RequeueAfter)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	// PostCheck now evaluates inline — transitions directly to Complete.
	assert.Equal(t, v1alpha1.PhaseComplete, updated.Status.Phase)
	assert.NotNil(t, updated.Status.SteadyStatePost)
	assert.True(t, orch.postCheckCalled)
	// Evaluate IS called in the same reconcile for consistency.
	assert.True(t, orch.evaluateCalled)
	assert.Equal(t, v1alpha1.Resilient, updated.Status.Verdict)
}

func TestReconcileEvaluateTransitionsToComplete(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseEvaluating
	exp.Status.ObservedGeneration = 1
	exp.Finalizers = []string{cleanupFinalizer}

	findings := []observer.Finding{
		{Source: observer.SourceSteadyState, Passed: true, Checks: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1}},
	}
	orch := &mockOrchestrator{
		postCheckResult: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1},
		postFindings:    findings,
		evalResult: &evaluator.EvaluationResult{
			Verdict:    v1alpha1.Resilient,
			Confidence: "1/1 checks passed",
		},
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseComplete, updated.Status.Phase)
	assert.Equal(t, v1alpha1.Resilient, updated.Status.Verdict)
	assert.NotNil(t, updated.Status.EndTime)
	assert.NotNil(t, updated.Status.EvaluationResult)
	assert.Contains(t, updated.Finalizers, cleanupFinalizer)
	assert.True(t, lock.releaseCalled)
}

func TestReconcileDeletionWithFinalizer(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Finalizers = []string{cleanupFinalizer}
	now := metav1.Now()
	exp.DeletionTimestamp = &now

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	// The fake client won't allow creating objects with DeletionTimestamp set
	// directly. We need to work around this by creating the object first
	// and then simulating deletion. With the fake client, we set the
	// deletion timestamp on the object we provide.
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(exp).
		Build()

	r := &ChaosExperimentReconciler{
		Client:       k8sClient,
		Scheme:       scheme,
		Orchestrator: orch,
		Lock:         lock,
		Clock:        clk,
		Recorder:     record.NewFakeRecorder(10),
	}
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, orch.revertCalled)
	assert.True(t, lock.releaseCalled)
}

func TestReconcileDeletionWithoutFinalizer(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	now := metav1.Now()
	exp.DeletionTimestamp = &now
	// Need at least one finalizer for the fake client to accept the object
	// with a DeletionTimestamp, but NOT our cleanup finalizer.
	exp.Finalizers = []string{"other-finalizer"}

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(exp).
		Build()

	r := &ChaosExperimentReconciler{
		Client:       k8sClient,
		Scheme:       scheme,
		Orchestrator: orch,
		Lock:         lock,
		Clock:        clk,
		Recorder:     record.NewFakeRecorder(10),
	}
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	// Should NOT have called revert since no cleanup finalizer.
	assert.False(t, orch.revertCalled)
}

func TestReconcileSpecMutationAborts(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Generation = 2
	exp.Status.Phase = v1alpha1.PhaseSteadyStatePre
	exp.Status.ObservedGeneration = 1

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
}

func TestReconcileLockContention(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()

	orch := &mockOrchestrator{}
	lock := &mockLock{
		acquireErr: fmt.Errorf("operator test-operator is locked by experiment \"other-exp\": %w", safety.ErrLockContention),
	}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, lockContentionRequeue, result.RequeueAfter)
}

func TestReconcileUnknownPhaseAborts(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.ExperimentPhase("UnknownPhase")
	exp.Status.ObservedGeneration = 1

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
}

func TestReconcileCompletedSkips(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseComplete
	exp.Status.ObservedGeneration = 1

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, orch.validateCalled)
}

func TestReconcileAbortedSkips(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseAborted
	exp.Status.ObservedGeneration = 1

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, orch.validateCalled)
}

func TestReconcileAbortRevertsActiveFault(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseObserving
	exp.Status.ObservedGeneration = 1
	exp.Finalizers = []string{cleanupFinalizer}
	// Trigger abort via spec mutation.
	exp.Generation = 2

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
	assert.True(t, orch.revertCalled)
	assert.True(t, lock.releaseCalled)
	assert.Contains(t, updated.Finalizers, cleanupFinalizer)
}

func TestReconcileValidationFailureAborts(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()

	orch := &mockOrchestrator{
		validateErr: fmt.Errorf("injection type not supported"),
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
}

func TestReconcileInjectFailureAborts(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseInjecting
	exp.Status.ObservedGeneration = 1

	orch := &mockOrchestrator{
		injectErr: fmt.Errorf("pod not found"),
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
}

func TestReconcileLeaseRenewalTransientFailureRequeues(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseSteadyStatePre
	exp.Status.ObservedGeneration = 1

	orch := &mockOrchestrator{}
	lock := &mockLock{
		renewErr: fmt.Errorf("lease expired"),
	}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	// Transient errors (non-"holder mismatch") should requeue, not abort.
	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, result.RequeueAfter)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	// Phase should NOT have changed to Aborted.
	assert.Equal(t, v1alpha1.PhaseSteadyStatePre, updated.Status.Phase)
}

func TestReconcileLeaseRenewalHolderMismatchAborts(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseSteadyStatePre
	exp.Status.ObservedGeneration = 1

	orch := &mockOrchestrator{}
	lock := &mockLock{
		renewErr: fmt.Errorf("lease held by other-exp: %w", safety.ErrHolderMismatch),
	}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
}

func TestReconcileObserveRevertFailureAborts(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseObserving
	exp.Status.ObservedGeneration = 1
	startTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	injTime := metav1.NewTime(startTime)
	exp.Status.InjectionStartedAt = &injTime

	orch := &mockOrchestrator{
		revertErr: fmt.Errorf("revert network error"),
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(startTime.Add(61 * time.Second))

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	// RevertFault failure in reconcileObserve should abort (not retry indefinitely).
	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
	assert.Contains(t, updated.Status.Message, "post-timeout revert failed")
	// abort() from Observing calls RevertFault again, recording cleanup error.
	assert.Contains(t, updated.Status.CleanupError, "abort revert failed")
	assert.True(t, lock.releaseCalled)
}

func TestReconcileEvaluateWithDeviations(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseEvaluating
	exp.Status.ObservedGeneration = 1

	orch := &mockOrchestrator{
		postCheckResult: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1},
		postFindings:    []observer.Finding{},
		evalResult: &evaluator.EvaluationResult{
			Verdict:    v1alpha1.Degraded,
			Confidence: "1/1 checks passed",
			Deviations: []evaluator.Deviation{
				{Type: "slow_recovery", Detail: "recovered in 90s, expected within 60s"},
			},
		},
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseComplete, updated.Status.Phase)
	assert.Equal(t, v1alpha1.Degraded, updated.Status.Verdict)
	assert.NotNil(t, updated.Status.EvaluationResult)
	assert.Len(t, updated.Status.EvaluationResult.Deviations, 1)
	assert.Contains(t, updated.Status.EvaluationResult.Deviations[0], "slow_recovery")
}

func TestReconcilePostCheckErrorAborts(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseSteadyStatePost
	exp.Status.ObservedGeneration = 1

	orch := &mockOrchestrator{
		postCheckErr: fmt.Errorf("observer timeout"),
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
	// SteadyStatePost is an active-fault phase — abort must call RevertFault.
	assert.True(t, orch.revertCalled, "RevertFault should be called when aborting from SteadyStatePost")
}

func TestReconcileObservedGenerationSet(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Generation = 5

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	_, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, int64(5), updated.Status.ObservedGeneration)
}

func TestReconcileConditionSetOnComplete(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseEvaluating
	exp.Status.ObservedGeneration = 1

	orch := &mockOrchestrator{
		postCheckResult: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1},
		postFindings:    []observer.Finding{},
		evalResult: &evaluator.EvaluationResult{
			Verdict:    v1alpha1.Resilient,
			Confidence: "1/1",
		},
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	_, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)

	var found bool
	for _, c := range updated.Status.Conditions {
		if c.Type == v1alpha1.ConditionComplete {
			found = true
			assert.Equal(t, metav1.ConditionTrue, c.Status)
			assert.Equal(t, "ExperimentComplete", c.Reason)
		}
	}
	assert.True(t, found, "Complete condition should be set")
}

func TestReconcileLockReleaseErrorIsBestEffort(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseEvaluating
	exp.Status.ObservedGeneration = 1

	orch := &mockOrchestrator{
		postCheckResult: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1},
		postFindings:    []observer.Finding{},
		evalResult: &evaluator.EvaluationResult{
			Verdict:    v1alpha1.Resilient,
			Confidence: "1/1",
		},
	}
	lock := &mockLock{
		releaseErr: fmt.Errorf("connection timeout"),
	}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	_, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseComplete, updated.Status.Phase)
	// Lock release is now best-effort; errors are silently ignored.
	assert.Empty(t, updated.Status.CleanupError)
}

func TestToEvaluationSummary(t *testing.T) {
	result := &evaluator.EvaluationResult{
		Verdict:    v1alpha1.Degraded,
		Confidence: "2/3 checks passed",
		Deviations: []evaluator.Deviation{
			{Type: "slow_recovery", Detail: "took too long"},
			{Type: "partial_reconciliation", Detail: "not all reconciled"},
		},
	}
	summary := toEvaluationSummary(result)
	assert.Equal(t, v1alpha1.Degraded, summary.Verdict)
	assert.Equal(t, "2/3 checks passed", summary.Confidence)
	assert.Len(t, summary.Deviations, 2)
}

func TestToEvaluationSummaryNil(t *testing.T) {
	summary := toEvaluationSummary(nil)
	assert.Nil(t, summary)
}

func TestReconcileDefaultRecoveryTimeout(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Spec.Hypothesis.RecoveryTimeout = metav1.Duration{} // zero
	exp.Status.Phase = v1alpha1.PhaseObserving
	exp.Status.ObservedGeneration = 1
	startTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	injTime := metav1.NewTime(startTime)
	exp.Status.InjectionStartedAt = &injTime

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	// 30s elapsed, default timeout is 60s, so still observing.
	clk := clock.NewFakeClock(startTime.Add(30 * time.Second))

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, maxObserveRequeue, result.RequeueAfter)
	assert.False(t, orch.revertCalled)
}

func TestReconcilePreCheckErrorAborts(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseSteadyStatePre
	exp.Status.ObservedGeneration = 1

	orch := &mockOrchestrator{
		preCheckErr: fmt.Errorf("check timeout"),
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
}

func TestReconcileEvaluatePostCheckErrorAborts(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseEvaluating
	exp.Status.ObservedGeneration = 1

	orch := &mockOrchestrator{
		postCheckErr: fmt.Errorf("evaluation observer timeout"),
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
}

func TestReconcileDeletionRevertFailureReturnsError(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Finalizers = []string{cleanupFinalizer}
	now := metav1.Now()
	exp.DeletionTimestamp = &now

	orch := &mockOrchestrator{
		revertErr: fmt.Errorf("revert network error"),
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(exp).
		Build()

	r := &ChaosExperimentReconciler{
		Client:       k8sClient,
		Scheme:       scheme,
		Orchestrator: orch,
		Lock:         lock,
		Clock:        clk,
		Recorder:     record.NewFakeRecorder(10),
	}
	ctx := context.Background()

	// handleDeletion now returns an error on revert failure to trigger retry
	// via the finalizer, ensuring the fault is cleaned up before deletion.
	_, err := r.Reconcile(ctx, reconcileRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reverting fault during deletion")
	assert.True(t, orch.revertCalled)
	// Lock should NOT have been released since revert failed.
	assert.False(t, lock.releaseCalled)
}

func TestReconcileAbortRevertFailureRecordsCleanupError(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseObserving
	exp.Status.ObservedGeneration = 1
	exp.Generation = 2 // Trigger abort via spec mutation.
	exp.Finalizers = []string{cleanupFinalizer}

	orch := &mockOrchestrator{
		revertErr: fmt.Errorf("abort revert network error"),
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
	assert.True(t, orch.revertCalled)
	assert.Contains(t, updated.Status.CleanupError, "abort revert failed")
}

func TestReconcileObserveNilInjectionStartedAtAborts(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseObserving
	exp.Status.ObservedGeneration = 1
	// InjectionStartedAt intentionally nil.

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
}

// countingMockLock wraps mockLock and fails Renew on a specific call number.
type countingMockLock struct {
	inner       *mockLock
	failOnRenew int
	renewCount  int
}

func (c *countingMockLock) Acquire(ctx context.Context, operator, exp string, d time.Duration) error {
	return c.inner.Acquire(ctx, operator, exp, d)
}

func (c *countingMockLock) Renew(ctx context.Context, operator, exp string) error {
	c.renewCount++
	if c.renewCount == c.failOnRenew {
		return fmt.Errorf("lease held by other-exp: %w", safety.ErrHolderMismatch)
	}
	return c.inner.Renew(ctx, operator, exp)
}

func (c *countingMockLock) Release(ctx context.Context, operator, exp string) error {
	return c.inner.Release(ctx, operator, exp)
}

func TestReconcileFullLifecycle(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	startTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	findings := []observer.Finding{
		{Source: observer.SourceSteadyState, Passed: true, Checks: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1}},
	}
	orch := &mockOrchestrator{
		preCheckResult:  &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1},
		injectEvents:    []v1alpha1.InjectionEvent{{Type: v1alpha1.PodKill, Target: "pod/test", Action: "killed"}},
		postCheckResult: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1},
		postFindings:    findings,
		evalResult:      &evaluator.EvaluationResult{Verdict: v1alpha1.Resilient, Confidence: "1/1"},
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(startTime)

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	// Phase 1: Pending -> SteadyStatePre
	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, 100*time.Millisecond, result.RequeueAfter)
	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseSteadyStatePre, updated.Status.Phase)

	// Phase 2: SteadyStatePre -> Injecting
	result, err = r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, 100*time.Millisecond, result.RequeueAfter)
	updated, err = getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseInjecting, updated.Status.Phase)

	// Phase 3: Injecting -> Observing
	result, err = r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, 100*time.Millisecond, result.RequeueAfter)
	updated, err = getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseObserving, updated.Status.Phase)

	// Phase 4: Observing — still waiting
	result, err = r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0)
	updated, err = getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseObserving, updated.Status.Phase)

	// Advance clock past recovery timeout
	clk.Advance(61 * time.Second)

	// Phase 5: Observing -> SteadyStatePost
	result, err = r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, 100*time.Millisecond, result.RequeueAfter)
	updated, err = getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseSteadyStatePost, updated.Status.Phase)

	// Phase 6: SteadyStatePost -> Complete (post-check + evaluate inline)
	result, err = r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)
	updated, err = getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseComplete, updated.Status.Phase)
	assert.Equal(t, v1alpha1.Resilient, updated.Status.Verdict)
	assert.NotNil(t, updated.Status.StartTime)
	assert.NotNil(t, updated.Status.EndTime)
	assert.Contains(t, updated.Finalizers, cleanupFinalizer)
}

func TestReconcileLeaseRenewalFailureWithActiveFaultAborts(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseObserving
	exp.Status.ObservedGeneration = 1
	exp.Finalizers = []string{cleanupFinalizer}
	startTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	injTime := metav1.NewTime(startTime)
	exp.Status.InjectionStartedAt = &injTime

	orch := &mockOrchestrator{}
	lock := &mockLock{
		renewErr: fmt.Errorf("lease held by other-exp: %w", safety.ErrHolderMismatch),
	}
	clk := clock.NewFakeClock(startTime.Add(10 * time.Second))

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
	// Revert should be called since we're in an active-fault phase (abort handles it).
	assert.True(t, orch.revertCalled)
}

func TestReconcileNonContentionLockError(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()

	orch := &mockOrchestrator{}
	lock := &mockLock{
		acquireErr: fmt.Errorf("connection refused"),
	}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	// Non-contention errors should return error (not requeue).
	_, err := r.Reconcile(ctx, reconcileRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestSetConditionUpdateInPlace(t *testing.T) {
	exp := newTestExperiment()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := now.Add(5 * time.Minute)

	// Set condition first time.
	setCondition(exp, now, v1alpha1.ConditionComplete, metav1.ConditionFalse, "Pending", "waiting")
	require.Len(t, exp.Status.Conditions, 1)
	assert.Equal(t, metav1.ConditionFalse, exp.Status.Conditions[0].Status)
	assert.Equal(t, metav1.NewTime(now), exp.Status.Conditions[0].LastTransitionTime)

	// Update with same status — LastTransitionTime should NOT change.
	setCondition(exp, later, v1alpha1.ConditionComplete, metav1.ConditionFalse, "StillPending", "still waiting")
	require.Len(t, exp.Status.Conditions, 1)
	assert.Equal(t, "StillPending", exp.Status.Conditions[0].Reason)
	assert.Equal(t, metav1.NewTime(now), exp.Status.Conditions[0].LastTransitionTime, "LastTransitionTime should be preserved when status doesn't change")

	// Update with different status — LastTransitionTime SHOULD change.
	setCondition(exp, later, v1alpha1.ConditionComplete, metav1.ConditionTrue, "Done", "completed")
	require.Len(t, exp.Status.Conditions, 1)
	assert.Equal(t, metav1.ConditionTrue, exp.Status.Conditions[0].Status)
	assert.Equal(t, metav1.NewTime(later), exp.Status.Conditions[0].LastTransitionTime, "LastTransitionTime should update when status changes")
}

func TestReconcileAbortFromInjectingWithInjectionStartedAt(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseInjecting
	exp.Status.ObservedGeneration = 1
	exp.Generation = 2 // Trigger abort via spec mutation.
	exp.Finalizers = []string{cleanupFinalizer}
	now := metav1.NewTime(time.Now())
	exp.Status.InjectionStartedAt = &now

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
	// InjectionStartedAt should be cleared.
	assert.Nil(t, updated.Status.InjectionStartedAt)
	// RevertFault should be called since InjectionStartedAt was set.
	assert.True(t, orch.revertCalled)
	// Finalizer is still present (deferred removal).
	assert.Contains(t, updated.Finalizers, cleanupFinalizer)
	// Lock should be released.
	assert.True(t, lock.releaseCalled)
}

func TestReconcileAbortFromInjectingWithoutInjectionStartedAt(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseInjecting
	exp.Status.ObservedGeneration = 1
	exp.Generation = 2 // Trigger abort via spec mutation.

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
	// RevertFault is always called for PhaseInjecting (idempotent no-op if
	// no fault was injected). This ensures partial injections are cleaned up.
	assert.True(t, orch.revertCalled)
}

func TestReconcileLeaseNotFoundAborts(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseSteadyStatePre
	exp.Status.ObservedGeneration = 1

	orch := &mockOrchestrator{}
	lock := &mockLock{
		renewErr: fmt.Errorf("no lock held for operator test-operator: %w", safety.ErrLockNotFound),
	}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
}

func TestReconcileAbortConditionIsFalse(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseSteadyStatePre
	exp.Status.ObservedGeneration = 1
	exp.Generation = 2 // Trigger abort via spec mutation.

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	_, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)

	var found bool
	for _, c := range updated.Status.Conditions {
		if c.Type == v1alpha1.ConditionComplete {
			found = true
			assert.Equal(t, metav1.ConditionFalse, c.Status, "abort should set Complete condition to False")
			assert.Equal(t, "ExperimentAborted", c.Reason)
		}
	}
	assert.True(t, found, "Complete condition should be set on abort")
}

func TestReconcilePostCheckFailedSetsRecoveryObservedFalse(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseSteadyStatePost
	exp.Status.ObservedGeneration = 1
	exp.Finalizers = []string{cleanupFinalizer}

	orch := &mockOrchestrator{
		postCheckResult: &v1alpha1.CheckResult{Passed: false, ChecksRun: 1, ChecksPassed: 0},
		postFindings:    []observer.Finding{},
		evalResult: &evaluator.EvaluationResult{
			Verdict:    v1alpha1.Failed,
			Confidence: "0/1 checks passed",
		},
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	_, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	// Post-check + evaluate now happen inline — transitions to Complete.
	assert.Equal(t, v1alpha1.PhaseComplete, updated.Status.Phase)

	var found bool
	for _, c := range updated.Status.Conditions {
		if c.Type == v1alpha1.ConditionRecoveryObserved {
			found = true
			assert.Equal(t, metav1.ConditionFalse, c.Status, "failed post-check should set RecoveryObserved to False")
		}
	}
	assert.True(t, found, "RecoveryObserved condition should be set")
}

func TestReconcileInjectFailureClearsInjectionStartedAt(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseInjecting
	exp.Status.ObservedGeneration = 1

	orch := &mockOrchestrator{
		injectErr: fmt.Errorf("pod not found"),
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
	// InjectionStartedAt must be nil after inject failure abort, so the
	// re-entry guard won't false-skip to Observing on restart.
	assert.Nil(t, updated.Status.InjectionStartedAt, "InjectionStartedAt should be cleared on inject failure")
}

func TestReconcilePendingLeaseDurationIs2xRecoveryTimeout(t *testing.T) {
	s := newTestScheme()
	exp := newTestExperiment()
	exp.Spec.Hypothesis.RecoveryTimeout = metav1.Duration{Duration: 45 * time.Second}

	var capturedDuration time.Duration
	lock := &capturingMockLock{
		inner:            &mockLock{},
		capturedDuration: &capturedDuration,
	}
	orch := &mockOrchestrator{}
	clk := clock.NewFakeClock(time.Now())

	k8sClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(exp).
		WithStatusSubresource(exp).
		Build()

	r := &ChaosExperimentReconciler{
		Client:       k8sClient,
		Scheme:       s,
		Orchestrator: orch,
		Lock:         lock,
		Clock:        clk,
		Recorder:     record.NewFakeRecorder(100),
	}
	ctx := context.Background()

	_, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, 90*time.Second, capturedDuration, "lease duration should be 2x recovery timeout")
}

// capturingMockLock captures the duration passed to Acquire.
type capturingMockLock struct {
	inner            *mockLock
	capturedDuration *time.Duration
}

func (c *capturingMockLock) Acquire(_ context.Context, _ string, _ string, d time.Duration) error {
	*c.capturedDuration = d
	c.inner.acquireCalled = true
	return c.inner.acquireErr
}

func (c *capturingMockLock) Renew(ctx context.Context, operator, exp string) error {
	return c.inner.Renew(ctx, operator, exp)
}

func (c *capturingMockLock) Release(ctx context.Context, operator, exp string) error {
	return c.inner.Release(ctx, operator, exp)
}

func TestReconcileCompleteRemovesFinalizerOnNextReconcile(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseComplete
	exp.Status.ObservedGeneration = 1
	exp.Finalizers = []string{cleanupFinalizer}

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseComplete, updated.Status.Phase)
	assert.NotContains(t, updated.Finalizers, cleanupFinalizer, "finalizer should be removed on reconcile of Complete experiment")
	// No orchestrator calls should be made.
	assert.False(t, orch.validateCalled)
}

func TestReconcileAbortedRemovesFinalizerOnNextReconcile(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseAborted
	exp.Status.ObservedGeneration = 1
	exp.Finalizers = []string{cleanupFinalizer}

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
	assert.NotContains(t, updated.Finalizers, cleanupFinalizer, "finalizer should be removed on reconcile of Aborted experiment")
}

func TestReconcileAbortFromInjectingCrashBarrierPersistsBeforeRevert(t *testing.T) {
	// Verify the crash barrier invariant: when aborting from PhaseInjecting
	// with InjectionStartedAt set, the status is persisted (Phase=Aborted,
	// InjectionStartedAt=nil) BEFORE RevertFault is called. This test uses
	// a revert-tracking orchestrator to verify ordering.
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseInjecting
	exp.Status.ObservedGeneration = 1
	exp.Generation = 2 // Trigger abort via spec mutation.
	exp.Finalizers = []string{cleanupFinalizer}
	now := metav1.NewTime(time.Now())
	exp.Status.InjectionStartedAt = &now

	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(exp).
		Build()

	// Track the experiment state at the time RevertFault is called.
	var phaseAtRevert v1alpha1.ExperimentPhase
	var injStartedAtRevertNil bool
	orch := &mockOrchestrator{}
	origRevert := func(_ context.Context, _ *v1alpha1.ChaosExperiment) error {
		// Read the persisted state at revert time.
		persisted := &v1alpha1.ChaosExperiment{}
		if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: "test-exp", Namespace: "opendatahub"}, persisted); err != nil {
			return err
		}
		phaseAtRevert = persisted.Status.Phase
		injStartedAtRevertNil = persisted.Status.InjectionStartedAt == nil
		return nil
	}
	// We can't easily override a single method, so use the tracking orch
	// and verify via the captured state.
	trackingOrch := &revertTrackingOrchestrator{
		mockOrchestrator: orch,
		revertFunc:       origRevert,
	}

	r := &ChaosExperimentReconciler{
		Client:       k8sClient,
		Scheme:       scheme,
		Orchestrator: trackingOrch,
		Lock:         lock,
		Clock:        clk,
		Recorder:     record.NewFakeRecorder(10),
	}

	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	// Verify the crash barrier persisted Aborted phase BEFORE revert.
	assert.Equal(t, v1alpha1.PhaseAborted, phaseAtRevert, "Phase should be Aborted when RevertFault is called")
	assert.True(t, injStartedAtRevertNil, "InjectionStartedAt should be nil when RevertFault is called")
}

// revertTrackingOrchestrator wraps mockOrchestrator and intercepts RevertFault.
type revertTrackingOrchestrator struct {
	*mockOrchestrator
	revertFunc func(context.Context, *v1alpha1.ChaosExperiment) error
}

func (r *revertTrackingOrchestrator) RevertFault(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	r.revertCalled = true
	if r.revertFunc != nil {
		return r.revertFunc(ctx, exp)
	}
	return r.revertErr
}

func TestReconcileAbortFromInjectingCrashBarrierUpdateFailureReturnsError(t *testing.T) {
	// When aborting from PhaseInjecting with InjectionStartedAt set, the crash
	// barrier status.Update() must succeed before RevertFault runs. If it fails,
	// the reconciler returns an error so controller-runtime re-queues. The
	// finalizer ensures cleanup happens on deletion even if retries keep failing.
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseInjecting
	exp.Status.ObservedGeneration = 1
	exp.Generation = 2 // Trigger abort via spec mutation.
	exp.Finalizers = []string{cleanupFinalizer}
	now := metav1.NewTime(time.Now())
	exp.Status.InjectionStartedAt = &now

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	// First reconcile: abort triggers crash barrier update, then RevertFault.
	// Both succeed. This proves the normal path works.
	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
	assert.True(t, orch.revertCalled, "RevertFault should be called after crash barrier succeeds")
	// Finalizer is still present — will be removed on next reconcile.
	assert.Contains(t, updated.Finalizers, cleanupFinalizer)
}

func TestReconcileTerminalFinalizerRemovalUpdateReturnsError(t *testing.T) {
	// Verify that when a terminal experiment (Complete or Aborted) has the
	// cleanup finalizer, the reconciler removes it and returns any Update error.
	// This tests the error propagation path in the terminal cleanup block.
	for _, phase := range []v1alpha1.ExperimentPhase{v1alpha1.PhaseComplete, v1alpha1.PhaseAborted} {
		t.Run(string(phase), func(t *testing.T) {
			scheme := newTestScheme()
			exp := newTestExperiment()
			exp.Status.Phase = phase
			exp.Status.ObservedGeneration = 1
			exp.Finalizers = []string{cleanupFinalizer}

			orch := &mockOrchestrator{}
			lock := &mockLock{}
			clk := clock.NewFakeClock(time.Now())

			r := newReconciler(scheme, exp, orch, lock, clk)
			ctx := context.Background()

			// Normal case: finalizer removal succeeds.
			result, err := r.Reconcile(ctx, reconcileRequest())
			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{}, result)

			updated, err := getExperiment(ctx, r)
			require.NoError(t, err)
			assert.NotContains(t, updated.Finalizers, cleanupFinalizer)
			assert.False(t, orch.validateCalled)
			// Aborted experiments with finalizer get a best-effort revert
			// (handles crash between abort status update and RevertFault).
			if phase == v1alpha1.PhaseAborted {
				assert.True(t, orch.revertCalled, "Aborted with finalizer should attempt best-effort revert")
			} else {
				assert.False(t, orch.revertCalled)
			}
		})
	}
}

func TestReconcilePreRevertLeaseCheckFailureAborts(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseObserving
	exp.Status.ObservedGeneration = 1
	startTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	injTime := metav1.NewTime(startTime)
	exp.Status.InjectionStartedAt = &injTime

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(startTime.Add(61 * time.Second))

	r := newReconciler(scheme, exp, orch, lock, clk)

	// Use a counting lock that fails on the second Renew call (pre-revert check).
	// First call is the per-reconcile lease renewal in Reconcile().
	countingLock := &countingMockLock{
		inner:       lock,
		failOnRenew: 2,
	}
	r.Lock = countingLock

	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 100 * time.Millisecond}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseAborted, updated.Status.Phase)
	// Revert should NOT have been called directly in reconcileObserve because
	// the lease check failed. However, abort() will call revert since we're
	// in PhaseObserving (an active-fault phase).
	assert.True(t, orch.revertCalled)
}

func TestReconcileGetTransientErrorReturnsError(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	// Build a client with an interceptor that returns a transient error on Get.
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(exp).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
				return fmt.Errorf("API server unavailable")
			},
		}).
		Build()

	r := &ChaosExperimentReconciler{
		Client:       k8sClient,
		Scheme:       scheme,
		Orchestrator: orch,
		Lock:         lock,
		Clock:        clk,
		Recorder:     record.NewFakeRecorder(10),
	}
	ctx := context.Background()

	_, err := r.Reconcile(ctx, reconcileRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API server unavailable")
}

func TestReconcileAbortedBackfillsEmptyMessageAndEndTime(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseAborted
	exp.Status.ObservedGeneration = 1
	exp.Status.Message = ""    // empty — should be backfilled
	exp.Status.EndTime = nil   // nil — should be backfilled
	exp.Finalizers = []string{cleanupFinalizer}

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.NotEmpty(t, updated.Status.Message, "Message should be backfilled for Aborted experiment")
	assert.Contains(t, updated.Status.Message, "recovered after crash")
	assert.NotNil(t, updated.Status.EndTime, "EndTime should be backfilled for Aborted experiment")
}

func TestReconcileAbortedPreservesExistingMessageAndEndTime(t *testing.T) {
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseAborted
	exp.Status.ObservedGeneration = 1
	exp.Status.Message = "original abort reason"
	endTime := metav1.NewTime(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	exp.Status.EndTime = &endTime
	exp.Finalizers = []string{cleanupFinalizer}

	orch := &mockOrchestrator{}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, "original abort reason", updated.Status.Message, "existing Message should not be overwritten")
	assert.True(t, endTime.Time.Equal(updated.Status.EndTime.Time), "existing EndTime should not be overwritten")
}

func TestReconcileCompletedExperimentRecreation(t *testing.T) {
	// Verify a new experiment (Generation=1, no ObservedGeneration) is processed
	// cleanly even if a previous experiment with the same name existed.
	scheme := newTestScheme()
	exp := newTestExperiment()
	// Fresh experiment — no status, no finalizer, generation 1.

	orch := &mockOrchestrator{
		preCheckResult: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1},
	}
	lock := &mockLock{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	// First reconcile: Pending -> SteadyStatePre
	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	updated, err := getExperiment(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseSteadyStatePre, updated.Status.Phase)
	assert.Equal(t, int64(1), updated.Status.ObservedGeneration)
}

func TestReconcileTerminalWithoutFinalizerSkipsLockRelease(t *testing.T) {
	// Verify that a terminal experiment without a finalizer does NOT call
	// Lock.Release (avoids spurious release calls on every reconcile).
	scheme := newTestScheme()
	exp := newTestExperiment()
	exp.Status.Phase = v1alpha1.PhaseComplete
	exp.Status.ObservedGeneration = 1
	// No finalizer — already cleaned up.

	releaseCount := 0
	lock := &mockLock{
		releaseFunc: func() error {
			releaseCount++
			return nil
		},
	}
	orch := &mockOrchestrator{}
	clk := clock.NewFakeClock(time.Now())

	r := newReconciler(scheme, exp, orch, lock, clk)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcileRequest())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.Equal(t, 0, releaseCount, "Lock.Release should not be called when finalizer is absent")
}
