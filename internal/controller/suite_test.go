//go:build integration

package controller

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/clock"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/evaluator"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/observer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func TestIntegrationHappyPath(t *testing.T) {
	// Set up logger for controller-runtime.
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Set up envtest with CRDs.
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	defer func() {
		err := testEnv.Stop()
		assert.NoError(t, err)
	}()

	// Use a dedicated scheme to avoid mutating the global scheme.Scheme.
	testScheme := runtime.NewScheme()
	err = v1alpha1.AddToScheme(testScheme)
	require.NoError(t, err)
	err = corev1.AddToScheme(testScheme)
	require.NoError(t, err)

	// Create manager.
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: testScheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	})
	require.NoError(t, err)

	// Create a mock orchestrator that succeeds at every phase.
	now := metav1.Now()
	orch := &mockOrchestrator{
		preCheckResult: &v1alpha1.CheckResult{
			Passed:       true,
			ChecksRun:    1,
			ChecksPassed: 1,
			Timestamp:    now,
		},
		injectEvents: []v1alpha1.InjectionEvent{
			{Timestamp: now, Type: v1alpha1.PodKill, Target: "pod/test-pod", Action: "killed"},
		},
		postCheckResult: &v1alpha1.CheckResult{
			Passed:       true,
			ChecksRun:    1,
			ChecksPassed: 1,
			Timestamp:    now,
		},
		postFindings: []observer.Finding{
			{Source: observer.SourceSteadyState, Passed: true, Checks: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1, Timestamp: now}},
		},
		evalResult: &evaluator.EvaluationResult{
			Verdict:    v1alpha1.Resilient,
			Confidence: "1/1 checks passed",
		},
	}

	// Mock lock that always succeeds.
	lock := &mockLock{}

	// Set up the reconciler with real clock (but short timeout).
	reconciler := &ChaosExperimentReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		Orchestrator: orch,
		Lock:         lock,
		Clock:        clock.RealClock{},
		Recorder:     record.NewFakeRecorder(100),
	}

	err = reconciler.SetupWithManager(mgr)
	require.NoError(t, err)

	// Start the manager in a goroutine.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := mgr.Start(ctx); err != nil {
			ctrl.Log.Error(err, "manager stopped with error")
		}
	}()

	// Wait for cache to sync.
	require.True(t, mgr.GetCache().WaitForCacheSync(ctx))

	// Create the opendatahub namespace.
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "opendatahub",
		},
	}
	err = mgr.GetClient().Create(ctx, ns)
	require.NoError(t, err)

	// Create a ChaosExperiment with a short RecoveryTimeout for fast testing.
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-exp",
			Namespace: "opendatahub",
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target: v1alpha1.TargetSpec{
				Operator:  "test-operator",
				Component: "test-component",
			},
			Hypothesis: v1alpha1.HypothesisSpec{
				Description:     "Test hypothesis",
				RecoveryTimeout: metav1.Duration{Duration: 1 * time.Second},
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

	err = mgr.GetClient().Create(ctx, exp)
	require.NoError(t, err)

	// Wait for the experiment to reach PhaseComplete.
	require.Eventually(t, func() bool {
		updated := &v1alpha1.ChaosExperiment{}
		err := mgr.GetClient().Get(ctx, types.NamespacedName{
			Name:      "test-exp",
			Namespace: "opendatahub",
		}, updated)
		if err != nil {
			t.Logf("Failed to get experiment: %v", err)
			return false
		}
		t.Logf("Current phase: %s", updated.Status.Phase)
		return updated.Status.Phase == v1alpha1.PhaseComplete
	}, 30*time.Second, 500*time.Millisecond, "Experiment should reach PhaseComplete")

	// Verify final status.
	final := &v1alpha1.ChaosExperiment{}
	err = mgr.GetClient().Get(ctx, types.NamespacedName{
		Name:      "test-exp",
		Namespace: "opendatahub",
	}, final)
	require.NoError(t, err)

	assert.Equal(t, v1alpha1.PhaseComplete, final.Status.Phase)
	assert.Equal(t, v1alpha1.Resilient, final.Status.Verdict)
	assert.NotNil(t, final.Status.StartTime)
	assert.NotNil(t, final.Status.EndTime)
	assert.NotNil(t, final.Status.EvaluationResult)
	assert.Equal(t, v1alpha1.Resilient, final.Status.EvaluationResult.Verdict)
	assert.Equal(t, "1/1 checks passed", final.Status.EvaluationResult.Confidence)

	// Verify all orchestrator phases were called.
	assert.True(t, orch.validateCalled)
	assert.True(t, orch.preCheckCalled)
	assert.True(t, orch.injectCalled)
	assert.True(t, orch.revertCalled)
	assert.True(t, orch.postCheckCalled)
	assert.True(t, orch.evaluateCalled)

	// Verify lock operations.
	assert.True(t, lock.acquireCalled)
	assert.True(t, lock.releaseCalled)
}

func TestIntegrationAbortOnSpecMutation(t *testing.T) {
	// Set up logger for controller-runtime.
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Set up envtest with CRDs.
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	defer func() {
		err := testEnv.Stop()
		assert.NoError(t, err)
	}()

	testScheme := runtime.NewScheme()
	err = v1alpha1.AddToScheme(testScheme)
	require.NoError(t, err)
	err = corev1.AddToScheme(testScheme)
	require.NoError(t, err)

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: testScheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	})
	require.NoError(t, err)

	now := metav1.Now()
	orch := &mockOrchestrator{
		preCheckResult: &v1alpha1.CheckResult{
			Passed:       true,
			ChecksRun:    1,
			ChecksPassed: 1,
			Timestamp:    now,
		},
		injectEvents: []v1alpha1.InjectionEvent{
			{Timestamp: now, Type: v1alpha1.PodKill, Target: "pod/test-pod", Action: "killed"},
		},
		postCheckResult: &v1alpha1.CheckResult{
			Passed:       true,
			ChecksRun:    1,
			ChecksPassed: 1,
			Timestamp:    now,
		},
		postFindings: []observer.Finding{
			{Source: observer.SourceSteadyState, Passed: true, Checks: &v1alpha1.CheckResult{Passed: true, ChecksRun: 1, ChecksPassed: 1, Timestamp: now}},
		},
		evalResult: &evaluator.EvaluationResult{
			Verdict:    v1alpha1.Resilient,
			Confidence: "1/1 checks passed",
		},
	}

	lock := &mockLock{}

	reconciler := &ChaosExperimentReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		Orchestrator: orch,
		Lock:         lock,
		Clock:        clock.RealClock{},
		Recorder:     record.NewFakeRecorder(100),
	}

	err = reconciler.SetupWithManager(mgr)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := mgr.Start(ctx); err != nil {
			ctrl.Log.Error(err, "manager stopped with error")
		}
	}()

	require.True(t, mgr.GetCache().WaitForCacheSync(ctx))

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "opendatahub-abort",
		},
	}
	err = mgr.GetClient().Create(ctx, ns)
	require.NoError(t, err)

	// Create a ChaosExperiment with a long RecoveryTimeout so it stays in Observing.
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-abort-exp",
			Namespace: "opendatahub-abort",
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target: v1alpha1.TargetSpec{
				Operator:  "test-operator-abort",
				Component: "test-component",
			},
			Hypothesis: v1alpha1.HypothesisSpec{
				Description:     "Test abort hypothesis",
				RecoveryTimeout: metav1.Duration{Duration: 10 * time.Minute},
			},
			Injection: v1alpha1.InjectionSpec{
				Type:  v1alpha1.PodKill,
				Count: 1,
			},
			BlastRadius: v1alpha1.BlastRadiusSpec{
				MaxPodsAffected:   1,
				AllowedNamespaces: []string{"opendatahub-abort"},
			},
		},
	}

	err = mgr.GetClient().Create(ctx, exp)
	require.NoError(t, err)

	key := types.NamespacedName{Name: "test-abort-exp", Namespace: "opendatahub-abort"}

	// Wait for the experiment to reach Observing phase (fault injected, waiting for timeout).
	require.Eventually(t, func() bool {
		updated := &v1alpha1.ChaosExperiment{}
		if err := mgr.GetClient().Get(ctx, key, updated); err != nil {
			return false
		}
		t.Logf("Current phase: %s", updated.Status.Phase)
		return updated.Status.Phase == v1alpha1.PhaseObserving
	}, 30*time.Second, 500*time.Millisecond, "Experiment should reach PhaseObserving")

	// Mutate the spec to trigger abort.
	current := &v1alpha1.ChaosExperiment{}
	err = mgr.GetClient().Get(ctx, key, current)
	require.NoError(t, err)

	current.Spec.Hypothesis.Description = "Mutated hypothesis to trigger abort"
	err = mgr.GetClient().Update(ctx, current)
	require.NoError(t, err)

	// Wait for the experiment to reach PhaseAborted.
	require.Eventually(t, func() bool {
		updated := &v1alpha1.ChaosExperiment{}
		if err := mgr.GetClient().Get(ctx, key, updated); err != nil {
			return false
		}
		t.Logf("Current phase after mutation: %s", updated.Status.Phase)
		return updated.Status.Phase == v1alpha1.PhaseAborted
	}, 30*time.Second, 500*time.Millisecond, "Experiment should reach PhaseAborted after spec mutation")

	// Verify final status.
	final := &v1alpha1.ChaosExperiment{}
	err = mgr.GetClient().Get(ctx, key, final)
	require.NoError(t, err)

	assert.Equal(t, v1alpha1.PhaseAborted, final.Status.Phase)
	assert.NotNil(t, final.Status.EndTime)
	assert.True(t, orch.revertCalled, "RevertFault should be called during abort from Observing")
}
