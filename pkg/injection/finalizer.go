package injection

import (
	"context"
	"fmt"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const defaultFinalizerName = "chaos.opendatahub.io/block"

// FinalizerBlockInjector adds a stuck finalizer to a resource, blocking its
// deletion and testing how operators handle resources stuck in a Terminating state.
type FinalizerBlockInjector struct {
	client client.Client
}

// NewFinalizerBlockInjector creates a new FinalizerBlockInjector.
func NewFinalizerBlockInjector(c client.Client) *FinalizerBlockInjector {
	return &FinalizerBlockInjector{client: c}
}

func (f *FinalizerBlockInjector) Validate(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error {
	return validateFinalizerBlockParams(spec)
}

// Inject adds a finalizer to the target resource:
//  1. Creates an Unstructured object with the apiVersion/kind from parameters
//  2. Fetches the resource from the cluster
//  3. Adds the finalizer to its finalizers list
//  4. Updates the object
//  5. Returns a cleanup function that removes the finalizer
func (f *FinalizerBlockInjector) Inject(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) (CleanupFunc, []v1alpha1.InjectionEvent, error) {
	apiVersion := spec.Parameters["apiVersion"]
	if apiVersion == "" {
		apiVersion = "v1"
	}

	kind := spec.Parameters["kind"]
	name := spec.Parameters["name"]

	finalizerName := spec.Parameters["finalizer"]
	if finalizerName == "" {
		finalizerName = defaultFinalizerName
	}

	// Build unstructured object to fetch the target resource
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(apiVersion)
	obj.SetKind(kind)

	key := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}

	if err := f.client.Get(ctx, key, obj); err != nil {
		return nil, nil, fmt.Errorf("getting resource %s/%s: %w", kind, name, err)
	}

	// Add the finalizer using controller-runtime helper.
	// If AddFinalizer returns false the finalizer already exists on the
	// resource and the injection would be a no-op — abort with an error so
	// the experiment does not silently claim success.
	if !controllerutil.AddFinalizer(obj, finalizerName) {
		return nil, nil, fmt.Errorf("finalizer %q already exists on %s/%s; injection would be a no-op", finalizerName, kind, name)
	}

	// Store rollback annotation for crash-safe recovery
	rollbackData := map[string]string{
		"finalizer": finalizerName,
	}
	rollbackStr, err := safety.WrapRollbackData(rollbackData)
	if err != nil {
		return nil, nil, fmt.Errorf("serializing rollback data for %s/%s: %w", kind, name, err)
	}

	safety.ApplyChaosMetadata(obj, rollbackStr, string(v1alpha1.FinalizerBlock))

	if err := f.client.Update(ctx, obj); err != nil {
		return nil, nil, fmt.Errorf("adding finalizer to %s/%s: %w", kind, name, err)
	}

	events := []v1alpha1.InjectionEvent{
		NewEvent(v1alpha1.FinalizerBlock, key.String(), "addFinalizer",
			map[string]string{
				"apiVersion": apiVersion,
				"kind":       kind,
				"name":       name,
				"finalizer":  finalizerName,
			}),
	}

	// Cleanup removes the finalizer, rollback annotation, and chaos labels
	cleanup := func(ctx context.Context) error {
		current := &unstructured.Unstructured{}
		current.SetAPIVersion(apiVersion)
		current.SetKind(kind)

		if err := f.client.Get(ctx, key, current); err != nil {
			return fmt.Errorf("re-fetching %s/%s for cleanup: %w", kind, name, err)
		}

		changed := controllerutil.RemoveFinalizer(current, finalizerName)

		// Remove rollback annotation and chaos labels
		if _, ok := current.GetAnnotations()[safety.RollbackAnnotationKey]; ok {
			changed = true
		}
		for k := range safety.ChaosLabels(string(v1alpha1.FinalizerBlock)) {
			if _, ok := current.GetLabels()[k]; ok {
				changed = true
			}
		}
		safety.RemoveChaosMetadata(current, string(v1alpha1.FinalizerBlock))

		if changed {
			if err := f.client.Update(ctx, current); err != nil {
				return fmt.Errorf("removing finalizer from %s/%s: %w", kind, name, err)
			}
		}

		return nil
	}

	return cleanup, events, nil
}

// Revert removes the injected finalizer and chaos metadata from the target resource.
// It is idempotent: if no rollback annotation is present, it returns nil.
func (f *FinalizerBlockInjector) Revert(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) error {
	apiVersion := spec.Parameters["apiVersion"]
	if apiVersion == "" {
		apiVersion = "v1"
	}
	kind := spec.Parameters["kind"]
	name := spec.Parameters["name"]

	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(apiVersion)
	obj.SetKind(kind)

	key := types.NamespacedName{Name: name, Namespace: namespace}

	if err := f.client.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("getting resource %s/%s for revert: %w", kind, name, err)
	}

	// Check for rollback annotation — if absent, already reverted
	rollbackStr, ok := obj.GetAnnotations()[safety.RollbackAnnotationKey]
	if !ok {
		return nil
	}

	var rollbackData map[string]string
	if err := safety.UnwrapRollbackData(rollbackStr, &rollbackData); err != nil {
		return fmt.Errorf("unwrapping rollback data for %s/%s: %w", kind, name, err)
	}

	finalizerName := rollbackData["finalizer"]
	changed := controllerutil.RemoveFinalizer(obj, finalizerName)

	// Remove chaos metadata
	if _, hasAnnotation := obj.GetAnnotations()[safety.RollbackAnnotationKey]; hasAnnotation {
		changed = true
	}
	for k := range safety.ChaosLabels(string(v1alpha1.FinalizerBlock)) {
		if _, ok := obj.GetLabels()[k]; ok {
			changed = true
		}
	}
	safety.RemoveChaosMetadata(obj, string(v1alpha1.FinalizerBlock))

	if changed {
		if err := f.client.Update(ctx, obj); err != nil {
			return fmt.Errorf("removing finalizer from %s/%s during revert: %w", kind, name, err)
		}
	}

	return nil
}
