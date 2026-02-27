package injection

import (
	"context"
	"testing"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
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
				Type: v1alpha1.ConfigDrift,
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
