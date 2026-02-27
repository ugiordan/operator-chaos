package injection

import (
	"context"
	"encoding/json"
	"fmt"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RBACRevokeInjector temporarily revokes RBAC permissions by clearing subjects
// from ClusterRoleBindings or RoleBindings. The cleanup function restores the
// original subjects.
type RBACRevokeInjector struct {
	client client.Client
}

// NewRBACRevokeInjector creates a new RBACRevokeInjector.
func NewRBACRevokeInjector(c client.Client) *RBACRevokeInjector {
	return &RBACRevokeInjector{client: c}
}

// Validate checks that the injection spec contains the required parameters:
// - bindingName: the name of the binding to revoke
// - bindingType: must be "ClusterRoleBinding" or "RoleBinding"
func (r *RBACRevokeInjector) Validate(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error {
	if _, ok := spec.Parameters["bindingName"]; !ok {
		return fmt.Errorf("RBACRevoke requires 'bindingName' parameter")
	}

	bindingType, ok := spec.Parameters["bindingType"]
	if !ok {
		return fmt.Errorf("RBACRevoke requires 'bindingType' parameter")
	}

	if bindingType != "ClusterRoleBinding" && bindingType != "RoleBinding" {
		return fmt.Errorf("RBACRevoke bindingType must be 'ClusterRoleBinding' or 'RoleBinding', got %q", bindingType)
	}

	return nil
}

// Inject performs the RBAC revocation:
// 1. Fetches the binding (ClusterRoleBinding or RoleBinding) by name
// 2. Saves the original subjects
// 3. Clears subjects (empty slice)
// 4. Updates the binding
// 5. Returns a cleanup function that restores the original subjects
func (r *RBACRevokeInjector) Inject(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) (CleanupFunc, []v1alpha1.InjectionEvent, error) {
	bindingName := spec.Parameters["bindingName"]
	bindingType := spec.Parameters["bindingType"]

	switch bindingType {
	case "ClusterRoleBinding":
		return r.injectClusterRoleBinding(ctx, bindingName)
	case "RoleBinding":
		return r.injectRoleBinding(ctx, bindingName, namespace)
	default:
		return nil, nil, fmt.Errorf("unsupported bindingType %q", bindingType)
	}
}

// injectClusterRoleBinding handles the injection for cluster-scoped ClusterRoleBindings.
func (r *RBACRevokeInjector) injectClusterRoleBinding(ctx context.Context, bindingName string) (CleanupFunc, []v1alpha1.InjectionEvent, error) {
	// Fetch the ClusterRoleBinding
	crb := &rbacv1.ClusterRoleBinding{}
	if err := r.client.Get(ctx, client.ObjectKey{Name: bindingName}, crb); err != nil {
		return nil, nil, fmt.Errorf("getting ClusterRoleBinding %q: %w", bindingName, err)
	}

	// Save original subjects
	originalSubjects := make([]rbacv1.Subject, len(crb.Subjects))
	copy(originalSubjects, crb.Subjects)

	// Serialize original subjects to JSON for crash-safe rollback
	rollbackData, err := json.Marshal(originalSubjects)
	if err != nil {
		return nil, nil, fmt.Errorf("serializing original subjects for ClusterRoleBinding %q: %w", bindingName, err)
	}

	// Store rollback annotation and chaos labels
	if crb.Annotations == nil {
		crb.Annotations = make(map[string]string)
	}
	crb.Annotations[safety.RollbackAnnotationKey] = string(rollbackData)

	if crb.Labels == nil {
		crb.Labels = make(map[string]string)
	}
	for k, v := range safety.ChaosLabels(string(v1alpha1.RBACRevoke)) {
		crb.Labels[k] = v
	}

	// Clear subjects
	crb.Subjects = []rbacv1.Subject{}

	// Update the binding
	if err := r.client.Update(ctx, crb); err != nil {
		return nil, nil, fmt.Errorf("updating ClusterRoleBinding %q: %w", bindingName, err)
	}

	events := []v1alpha1.InjectionEvent{
		NewEvent(v1alpha1.RBACRevoke, bindingName, "revokeSubjects",
			map[string]string{
				"bindingName":    bindingName,
				"bindingType":    "ClusterRoleBinding",
				"subjectsCleared": fmt.Sprintf("%d", len(originalSubjects)),
			}),
	}

	// Cleanup restores original subjects and removes rollback metadata
	cleanup := func(ctx context.Context) error {
		current := &rbacv1.ClusterRoleBinding{}
		if err := r.client.Get(ctx, client.ObjectKey{Name: bindingName}, current); err != nil {
			return fmt.Errorf("re-fetching ClusterRoleBinding %q for cleanup: %w", bindingName, err)
		}

		current.Subjects = originalSubjects

		// Remove rollback annotation
		delete(current.Annotations, safety.RollbackAnnotationKey)

		// Remove chaos labels
		for k := range safety.ChaosLabels(string(v1alpha1.RBACRevoke)) {
			delete(current.Labels, k)
		}

		return r.client.Update(ctx, current)
	}

	return cleanup, events, nil
}

// injectRoleBinding handles the injection for namespace-scoped RoleBindings.
func (r *RBACRevokeInjector) injectRoleBinding(ctx context.Context, bindingName, namespace string) (CleanupFunc, []v1alpha1.InjectionEvent, error) {
	// Fetch the RoleBinding
	rb := &rbacv1.RoleBinding{}
	if err := r.client.Get(ctx, client.ObjectKey{Name: bindingName, Namespace: namespace}, rb); err != nil {
		return nil, nil, fmt.Errorf("getting RoleBinding %q in namespace %q: %w", bindingName, namespace, err)
	}

	// Save original subjects
	originalSubjects := make([]rbacv1.Subject, len(rb.Subjects))
	copy(originalSubjects, rb.Subjects)

	// Serialize original subjects to JSON for crash-safe rollback
	rollbackData, err := json.Marshal(originalSubjects)
	if err != nil {
		return nil, nil, fmt.Errorf("serializing original subjects for RoleBinding %q: %w", bindingName, err)
	}

	// Store rollback annotation and chaos labels
	if rb.Annotations == nil {
		rb.Annotations = make(map[string]string)
	}
	rb.Annotations[safety.RollbackAnnotationKey] = string(rollbackData)

	if rb.Labels == nil {
		rb.Labels = make(map[string]string)
	}
	for k, v := range safety.ChaosLabels(string(v1alpha1.RBACRevoke)) {
		rb.Labels[k] = v
	}

	// Clear subjects
	rb.Subjects = []rbacv1.Subject{}

	// Update the binding
	if err := r.client.Update(ctx, rb); err != nil {
		return nil, nil, fmt.Errorf("updating RoleBinding %q in namespace %q: %w", bindingName, namespace, err)
	}

	events := []v1alpha1.InjectionEvent{
		NewEvent(v1alpha1.RBACRevoke, bindingName, "revokeSubjects",
			map[string]string{
				"bindingName":    bindingName,
				"bindingType":    "RoleBinding",
				"namespace":      namespace,
				"subjectsCleared": fmt.Sprintf("%d", len(originalSubjects)),
			}),
	}

	// Cleanup restores original subjects and removes rollback metadata
	cleanup := func(ctx context.Context) error {
		current := &rbacv1.RoleBinding{}
		if err := r.client.Get(ctx, client.ObjectKey{Name: bindingName, Namespace: namespace}, current); err != nil {
			return fmt.Errorf("re-fetching RoleBinding %q in namespace %q for cleanup: %w", bindingName, namespace, err)
		}

		current.Subjects = originalSubjects

		// Remove rollback annotation
		delete(current.Annotations, safety.RollbackAnnotationKey)

		// Remove chaos labels
		for k := range safety.ChaosLabels(string(v1alpha1.RBACRevoke)) {
			delete(current.Labels, k)
		}

		return r.client.Update(ctx, current)
	}

	return cleanup, events, nil
}
