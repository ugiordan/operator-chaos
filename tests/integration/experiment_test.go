package integration

import (
	"context"
	"testing"
	"time"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/evaluator"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/injection"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/observer"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/orchestrator"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestPodKillExperimentE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// 1. Start envtest
	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	if err != nil {
		t.Skipf("skipping: envtest not available: %v", err)
	}
	defer testEnv.Stop() //nolint:errcheck

	k8sClient, err := client.New(cfg, client.Options{})
	require.NoError(t, err)

	ctx := context.Background()

	// 2. Create namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "chaos-test"},
	}
	require.NoError(t, k8sClient.Create(ctx, ns))

	// 3. Create a Deployment
	replicas := int32(1)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "chaos-test",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test-app"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test-app"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "test",
							Image: "busybox:latest",
						},
					},
				},
			},
		},
	}
	require.NoError(t, k8sClient.Create(ctx, deploy))

	// 4. Build orchestrator
	registry := injection.NewRegistry()
	registry.Register(v1alpha1.PodKill, injection.NewPodKillInjector(k8sClient))

	orch := orchestrator.New(orchestrator.OrchestratorConfig{
		Registry:  registry,
		Observer:  observer.NewKubernetesObserver(k8sClient),
		Evaluator: evaluator.New(10),
		Lock:      safety.NewLocalExperimentLock(),
		Verbose:   true,
	})

	// 5. Create experiment
	exp := &v1alpha1.ChaosExperiment{
		Metadata: v1alpha1.Metadata{
			Name:      "test-pod-kill",
			Namespace: "chaos-test",
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target: v1alpha1.TargetSpec{
				Operator:  "test-operator",
				Component: "test-app",
			},
			Injection: v1alpha1.InjectionSpec{
				Type:  v1alpha1.PodKill,
				Count: 1,
				Parameters: map[string]string{
					"labelSelector": "app=test-app",
				},
			},
			BlastRadius: v1alpha1.BlastRadiusSpec{
				MaxPodsAffected:   1,
				AllowedNamespaces: []string{"chaos-test"},
			},
			Hypothesis: v1alpha1.HypothesisSpec{
				Description:     "App recovers from pod kill",
				RecoveryTimeout: v1alpha1.Duration{Duration: 30 * time.Second},
			},
		},
	}

	// 6. Run the experiment
	// Note: In envtest there's no controller to create pods from the Deployment,
	// so the PodKill injector will fail to find pods. This is expected behavior --
	// we're testing that the orchestrator handles this gracefully.
	result, err := orch.Run(ctx, exp)

	// The experiment should error due to no pods
	// (envtest doesn't run controllers so no pods will be created from the Deployment)
	if err != nil {
		// Expected: injection fails because no pods exist
		assert.Contains(t, err.Error(), "no pods found")
		assert.Equal(t, v1alpha1.PhaseAborted, result.Phase)
	} else {
		// If somehow it completed, verify it has a verdict
		assert.NotEmpty(t, result.Verdict)
	}
}

func TestDryRunExperimentE2E(t *testing.T) {
	// This test doesn't need envtest at all -- dry run skips K8s interaction
	registry := injection.NewRegistry()
	registry.Register(v1alpha1.PodKill, injection.NewPodKillInjector(nil))

	orch := orchestrator.New(orchestrator.OrchestratorConfig{
		Registry:  registry,
		Observer:  observer.NewKubernetesObserver(nil),
		Evaluator: evaluator.New(10),
		Lock:      safety.NewLocalExperimentLock(),
		Verbose:   false,
	})

	exp := &v1alpha1.ChaosExperiment{
		Metadata: v1alpha1.Metadata{
			Name:      "dry-run-test",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target: v1alpha1.TargetSpec{
				Operator:  "test-operator",
				Component: "test-app",
			},
			Injection: v1alpha1.InjectionSpec{
				Type:  v1alpha1.PodKill,
				Count: 1,
				Parameters: map[string]string{
					"labelSelector": "app=test",
				},
			},
			BlastRadius: v1alpha1.BlastRadiusSpec{
				MaxPodsAffected:   1,
				AllowedNamespaces: []string{"test-ns"},
				DryRun:            true,
			},
			Hypothesis: v1alpha1.HypothesisSpec{
				Description:     "Dry run test",
				RecoveryTimeout: v1alpha1.Duration{Duration: 60 * time.Second},
			},
		},
	}

	result, err := orch.Run(context.Background(), exp)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.PhaseComplete, result.Phase)
	assert.Equal(t, v1alpha1.Inconclusive, result.Verdict)
}

func TestChaosClientIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Use fake client for integration test (no envtest needed)
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-config", Namespace: "default"},
		Data:       map[string]string{"key": "value"},
	}
	inner := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	// Test 1: ChaosClient with nil faults (passthrough)
	cc := sdk.NewChaosClient(inner, nil)
	obj := &corev1.ConfigMap{}
	err := cc.Get(context.Background(), client.ObjectKey{Name: "test-config", Namespace: "default"}, obj)
	assert.NoError(t, err)
	assert.Equal(t, "value", obj.Data["key"])

	// Test 2: ChaosClient with inactive faults (still passthrough)
	faults := &sdk.FaultConfig{Active: false}
	cc2 := sdk.NewChaosClient(inner, faults)
	obj2 := &corev1.ConfigMap{}
	err = cc2.Get(context.Background(), client.ObjectKey{Name: "test-config", Namespace: "default"}, obj2)
	assert.NoError(t, err)

	// Test 3: ChaosClient with active faults
	faults.Active = true
	faults.Faults = map[string]sdk.FaultSpec{
		"get": {ErrorRate: 1.0, Error: "chaos: api server error"},
	}
	cc3 := sdk.NewChaosClient(inner, faults)
	obj3 := &corev1.ConfigMap{}
	err = cc3.Get(context.Background(), client.ObjectKey{Name: "test-config", Namespace: "default"}, obj3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "chaos: api server error")
}

func TestWrapReconcilerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Test WrapReconciler with and without faults
	called := false
	inner := reconcile.Func(func(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
		called = true
		return reconcile.Result{}, nil
	})

	// Without faults
	wrapped := sdk.WrapReconciler(inner)
	_, err := wrapped.Reconcile(context.Background(), reconcile.Request{})
	assert.NoError(t, err)
	assert.True(t, called)

	// With active faults
	called = false
	faults := &sdk.FaultConfig{
		Active: true,
		Faults: map[string]sdk.FaultSpec{
			"reconcile": {ErrorRate: 1.0, Error: "chaos: reconcile failed"},
		},
	}
	wrapped2 := sdk.WrapReconciler(inner, sdk.WithFaultConfig(faults))
	_, err = wrapped2.Reconcile(context.Background(), reconcile.Request{})
	assert.Error(t, err)
	assert.False(t, called, "inner reconciler should not be called when fault fires")
}

func TestTestChaosHelperIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Test the test helper
	ch := sdk.NewForTest(t, "model-registry")
	ch.Activate("get", sdk.FaultSpec{ErrorRate: 1.0, Error: "test error"})

	err := ch.Config().MaybeInject("get")
	assert.Error(t, err)
	assert.Equal(t, "test error", err.Error())

	ch.Deactivate("get")
	err = ch.Config().MaybeInject("get")
	assert.Nil(t, err)
}
