package injection

import (
	"context"
	"testing"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFinalizerBlockValidate(t *testing.T) {
	injector := NewFinalizerBlockInjector(nil)
	blast := v1alpha1.BlastRadiusSpec{MaxPodsAffected: 1, AllowedNamespaces: []string{"test"}}

	tests := []struct {
		name    string
		spec    v1alpha1.InjectionSpec
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid spec",
			spec: v1alpha1.InjectionSpec{
				Type:        v1alpha1.FinalizerBlock,
				DangerLevel: v1alpha1.DangerLevelHigh,
				Parameters: map[string]string{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"name":       "my-config",
					"finalizer":  "chaos.opendatahub.io/block",
				},
			},
			wantErr: false,
		},
		{
			name: "valid spec without finalizer uses default",
			spec: v1alpha1.InjectionSpec{
				Type:        v1alpha1.FinalizerBlock,
				DangerLevel: v1alpha1.DangerLevelHigh,
				Parameters: map[string]string{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"name":       "my-config",
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.FinalizerBlock,
				Parameters: map[string]string{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
				},
			},
			wantErr: true,
			errMsg:  "name",
		},
		{
			name: "missing kind",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.FinalizerBlock,
				Parameters: map[string]string{
					"apiVersion": "v1",
					"name":       "my-config",
				},
			},
			wantErr: true,
			errMsg:  "kind",
		},
		{
			name: "invalid resource name",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.FinalizerBlock,
				Parameters: map[string]string{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"name":       "INVALID NAME!",
				},
			},
			wantErr: true,
			errMsg:  "not a valid Kubernetes name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := injector.Validate(tt.spec, blast)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFinalizerBlockInjectAndCleanup(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "my-config", Namespace: "default"},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	injector := NewFinalizerBlockInjector(k8sClient)

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.FinalizerBlock,
		Parameters: map[string]string{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"name":       "my-config",
			"finalizer":  "chaos.opendatahub.io/block",
		},
	}

	cleanup, events, err := injector.Inject(context.Background(), spec, "default")
	require.NoError(t, err)
	assert.NotEmpty(t, events)

	// Verify finalizer was added
	modified := &corev1.ConfigMap{}
	require.NoError(t, k8sClient.Get(context.Background(),
		client.ObjectKey{Name: "my-config", Namespace: "default"}, modified))
	assert.Contains(t, modified.Finalizers, "chaos.opendatahub.io/block")

	// Cleanup should remove the finalizer
	require.NoError(t, cleanup(context.Background()))
	restored := &corev1.ConfigMap{}
	require.NoError(t, k8sClient.Get(context.Background(),
		client.ObjectKey{Name: "my-config", Namespace: "default"}, restored))
	assert.NotContains(t, restored.Finalizers, "chaos.opendatahub.io/block")
}

func TestFinalizerBlockInjectDefaultFinalizer(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default"},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	injector := NewFinalizerBlockInjector(k8sClient)

	// No "finalizer" parameter — should use default
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.FinalizerBlock,
		Parameters: map[string]string{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"name":       "test-cm",
		},
	}

	cleanup, events, err := injector.Inject(context.Background(), spec, "default")
	require.NoError(t, err)
	assert.NotEmpty(t, events)

	// Verify default finalizer was added
	modified := &corev1.ConfigMap{}
	require.NoError(t, k8sClient.Get(context.Background(),
		client.ObjectKey{Name: "test-cm", Namespace: "default"}, modified))
	assert.Contains(t, modified.Finalizers, "chaos.opendatahub.io/block")

	// Cleanup should remove it
	require.NoError(t, cleanup(context.Background()))
	restored := &corev1.ConfigMap{}
	require.NoError(t, k8sClient.Get(context.Background(),
		client.ObjectKey{Name: "test-cm", Namespace: "default"}, restored))
	assert.NotContains(t, restored.Finalizers, "chaos.opendatahub.io/block")
}

func TestFinalizerBlockNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	injector := NewFinalizerBlockInjector(k8sClient)

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.FinalizerBlock,
		Parameters: map[string]string{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"name":       "nonexistent",
		},
	}

	_, _, err := injector.Inject(context.Background(), spec, "default")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestFinalizerBlockPreservesExistingFinalizers(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "existing-finalizers",
			Namespace:  "default",
			Finalizers: []string{"existing.io/finalizer"},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	injector := NewFinalizerBlockInjector(k8sClient)

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.FinalizerBlock,
		Parameters: map[string]string{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"name":       "existing-finalizers",
			"finalizer":  "chaos.opendatahub.io/block",
		},
	}

	cleanup, _, err := injector.Inject(context.Background(), spec, "default")
	require.NoError(t, err)

	// Verify both finalizers present
	modified := &corev1.ConfigMap{}
	require.NoError(t, k8sClient.Get(context.Background(),
		client.ObjectKey{Name: "existing-finalizers", Namespace: "default"}, modified))
	assert.Contains(t, modified.Finalizers, "existing.io/finalizer")
	assert.Contains(t, modified.Finalizers, "chaos.opendatahub.io/block")

	// Cleanup should only remove the chaos finalizer
	require.NoError(t, cleanup(context.Background()))
	restored := &corev1.ConfigMap{}
	require.NoError(t, k8sClient.Get(context.Background(),
		client.ObjectKey{Name: "existing-finalizers", Namespace: "default"}, restored))
	assert.Contains(t, restored.Finalizers, "existing.io/finalizer")
	assert.NotContains(t, restored.Finalizers, "chaos.opendatahub.io/block")
}

func TestFinalizerBlockInjectStoresRollbackAnnotation(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "rollback-test", Namespace: "default"},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	injector := NewFinalizerBlockInjector(k8sClient)

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.FinalizerBlock,
		Parameters: map[string]string{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"name":       "rollback-test",
			"finalizer":  "chaos.opendatahub.io/block",
		},
	}

	ctx := context.Background()

	// Inject
	cleanup, _, err := injector.Inject(ctx, spec, "default")
	require.NoError(t, err)

	// Verify rollback annotation is present after injection
	modified := &corev1.ConfigMap{}
	require.NoError(t, k8sClient.Get(ctx,
		client.ObjectKey{Name: "rollback-test", Namespace: "default"}, modified))

	rollbackJSON, ok := modified.Annotations[safety.RollbackAnnotationKey]
	require.True(t, ok, "rollback annotation should be present after injection")

	var rollbackData map[string]string
	require.NoError(t, safety.UnwrapRollbackData(rollbackJSON, &rollbackData))
	assert.Equal(t, "chaos.opendatahub.io/block", rollbackData["finalizer"],
		"rollback data should contain the finalizer name")

	// Verify chaos labels are present
	assert.Equal(t, safety.ManagedByValue, modified.Labels[safety.ManagedByLabel])
	assert.Equal(t, string(v1alpha1.FinalizerBlock), modified.Labels[safety.ChaosTypeLabel])

	// Cleanup should remove the annotation and labels
	require.NoError(t, cleanup(ctx))

	restored := &corev1.ConfigMap{}
	require.NoError(t, k8sClient.Get(ctx,
		client.ObjectKey{Name: "rollback-test", Namespace: "default"}, restored))

	_, hasAnnotation := restored.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasAnnotation, "rollback annotation should be removed after cleanup")

	_, hasManagedBy := restored.Labels[safety.ManagedByLabel]
	assert.False(t, hasManagedBy, "managed-by label should be removed after cleanup")

	_, hasChaosType := restored.Labels[safety.ChaosTypeLabel]
	assert.False(t, hasChaosType, "chaos-type label should be removed after cleanup")

	// Verify finalizer was also removed
	assert.NotContains(t, restored.Finalizers, "chaos.opendatahub.io/block")
}

func TestFinalizerBlockRevert(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "revert-finalized", Namespace: "default"},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	injector := NewFinalizerBlockInjector(k8sClient)
	ctx := context.Background()

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.FinalizerBlock,
		Parameters: map[string]string{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"name":       "revert-finalized",
			"finalizer":  "chaos.opendatahub.io/block",
		},
	}

	// Inject
	_, _, err := injector.Inject(ctx, spec, "default")
	require.NoError(t, err)

	// Verify finalizer was added
	modified := &corev1.ConfigMap{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "revert-finalized", Namespace: "default"}, modified))
	assert.Contains(t, modified.Finalizers, "chaos.opendatahub.io/block")

	// Revert
	err = injector.Revert(ctx, spec, "default")
	require.NoError(t, err)

	// Verify finalizer removed
	restored := &corev1.ConfigMap{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "revert-finalized", Namespace: "default"}, restored))
	assert.NotContains(t, restored.Finalizers, "chaos.opendatahub.io/block")

	// Idempotent
	err = injector.Revert(ctx, spec, "default")
	assert.NoError(t, err)
}

func TestFinalizerBlockRejectsDuplicateFinalizer(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	// Create a ConfigMap that already has the chaos finalizer
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "already-finalized",
			Namespace:  "default",
			Finalizers: []string{"chaos.opendatahub.io/block"},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	injector := NewFinalizerBlockInjector(k8sClient)

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.FinalizerBlock,
		Parameters: map[string]string{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"name":       "already-finalized",
			"finalizer":  "chaos.opendatahub.io/block",
		},
	}

	cleanup, events, err := injector.Inject(context.Background(), spec, "default")
	assert.Nil(t, cleanup, "cleanup should be nil when injection is a no-op")
	assert.Nil(t, events, "events should be nil when injection is a no-op")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
	assert.Contains(t, err.Error(), "no-op")
}
