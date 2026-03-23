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

func TestConfigDriftValidate(t *testing.T) {
	injector := &ConfigDriftInjector{}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}

	tests := []struct {
		name    string
		spec    v1alpha1.InjectionSpec
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid spec with all required params",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.ConfigDrift,
				Parameters: map[string]string{
					"name":  "my-configmap",
					"key":   "config.yaml",
					"value": "corrupted-data",
				},
			},
			wantErr: false,
		},
		{
			name: "valid spec with Secret resourceType",
			spec: v1alpha1.InjectionSpec{
				Type:        v1alpha1.ConfigDrift,
				DangerLevel: v1alpha1.DangerLevelHigh,
				Parameters: map[string]string{
					"name":         "my-secret",
					"key":          "password",
					"value":        "wrong-password",
					"resourceType": "Secret",
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.ConfigDrift,
				Parameters: map[string]string{
					"key":   "config.yaml",
					"value": "corrupted-data",
				},
			},
			wantErr: true,
			errMsg:  "name",
		},
		{
			name: "missing key",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.ConfigDrift,
				Parameters: map[string]string{
					"name":  "my-configmap",
					"value": "corrupted-data",
				},
			},
			wantErr: true,
			errMsg:  "key",
		},
		{
			name: "missing value",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.ConfigDrift,
				Parameters: map[string]string{
					"name": "my-configmap",
					"key":  "config.yaml",
				},
			},
			wantErr: true,
			errMsg:  "value",
		},
		{
			name: "nil parameters",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.ConfigDrift,
			},
			wantErr: true,
			errMsg:  "name",
		},
		{
			name: "invalid resource name",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.ConfigDrift,
				Parameters: map[string]string{
					"name":  "INVALID NAME!",
					"key":   "config.yaml",
					"value": "corrupted",
				},
			},
			wantErr: true,
			errMsg:  "not a valid Kubernetes name",
		},
		{
			name: "Secret rollback name exceeds K8s limit",
			spec: func() v1alpha1.InjectionSpec {
				// "chaos-rollback-" (16) + name (230) + "-" (1) + key (10) = 257 > 253
				longName := make([]byte, 230)
				for i := range longName {
					longName[i] = 'a'
				}
				return v1alpha1.InjectionSpec{
					Type:        v1alpha1.ConfigDrift,
					DangerLevel: v1alpha1.DangerLevelHigh,
					Parameters: map[string]string{
						"name":         string(longName),
						"key":          "my-key-val",
						"value":        "corrupted",
						"resourceType": "Secret",
					},
				}
			}(),
			wantErr: true,
			errMsg:  "exceeds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := injector.Validate(tt.spec, blast)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigDriftInjectAndCleanup(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	originalData := map[string]string{
		"config.yaml": "original-value",
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-configmap",
			Namespace: "test-ns",
		},
		Data: originalData,
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	injector := NewConfigDriftInjector(fakeClient)

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ConfigDrift,
		Parameters: map[string]string{
			"name":  "my-configmap",
			"key":   "config.yaml",
			"value": "corrupted-data",
		},
	}

	ctx := context.Background()

	// Inject drift
	cleanup, events, err := injector.Inject(ctx, spec, "test-ns")
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, v1alpha1.ConfigDrift, events[0].Type)
	assert.Equal(t, "drifted", events[0].Action)
	require.NotNil(t, cleanup)

	// Verify ConfigMap value was changed
	driftedCM := &corev1.ConfigMap{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{Name: "my-configmap", Namespace: "test-ns"}, driftedCM))
	assert.Equal(t, "corrupted-data", driftedCM.Data["config.yaml"], "value should be corrupted after inject")

	// Cleanup: should restore original value
	require.NoError(t, cleanup(ctx))

	// Verify ConfigMap value was restored
	restoredCM := &corev1.ConfigMap{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{Name: "my-configmap", Namespace: "test-ns"}, restoredCM))
	assert.Equal(t, "original-value", restoredCM.Data["config.yaml"], "value should be restored after cleanup")
}

func TestConfigDriftInjectStoresRollbackAnnotation(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "annotated-cm",
			Namespace: "test-ns",
		},
		Data: map[string]string{
			"app.conf": "original-config",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	injector := NewConfigDriftInjector(fakeClient)
	ctx := context.Background()

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ConfigDrift,
		Parameters: map[string]string{
			"name":  "annotated-cm",
			"key":   "app.conf",
			"value": "corrupted-config",
		},
	}

	// Inject
	cleanup, _, err := injector.Inject(ctx, spec, "test-ns")
	require.NoError(t, err)

	// Verify rollback annotation is present
	modified := &corev1.ConfigMap{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{Name: "annotated-cm", Namespace: "test-ns"}, modified))

	rollbackJSON, ok := modified.Annotations[safety.RollbackAnnotationKey]
	require.True(t, ok, "rollback annotation should be present after injection")

	var rollbackData map[string]string
	require.NoError(t, safety.UnwrapRollbackData(rollbackJSON, &rollbackData))
	assert.Equal(t, "ConfigMap", rollbackData["resourceType"])
	assert.Equal(t, "app.conf", rollbackData["key"])
	assert.Equal(t, "original-config", rollbackData["originalValue"])

	// Verify chaos labels
	assert.Equal(t, safety.ManagedByValue, modified.Labels[safety.ManagedByLabel])
	assert.Equal(t, string(v1alpha1.ConfigDrift), modified.Labels[safety.ChaosTypeLabel])

	// Cleanup should remove annotation and labels
	require.NoError(t, cleanup(ctx))

	restored := &corev1.ConfigMap{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{Name: "annotated-cm", Namespace: "test-ns"}, restored))

	_, hasAnnotation := restored.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasAnnotation, "rollback annotation should be removed after cleanup")

	_, hasManagedBy := restored.Labels[safety.ManagedByLabel]
	assert.False(t, hasManagedBy, "managed-by label should be removed after cleanup")

	_, hasChaosType := restored.Labels[safety.ChaosTypeLabel]
	assert.False(t, hasChaosType, "chaos-type label should be removed after cleanup")

	// Verify value was restored
	assert.Equal(t, "original-config", restored.Data["app.conf"])
}

func TestConfigDriftRevertConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "revert-cm",
			Namespace: "test-ns",
		},
		Data: map[string]string{
			"config.yaml": "original-value",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	injector := NewConfigDriftInjector(fakeClient)
	ctx := context.Background()

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ConfigDrift,
		Parameters: map[string]string{
			"name":  "revert-cm",
			"key":   "config.yaml",
			"value": "corrupted",
		},
	}

	// Inject
	_, _, err := injector.Inject(ctx, spec, "test-ns")
	require.NoError(t, err)

	// Revert via Revert() method
	err = injector.Revert(ctx, spec, "test-ns")
	require.NoError(t, err)

	// Verify data restored
	restored := &corev1.ConfigMap{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{Name: "revert-cm", Namespace: "test-ns"}, restored))
	assert.Equal(t, "original-value", restored.Data["config.yaml"])

	// Verify idempotent — second Revert is a no-op
	err = injector.Revert(ctx, spec, "test-ns")
	assert.NoError(t, err)
}

func TestConfigDriftRevertSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "revert-secret",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"password": []byte("original-password"),
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	injector := NewConfigDriftInjector(fakeClient)
	ctx := context.Background()

	spec := v1alpha1.InjectionSpec{
		Type:        v1alpha1.ConfigDrift,
		DangerLevel: v1alpha1.DangerLevelHigh,
		Parameters: map[string]string{
			"name":         "revert-secret",
			"key":          "password",
			"value":        "corrupted-pw",
			"resourceType": "Secret",
		},
	}

	// Inject
	_, _, err := injector.Inject(ctx, spec, "test-ns")
	require.NoError(t, err)

	// Revert
	err = injector.Revert(ctx, spec, "test-ns")
	require.NoError(t, err)

	// Verify data restored
	restored := &corev1.Secret{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{Name: "revert-secret", Namespace: "test-ns"}, restored))
	assert.Equal(t, "original-password", string(restored.Data["password"]))

	// Verify idempotent
	err = injector.Revert(ctx, spec, "test-ns")
	assert.NoError(t, err)
}

func TestConfigDriftInjectSecretAndCleanup(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"password": []byte("original-password"),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	injector := NewConfigDriftInjector(fakeClient)
	ctx := context.Background()

	spec := v1alpha1.InjectionSpec{
		Type:        v1alpha1.ConfigDrift,
		DangerLevel: v1alpha1.DangerLevelHigh,
		Parameters: map[string]string{
			"name":         "my-secret",
			"key":          "password",
			"value":        "corrupted-password",
			"resourceType": "Secret",
		},
	}

	// Inject
	cleanup, events, err := injector.Inject(ctx, spec, "test-ns")
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "drifted", events[0].Action)

	// Verify Secret value was changed
	modified := &corev1.Secret{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{Name: "my-secret", Namespace: "test-ns"}, modified))
	assert.Equal(t, "corrupted-password", string(modified.Data["password"]),
		"value should be corrupted after inject")

	// Verify rollback annotation is present
	rollbackJSON, ok := modified.Annotations[safety.RollbackAnnotationKey]
	require.True(t, ok, "rollback annotation should be present after injection")

	var rollbackData map[string]string
	require.NoError(t, safety.UnwrapRollbackData(rollbackJSON, &rollbackData))
	assert.Equal(t, "Secret", rollbackData["resourceType"])
	assert.Equal(t, "password", rollbackData["key"])
	assert.Equal(t, "chaos-rollback-my-secret-password", rollbackData["rollbackSecretRef"])
	assert.Empty(t, rollbackData["originalValue"], "originalValue should not be stored for Secrets")

	// Verify chaos labels
	assert.Equal(t, safety.ManagedByValue, modified.Labels[safety.ManagedByLabel])
	assert.Equal(t, string(v1alpha1.ConfigDrift), modified.Labels[safety.ChaosTypeLabel])

	// Verify rollback Secret was created
	rbSecret := &corev1.Secret{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{Name: "chaos-rollback-my-secret-password", Namespace: "test-ns"}, rbSecret),
		"rollback Secret should exist after injection")
	assert.Equal(t, []byte("original-password"), rbSecret.Data["password"],
		"rollback Secret should contain the original value")

	// Cleanup should restore value and remove metadata
	require.NoError(t, cleanup(ctx))

	restored := &corev1.Secret{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{Name: "my-secret", Namespace: "test-ns"}, restored))
	assert.Equal(t, "original-password", string(restored.Data["password"]),
		"value should be restored after cleanup")

	_, hasAnnotation := restored.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasAnnotation, "rollback annotation should be removed after cleanup")

	_, hasManagedBy := restored.Labels[safety.ManagedByLabel]
	assert.False(t, hasManagedBy, "managed-by label should be removed after cleanup")

	// Verify rollback Secret was deleted
	err = fakeClient.Get(ctx, client.ObjectKey{Name: "chaos-rollback-my-secret-password", Namespace: "test-ns"}, rbSecret)
	assert.Error(t, err, "rollback Secret should be deleted after cleanup")
}
