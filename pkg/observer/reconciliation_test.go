package observer

import (
	"context"
	"testing"
	"time"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewReconciliationChecker(t *testing.T) {
	rc := NewReconciliationChecker(nil)
	assert.NotNil(t, rc)
}

func TestReconciliationResultDefaults(t *testing.T) {
	result := &ReconciliationResult{}
	assert.False(t, result.AllReconciled)
	assert.Equal(t, 0, result.ReconcileCycles)
}

func TestResourceCheckResult(t *testing.T) {
	r := ResourceCheckResult{
		Kind:       "Deployment",
		Name:       "test-deploy",
		Namespace:  "test-ns",
		Reconciled: true,
		Details:    "exists",
	}
	assert.True(t, r.Reconciled)
	assert.Equal(t, "Deployment", r.Kind)
}

func TestCheckReconciliationRespectsContextCancellation(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	rc := NewReconciliationChecker(fakeClient)

	// Create a component with a managed resource that won't exist,
	// so the checker will loop and try to poll again.
	component := &model.ComponentModel{
		Name: "test-component",
		ManagedResources: []model.ManagedResource{
			{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "nonexistent",
				Namespace:  "default",
			},
		},
	}

	// Create a context that is already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	result, err := rc.CheckReconciliation(ctx, component, "default", 30*time.Second)
	elapsed := time.Since(start)

	// Should return quickly (well under the 30s timeout) with a context error
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, result.AllReconciled)
	// Should complete in under 1 second since context was already cancelled
	assert.Less(t, elapsed, 1*time.Second, "should return promptly when context is cancelled")
}

func TestCheckReconciliation_EmptyResources(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	rc := NewReconciliationChecker(fakeClient)

	component := &model.ComponentModel{
		Name:             "empty-component",
		ManagedResources: []model.ManagedResource{},
	}

	result, err := rc.CheckReconciliation(context.Background(), component, "default", 100*time.Millisecond)
	require.NoError(t, err)
	assert.True(t, result.AllReconciled, "no managed resources means all reconciled")
	assert.Equal(t, 1, result.ReconcileCycles)
	assert.Empty(t, result.Resources)
}

func TestCheckReconciliation_ResourceExists(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"})
	obj.SetName("my-config")
	obj.SetNamespace("test-ns")
	// Set an owner reference that matches
	obj.SetOwnerReferences([]metav1.OwnerReference{
		{
			Kind: "Operator",
			Name: "test-operator",
		},
	})

	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(obj).Build()
	rc := NewReconciliationChecker(fakeClient)

	component := &model.ComponentModel{
		Name: "test-component",
		ManagedResources: []model.ManagedResource{
			{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "my-config",
				Namespace:  "test-ns",
				OwnerRef:   "Operator",
			},
		},
	}

	result, err := rc.CheckReconciliation(context.Background(), component, "default", 100*time.Millisecond)
	require.NoError(t, err)
	assert.True(t, result.AllReconciled)
	assert.Equal(t, 1, result.ReconcileCycles)
	require.Len(t, result.Resources, 1)
	assert.True(t, result.Resources[0].Reconciled)
	assert.Equal(t, "exists", result.Resources[0].Details)
}

func TestCheckReconciliation_ResourceNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	rc := NewReconciliationChecker(fakeClient)

	component := &model.ComponentModel{
		Name: "test-component",
		ManagedResources: []model.ManagedResource{
			{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "nonexistent",
				Namespace:  "test-ns",
			},
		},
	}

	// Use a very short timeout so the polling loop exits quickly
	result, err := rc.CheckReconciliation(context.Background(), component, "default", 100*time.Millisecond)
	require.NoError(t, err)
	assert.False(t, result.AllReconciled)
	assert.GreaterOrEqual(t, result.ReconcileCycles, 1)
	// After timeout, resources array from last cycle should contain the not-found entry
	// But note: resources are reset between cycles, so the last cycle's result is there
	// unless allGood was true (it won't be here).
	// After the loop exits, result.Resources may be nil because they're reset at the end
	// of each non-successful cycle. Let's verify the overall result.
	assert.False(t, result.AllReconciled)
}

func TestCheckReconciliation_MissingOwnerRef(t *testing.T) {
	// Resource exists but does NOT have the expected owner reference
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"})
	obj.SetName("my-config")
	obj.SetNamespace("test-ns")
	// No owner references set

	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(obj).Build()
	rc := NewReconciliationChecker(fakeClient)

	component := &model.ComponentModel{
		Name: "test-component",
		ManagedResources: []model.ManagedResource{
			{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "my-config",
				Namespace:  "test-ns",
				OwnerRef:   "Operator",
			},
		},
	}

	// Short timeout so it doesn't loop too long
	result, err := rc.CheckReconciliation(context.Background(), component, "default", 100*time.Millisecond)
	require.NoError(t, err)
	assert.False(t, result.AllReconciled, "missing owner ref means not reconciled")
}

func TestCheckReconciliation_ExpectedSpecMatch(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
	obj.SetName("my-deploy")
	obj.SetNamespace("test-ns")
	obj.Object["spec"] = map[string]interface{}{
		"replicas": int64(3),
	}

	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(obj).Build()
	rc := NewReconciliationChecker(fakeClient)

	component := &model.ComponentModel{
		Name: "test-component",
		ManagedResources: []model.ManagedResource{
			{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "my-deploy",
				Namespace:  "test-ns",
				ExpectedSpec: map[string]interface{}{
					"replicas": int64(3),
				},
			},
		},
	}

	result, err := rc.CheckReconciliation(context.Background(), component, "default", 100*time.Millisecond)
	require.NoError(t, err)
	assert.True(t, result.AllReconciled)
	require.Len(t, result.Resources, 1)
	assert.True(t, result.Resources[0].Reconciled)
}

func TestCheckReconciliation_ExpectedSpecMismatch(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
	obj.SetName("my-deploy")
	obj.SetNamespace("test-ns")
	obj.Object["spec"] = map[string]interface{}{
		"replicas": int64(1),
	}

	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(obj).Build()
	rc := NewReconciliationChecker(fakeClient)

	component := &model.ComponentModel{
		Name: "test-component",
		ManagedResources: []model.ManagedResource{
			{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "my-deploy",
				Namespace:  "test-ns",
				ExpectedSpec: map[string]interface{}{
					"replicas": int64(3),
				},
			},
		},
	}

	result, err := rc.CheckReconciliation(context.Background(), component, "default", 100*time.Millisecond)
	require.NoError(t, err)
	assert.False(t, result.AllReconciled, "spec mismatch means not reconciled")
}

func TestCheckReconciliation_ExpectedSpecFieldNotFound(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
	obj.SetName("my-deploy")
	obj.SetNamespace("test-ns")
	// spec exists but missing the expected field
	obj.Object["spec"] = map[string]interface{}{}

	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(obj).Build()
	rc := NewReconciliationChecker(fakeClient)

	component := &model.ComponentModel{
		Name: "test-component",
		ManagedResources: []model.ManagedResource{
			{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "my-deploy",
				Namespace:  "test-ns",
				ExpectedSpec: map[string]interface{}{
					"replicas": int64(3),
				},
			},
		},
	}

	result, err := rc.CheckReconciliation(context.Background(), component, "default", 100*time.Millisecond)
	require.NoError(t, err)
	assert.False(t, result.AllReconciled, "missing spec field means not reconciled")
}

func TestCheckReconciliation_NamespaceFallback(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"})
	obj.SetName("my-config")
	obj.SetNamespace("fallback-ns")

	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(obj).Build()
	rc := NewReconciliationChecker(fakeClient)

	component := &model.ComponentModel{
		Name: "test-component",
		ManagedResources: []model.ManagedResource{
			{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "my-config",
				Namespace:  "", // empty -> use fallback namespace
			},
		},
	}

	result, err := rc.CheckReconciliation(context.Background(), component, "fallback-ns", 100*time.Millisecond)
	require.NoError(t, err)
	assert.True(t, result.AllReconciled, "should find resource using fallback namespace")
}

func TestCheckReconciliation_ContextTimeout(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	rc := NewReconciliationChecker(fakeClient)

	component := &model.ComponentModel{
		Name: "test-component",
		ManagedResources: []model.ManagedResource{
			{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "nonexistent",
				Namespace:  "test-ns",
			},
		},
	}

	// Use a very short context timeout to force early exit
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	result, err := rc.CheckReconciliation(ctx, component, "default", 30*time.Second)
	elapsed := time.Since(start)

	// Should exit due to context timeout, not the 30s deadline
	require.Error(t, err)
	assert.False(t, result.AllReconciled)
	assert.Less(t, elapsed, 5*time.Second, "should exit quickly due to context timeout")
}
