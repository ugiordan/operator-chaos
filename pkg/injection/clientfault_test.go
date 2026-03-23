package injection

import (
	"context"
	"encoding/json"
	"testing"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestClientFaultValidate(t *testing.T) {
	injector := &ClientFaultInjector{}
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
			name: "valid spec",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.ClientFault,
				Parameters: map[string]string{
					"faults": `{"get":{"errorRate":0.3,"error":"throttled"}}`,
				},
			},
			wantErr: false,
		},
		{
			name: "valid spec with custom configMapName",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.ClientFault,
				Parameters: map[string]string{
					"faults":        `{"update":{"errorRate":0.5,"error":"conflict"}}`,
					"configMapName": "odh-chaos-custom",
				},
			},
			wantErr: false,
		},
		{
			name: "missing faults",
			spec: v1alpha1.InjectionSpec{
				Type:       v1alpha1.ClientFault,
				Parameters: map[string]string{},
			},
			wantErr: true,
			errMsg:  "faults",
		},
		{
			name: "invalid operation",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.ClientFault,
				Parameters: map[string]string{
					"faults": `{"badop":{"errorRate":0.5,"error":"bad"}}`,
				},
			},
			wantErr: true,
			errMsg:  "badop",
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

func TestClientFaultInjectCreatesConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	injector := NewClientFaultInjector(fakeClient)
	ctx := context.Background()

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":0.3,"error":"throttled"},"update":{"errorRate":0.5,"error":"conflict"}}`,
		},
	}

	cleanup, events, err := injector.Inject(ctx, spec, "opendatahub")
	require.NoError(t, err)
	assert.NotEmpty(t, events)
	require.NotNil(t, cleanup)

	// Verify ConfigMap was created with correct fault config
	cm := &corev1.ConfigMap{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{
		Name:      sdk.ChaosConfigMapName,
		Namespace: "opendatahub",
	}, cm))

	var fc clientFaultConfig
	require.NoError(t, json.Unmarshal([]byte(cm.Data[sdk.ChaosConfigKey]), &fc))
	assert.True(t, fc.Active)
	assert.Len(t, fc.Faults, 2)
	assert.Equal(t, 0.3, fc.Faults["get"].ErrorRate)
	assert.Equal(t, "throttled", fc.Faults["get"].Error)
	assert.Equal(t, 0.5, fc.Faults["update"].ErrorRate)

	// Verify chaos metadata
	assert.Equal(t, safety.ManagedByValue, cm.Labels[safety.ManagedByLabel])
	assert.Equal(t, string(v1alpha1.ClientFault), cm.Labels[safety.ChaosTypeLabel])
}

func TestClientFaultInjectCleanupDeletesNewConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	injector := NewClientFaultInjector(fakeClient)
	ctx := context.Background()

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":0.5,"error":"test"}}`,
		},
	}

	cleanup, _, err := injector.Inject(ctx, spec, "opendatahub")
	require.NoError(t, err)

	// Cleanup should delete the ConfigMap (it didn't exist before)
	require.NoError(t, cleanup(ctx))

	cm := &corev1.ConfigMap{}
	err = fakeClient.Get(ctx, client.ObjectKey{
		Name:      sdk.ChaosConfigMapName,
		Namespace: "opendatahub",
	}, cm)
	assert.Error(t, err, "ConfigMap should be deleted after cleanup")
}

func TestClientFaultInjectCleanupRestoresExistingConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	// Pre-existing ConfigMap with some config
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sdk.ChaosConfigMapName,
			Namespace: "opendatahub",
		},
		Data: map[string]string{
			sdk.ChaosConfigKey: `{"active":false,"faults":{}}`,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingCM).
		Build()

	injector := NewClientFaultInjector(fakeClient)
	ctx := context.Background()

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":0.5,"error":"injected"}}`,
		},
	}

	cleanup, _, err := injector.Inject(ctx, spec, "opendatahub")
	require.NoError(t, err)

	// Cleanup should restore the original ConfigMap data
	require.NoError(t, cleanup(ctx))

	cm := &corev1.ConfigMap{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{
		Name:      sdk.ChaosConfigMapName,
		Namespace: "opendatahub",
	}, cm))
	assert.Equal(t, `{"active":false,"faults":{}}`, cm.Data[sdk.ChaosConfigKey],
		"original ConfigMap data should be restored")

	// Chaos metadata should be removed
	_, hasManagedBy := cm.Labels[safety.ManagedByLabel]
	assert.False(t, hasManagedBy)
}

func TestClientFaultInjectWithCustomConfigMapName(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	injector := NewClientFaultInjector(fakeClient)
	ctx := context.Background()

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults":        `{"list":{"errorRate":1.0,"error":"timeout"}}`,
			"configMapName": "odh-chaos-custom",
		},
	}

	cleanup, _, err := injector.Inject(ctx, spec, "opendatahub")
	require.NoError(t, err)

	// Verify it used the custom ConfigMap name
	cm := &corev1.ConfigMap{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{
		Name:      "odh-chaos-custom",
		Namespace: "opendatahub",
	}, cm))
	assert.Contains(t, cm.Data[sdk.ChaosConfigKey], "timeout")

	require.NoError(t, cleanup(ctx))
}

func TestClientFaultRevertDeletesCreatedCM(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	injector := NewClientFaultInjector(fakeClient)
	ctx := context.Background()

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":0.5,"error":"test"}}`,
		},
	}

	// Inject when no CM exists — should create it
	_, _, err := injector.Inject(ctx, spec, "opendatahub")
	require.NoError(t, err)

	// Verify CM exists
	cm := &corev1.ConfigMap{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{
		Name: sdk.ChaosConfigMapName, Namespace: "opendatahub",
	}, cm))

	// Revert should delete the CM
	err = injector.Revert(ctx, spec, "opendatahub")
	require.NoError(t, err)

	// Verify CM is gone
	err = fakeClient.Get(ctx, client.ObjectKey{
		Name: sdk.ChaosConfigMapName, Namespace: "opendatahub",
	}, cm)
	assert.Error(t, err, "CM should be deleted after revert")

	// Idempotent — second revert when CM is already gone
	err = injector.Revert(ctx, spec, "opendatahub")
	assert.NoError(t, err)
}

func TestClientFaultRevertRestoresExistingCM(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sdk.ChaosConfigMapName,
			Namespace: "opendatahub",
		},
		Data: map[string]string{
			sdk.ChaosConfigKey: `{"active":false,"faults":{}}`,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingCM).Build()
	injector := NewClientFaultInjector(fakeClient)
	ctx := context.Background()

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":0.5,"error":"injected"}}`,
		},
	}

	// Inject
	_, _, err := injector.Inject(ctx, spec, "opendatahub")
	require.NoError(t, err)

	// Revert
	err = injector.Revert(ctx, spec, "opendatahub")
	require.NoError(t, err)

	// Verify data restored
	cm := &corev1.ConfigMap{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{
		Name: sdk.ChaosConfigMapName, Namespace: "opendatahub",
	}, cm))
	assert.Equal(t, `{"active":false,"faults":{}}`, cm.Data[sdk.ChaosConfigKey])

	// Idempotent
	err = injector.Revert(ctx, spec, "opendatahub")
	assert.NoError(t, err)
}

func TestClientFaultCleanupWhenCMDeletedExternally(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sdk.ChaosConfigMapName,
			Namespace: "opendatahub",
		},
		Data: map[string]string{
			sdk.ChaosConfigKey: `{"active":false,"faults":{}}`,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingCM).
		Build()

	injector := NewClientFaultInjector(fakeClient)
	ctx := context.Background()

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":0.5,"error":"test"}}`,
		},
	}

	cleanup, _, err := injector.Inject(ctx, spec, "opendatahub")
	require.NoError(t, err)

	// Simulate external deletion of the ConfigMap
	require.NoError(t, fakeClient.Delete(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sdk.ChaosConfigMapName,
			Namespace: "opendatahub",
		},
	}))

	// Cleanup should succeed gracefully (not error)
	assert.NoError(t, cleanup(ctx))
}

func TestClientFaultInjectEventContent(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	injector := NewClientFaultInjector(fakeClient)
	ctx := context.Background()

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":0.5,"error":"test"}}`,
		},
	}

	_, events, err := injector.Inject(ctx, spec, "opendatahub")
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, v1alpha1.ClientFault, events[0].Type)
	assert.Equal(t, "activated", events[0].Action)
	assert.Equal(t, sdk.ChaosConfigMapName, events[0].Details["configMap"])
	assert.Equal(t, "opendatahub", events[0].Details["namespace"])
}

func TestClientFaultInjectStoresRollbackAnnotation(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sdk.ChaosConfigMapName,
			Namespace: "opendatahub",
		},
		Data: map[string]string{
			sdk.ChaosConfigKey: `{"active":false,"faults":{}}`,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingCM).
		Build()

	injector := NewClientFaultInjector(fakeClient)
	ctx := context.Background()

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":0.5,"error":"injected"}}`,
		},
	}

	_, _, err := injector.Inject(ctx, spec, "opendatahub")
	require.NoError(t, err)

	cm := &corev1.ConfigMap{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{
		Name:      sdk.ChaosConfigMapName,
		Namespace: "opendatahub",
	}, cm))

	// Verify rollback annotation is present and non-empty
	annotations := cm.GetAnnotations()
	rollbackData, ok := annotations[safety.RollbackAnnotationKey]
	assert.True(t, ok, "rollback annotation should be present")
	assert.NotEmpty(t, rollbackData, "rollback annotation should contain original data")

	// Verify the rollback data can be unwrapped and contains original CM data
	var restored map[string]string
	require.NoError(t, safety.UnwrapRollbackData(rollbackData, &restored))
	assert.Equal(t, `{"active":false,"faults":{}}`, restored[sdk.ChaosConfigKey])
}

func TestClientFaultInjectLabelsOnExistingCM(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sdk.ChaosConfigMapName,
			Namespace: "opendatahub",
		},
		Data: map[string]string{
			sdk.ChaosConfigKey: `{"active":false,"faults":{}}`,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingCM).
		Build()

	injector := NewClientFaultInjector(fakeClient)
	ctx := context.Background()

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":0.5,"error":"test"}}`,
		},
	}

	_, _, err := injector.Inject(ctx, spec, "opendatahub")
	require.NoError(t, err)

	cm := &corev1.ConfigMap{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{
		Name:      sdk.ChaosConfigMapName,
		Namespace: "opendatahub",
	}, cm))

	assert.Equal(t, safety.ManagedByValue, cm.Labels[safety.ManagedByLabel])
	assert.Equal(t, string(v1alpha1.ClientFault), cm.Labels[safety.ChaosTypeLabel])
}
