package injection

import (
	"context"
	"encoding/json"
	"fmt"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CRDMutationInjector struct {
	client client.Client
}

func NewCRDMutationInjector(c client.Client) *CRDMutationInjector {
	return &CRDMutationInjector{client: c}
}

func (m *CRDMutationInjector) Validate(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error {
	if _, ok := spec.Parameters["apiVersion"]; !ok {
		return fmt.Errorf("CRDMutation requires 'apiVersion' parameter")
	}
	if _, ok := spec.Parameters["kind"]; !ok {
		return fmt.Errorf("CRDMutation requires 'kind' parameter")
	}
	if _, ok := spec.Parameters["name"]; !ok {
		return fmt.Errorf("CRDMutation requires 'name' parameter")
	}
	if _, ok := spec.Parameters["field"]; !ok {
		return fmt.Errorf("CRDMutation requires 'field' parameter (JSON path to mutate)")
	}
	if _, ok := spec.Parameters["value"]; !ok {
		return fmt.Errorf("CRDMutation requires 'value' parameter (JSON value to set)")
	}
	return nil
}

func (m *CRDMutationInjector) Inject(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) (CleanupFunc, []v1alpha1.InjectionEvent, error) {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(spec.Parameters["apiVersion"])
	obj.SetKind(spec.Parameters["kind"])

	key := types.NamespacedName{
		Name:      spec.Parameters["name"],
		Namespace: namespace,
	}

	if err := m.client.Get(ctx, key, obj); err != nil {
		return nil, nil, fmt.Errorf("getting resource %s/%s: %w", spec.Parameters["kind"], spec.Parameters["name"], err)
	}

	// Save original field value for cleanup
	fieldName := spec.Parameters["field"]
	specMap, _, _ := unstructured.NestedMap(obj.Object, "spec")
	var originalValue interface{}
	if specMap != nil {
		originalValue = specMap[fieldName]
	}

	// Apply mutation via merge patch (use json.Marshal for safe serialization)
	patchMap := map[string]interface{}{
		"spec": map[string]interface{}{
			fieldName: spec.Parameters["value"],
		},
	}
	patch, err := json.Marshal(patchMap)
	if err != nil {
		return nil, nil, fmt.Errorf("building mutation patch: %w", err)
	}
	if err := m.client.Patch(ctx, obj, client.RawPatch(types.MergePatchType, patch)); err != nil {
		return nil, nil, fmt.Errorf("applying mutation: %w", err)
	}

	// Save GVK info for cleanup re-fetch
	apiVersion := spec.Parameters["apiVersion"]
	kind := spec.Parameters["kind"]

	events := []v1alpha1.InjectionEvent{
		NewEvent(v1alpha1.CRDMutation, key.String(), "mutated",
			map[string]string{
				"field": fieldName,
				"value": spec.Parameters["value"],
			}),
	}

	// Cleanup restores original field value using merge patch to avoid resourceVersion conflicts
	cleanup := func(ctx context.Context) error {
		// Build a merge patch that restores just the mutated field
		var restorePatchMap map[string]interface{}
		if originalValue == nil {
			// Field was not set before; set it to null to remove it via merge patch
			restorePatchMap = map[string]interface{}{
				"spec": map[string]interface{}{
					fieldName: nil,
				},
			}
		} else {
			restorePatchMap = map[string]interface{}{
				"spec": map[string]interface{}{
					fieldName: originalValue,
				},
			}
		}
		restorePatch, err := json.Marshal(restorePatchMap)
		if err != nil {
			return fmt.Errorf("building restore patch: %w", err)
		}

		// Re-fetch the resource to get current state as patch target
		current := &unstructured.Unstructured{}
		current.SetAPIVersion(apiVersion)
		current.SetKind(kind)
		if err := m.client.Get(ctx, key, current); err != nil {
			return fmt.Errorf("re-fetching resource for cleanup: %w", err)
		}

		return m.client.Patch(ctx, current, client.RawPatch(types.MergePatchType, restorePatch))
	}

	return cleanup, events, nil
}
