package injection

import (
	"context"
	"testing"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCRDMutationValidate(t *testing.T) {
	injector := &CRDMutationInjector{}
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
				Type: v1alpha1.CRDMutation,
				Parameters: map[string]string{
					"apiVersion": "v1",
					"kind":       "DataScienceCluster",
					"name":       "default-dsc",
					"field":      "replicas",
					"value":      "0",
				},
			},
			wantErr: false,
		},
		{
			name: "missing apiVersion",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.CRDMutation,
				Parameters: map[string]string{
					"kind":  "DataScienceCluster",
					"name":  "default-dsc",
					"field": "replicas",
					"value": "0",
				},
			},
			wantErr: true,
			errMsg:  "apiVersion",
		},
		{
			name: "missing kind",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.CRDMutation,
				Parameters: map[string]string{
					"apiVersion": "v1",
					"name":       "default-dsc",
					"field":      "replicas",
					"value":      "0",
				},
			},
			wantErr: true,
			errMsg:  "kind",
		},
		{
			name: "missing name",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.CRDMutation,
				Parameters: map[string]string{
					"apiVersion": "v1",
					"kind":       "DataScienceCluster",
					"field":      "replicas",
					"value":      "0",
				},
			},
			wantErr: true,
			errMsg:  "name",
		},
		{
			name: "missing field",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.CRDMutation,
				Parameters: map[string]string{
					"apiVersion": "v1",
					"kind":       "DataScienceCluster",
					"name":       "default-dsc",
					"value":      "0",
				},
			},
			wantErr: true,
			errMsg:  "field",
		},
		{
			name: "missing value",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.CRDMutation,
				Parameters: map[string]string{
					"apiVersion": "v1",
					"kind":       "DataScienceCluster",
					"name":       "default-dsc",
					"field":      "replicas",
				},
			},
			wantErr: true,
			errMsg:  "value",
		},
		{
			name: "nil parameters",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.CRDMutation,
			},
			wantErr: true,
			errMsg:  "apiVersion",
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

func TestCRDMutationInjectAndCleanup(t *testing.T) {
	scheme := runtime.NewScheme()
	gvk := schema.GroupVersionKind{Group: "test.example.com", Version: "v1", Kind: "TestResource"}
	scheme.AddKnownTypeWithName(gvk,
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "test.example.com", Version: "v1", Kind: "TestResourceList"},
		&unstructured.UnstructuredList{},
	)

	// Create an unstructured resource with a spec.replicas field
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName("my-resource")
	obj.SetNamespace("test-ns")
	obj.Object["spec"] = map[string]interface{}{
		"replicas": int64(3),
		"other":    "keep-me",
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(obj).
		Build()

	injector := NewCRDMutationInjector(fakeClient)
	ctx := context.Background()

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.CRDMutation,
		Parameters: map[string]string{
			"apiVersion": "test.example.com/v1",
			"kind":       "TestResource",
			"name":       "my-resource",
			"field":      "replicas",
			"value":      "0",
		},
	}

	// Inject the mutation
	cleanup, events, err := injector.Inject(ctx, spec, "test-ns")
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	require.Len(t, events, 1)
	assert.Equal(t, "mutated", events[0].Action)

	// Verify the field was mutated
	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(gvk)
	require.NoError(t, fakeClient.Get(ctx, client_key("my-resource", "test-ns"), current))
	specMap, ok, err := unstructured.NestedMap(current.Object, "spec")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "0", specMap["replicas"])
	// Other fields should be preserved
	assert.Equal(t, "keep-me", specMap["other"])

	// Simulate operator reconciliation by modifying another field
	// This changes the resourceVersion, which would cause the old Update approach to fail
	specMap["reconciledField"] = "operator-was-here"
	require.NoError(t, unstructured.SetNestedMap(current.Object, specMap, "spec"))
	require.NoError(t, fakeClient.Update(ctx, current))

	// Run cleanup - should succeed despite resourceVersion change
	err = cleanup(ctx)
	require.NoError(t, err)

	// Verify the field was restored to its original value
	restored := &unstructured.Unstructured{}
	restored.SetGroupVersionKind(gvk)
	require.NoError(t, fakeClient.Get(ctx, client_key("my-resource", "test-ns"), restored))
	restoredSpec, ok, err := unstructured.NestedMap(restored.Object, "spec")
	require.NoError(t, err)
	require.True(t, ok)
	// The mutated field should be restored to the original value
	assert.Equal(t, int64(3), restoredSpec["replicas"])
	// Other fields should still be preserved
	assert.Equal(t, "keep-me", restoredSpec["other"])
	// Operator's reconciled field should still be there (merge patch doesn't remove other fields)
	assert.Equal(t, "operator-was-here", restoredSpec["reconciledField"])
}

// client_key is a helper to create a NamespacedName for client.Get.
func client_key(name, namespace string) client.ObjectKey {
	return client.ObjectKey{Name: name, Namespace: namespace}
}
