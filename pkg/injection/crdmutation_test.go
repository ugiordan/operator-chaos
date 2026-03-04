package injection

import (
	"context"
	"testing"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
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
		{
			name: "invalid resource name",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.CRDMutation,
				Parameters: map[string]string{
					"apiVersion": "v1",
					"kind":       "DataScienceCluster",
					"name":       "INVALID NAME!",
					"field":      "replicas",
					"value":      "0",
				},
			},
			wantErr: true,
			errMsg:  "not a valid Kubernetes name",
		},
		{
			name: "invalid field name",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.CRDMutation,
				Parameters: map[string]string{
					"apiVersion": "v1",
					"kind":       "DataScienceCluster",
					"name":       "default-dsc",
					"field":      "123invalid",
					"value":      "0",
				},
			},
			wantErr: true,
			errMsg:  "not a valid field name",
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
	// "0" is parsed as a JSON number, so it becomes float64(0) in the unstructured map
	assert.Equal(t, int64(0), specMap["replicas"])
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

func TestCRDMutationInjectStoresRollbackAnnotation(t *testing.T) {
	scheme := runtime.NewScheme()
	gvk := schema.GroupVersionKind{Group: "test.example.com", Version: "v1", Kind: "TestResource"}
	scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "test.example.com", Version: "v1", Kind: "TestResourceList"},
		&unstructured.UnstructuredList{},
	)

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName("annotated-resource")
	obj.SetNamespace("test-ns")
	obj.Object["spec"] = map[string]interface{}{
		"replicas": int64(5),
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
			"name":       "annotated-resource",
			"field":      "replicas",
			"value":      "0",
		},
	}

	// Inject
	cleanup, _, err := injector.Inject(ctx, spec, "test-ns")
	require.NoError(t, err)

	// Verify rollback annotation is present
	modified := &unstructured.Unstructured{}
	modified.SetGroupVersionKind(gvk)
	require.NoError(t, fakeClient.Get(ctx, client_key("annotated-resource", "test-ns"), modified))

	annotations := modified.GetAnnotations()
	require.NotNil(t, annotations)
	rollbackJSON, ok := annotations[safety.RollbackAnnotationKey]
	require.True(t, ok, "rollback annotation should be present after injection")

	var rollbackData map[string]interface{}
	require.NoError(t, safety.UnwrapRollbackData(rollbackJSON, &rollbackData))
	assert.Equal(t, "test.example.com/v1", rollbackData["apiVersion"])
	assert.Equal(t, "TestResource", rollbackData["kind"])
	assert.Equal(t, "replicas", rollbackData["field"])
	// originalValue is stored as a float64 from JSON unmarshaling of int64
	assert.Equal(t, float64(5), rollbackData["originalValue"])

	// Verify chaos labels
	labels := modified.GetLabels()
	assert.Equal(t, safety.ManagedByValue, labels[safety.ManagedByLabel])
	assert.Equal(t, string(v1alpha1.CRDMutation), labels[safety.ChaosTypeLabel])

	// Cleanup should remove annotation and labels
	require.NoError(t, cleanup(ctx))

	restored := &unstructured.Unstructured{}
	restored.SetGroupVersionKind(gvk)
	require.NoError(t, fakeClient.Get(ctx, client_key("annotated-resource", "test-ns"), restored))

	restoredAnnotations := restored.GetAnnotations()
	if restoredAnnotations != nil {
		_, hasAnnotation := restoredAnnotations[safety.RollbackAnnotationKey]
		assert.False(t, hasAnnotation, "rollback annotation should be removed after cleanup")
	}

	restoredLabels := restored.GetLabels()
	if restoredLabels != nil {
		_, hasManagedBy := restoredLabels[safety.ManagedByLabel]
		assert.False(t, hasManagedBy, "managed-by label should be removed after cleanup")

		_, hasChaosType := restoredLabels[safety.ChaosTypeLabel]
		assert.False(t, hasChaosType, "chaos-type label should be removed after cleanup")
	}

	// Verify value was restored
	restoredSpec, _, _ := unstructured.NestedMap(restored.Object, "spec")
	assert.Equal(t, int64(5), restoredSpec["replicas"])
}

func TestCRDMutationInjectTypedValues(t *testing.T) {
	scheme := runtime.NewScheme()
	gvk := schema.GroupVersionKind{Group: "test.example.com", Version: "v1", Kind: "TestResource"}
	scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "test.example.com", Version: "v1", Kind: "TestResourceList"},
		&unstructured.UnstructuredList{},
	)

	tests := []struct {
		name          string
		paramValue    string
		originalValue any
		expectedValue any
	}{
		{
			name:          "integer value is injected as number",
			paramValue:    "999",
			originalValue: int64(3),
			expectedValue: int64(999),
		},
		{
			name:          "zero is injected as number",
			paramValue:    "0",
			originalValue: int64(5),
			expectedValue: int64(0),
		},
		{
			name:          "boolean true is injected as bool",
			paramValue:    "true",
			originalValue: false,
			expectedValue: true,
		},
		{
			name:          "boolean false is injected as bool",
			paramValue:    "false",
			originalValue: true,
			expectedValue: false,
		},
		{
			name:          "plain string stays as string",
			paramValue:    "some-value",
			originalValue: "original",
			expectedValue: "some-value",
		},
		{
			name:          "null is injected as nil",
			paramValue:    "null",
			originalValue: "something",
			expectedValue: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(gvk)
			obj.SetName("typed-resource")
			obj.SetNamespace("test-ns")
			obj.Object["spec"] = map[string]interface{}{
				"targetField": tt.originalValue,
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
					"name":       "typed-resource",
					"field":      "targetField",
					"value":      tt.paramValue,
				},
			}

			cleanup, events, err := injector.Inject(ctx, spec, "test-ns")
			require.NoError(t, err)
			require.NotNil(t, cleanup)
			require.Len(t, events, 1)

			// Verify the field was set with the correct type
			current := &unstructured.Unstructured{}
			current.SetGroupVersionKind(gvk)
			require.NoError(t, fakeClient.Get(ctx, client_key("typed-resource", "test-ns"), current))
			specMap, ok, err := unstructured.NestedMap(current.Object, "spec")
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, tt.expectedValue, specMap["targetField"],
				"expected %T(%v), got %T(%v)",
				tt.expectedValue, tt.expectedValue,
				specMap["targetField"], specMap["targetField"])

			// Cleanup should restore original
			require.NoError(t, cleanup(ctx))

			restored := &unstructured.Unstructured{}
			restored.SetGroupVersionKind(gvk)
			require.NoError(t, fakeClient.Get(ctx, client_key("typed-resource", "test-ns"), restored))
			restoredSpec, ok, err := unstructured.NestedMap(restored.Object, "spec")
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, tt.originalValue, restoredSpec["targetField"])
		})
	}
}

func TestParseTypedValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected any
	}{
		{name: "integer", input: "42", expected: float64(42)},
		{name: "negative integer", input: "-1", expected: float64(-1)},
		{name: "float", input: "3.14", expected: float64(3.14)},
		{name: "boolean true", input: "true", expected: true},
		{name: "boolean false", input: "false", expected: false},
		{name: "null", input: "null", expected: nil},
		{name: "plain string", input: "hello", expected: "hello"},
		{name: "string with spaces", input: "hello world", expected: "hello world"},
		{name: "empty string", input: "", expected: ""},
		{name: "quoted JSON string", input: `"quoted"`, expected: "quoted"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTypedValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// client_key is a helper to create a NamespacedName for client.Get.
func client_key(name, namespace string) client.ObjectKey {
	return client.ObjectKey{Name: name, Namespace: namespace}
}
