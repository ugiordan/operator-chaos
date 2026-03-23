package injection

import (
	"context"
	"testing"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRBACRevokeValidate(t *testing.T) {
	injector := NewRBACRevokeInjector(nil)
	blast := v1alpha1.BlastRadiusSpec{MaxPodsAffected: 1, AllowedNamespaces: []string{"test"}}

	tests := []struct {
		name    string
		spec    v1alpha1.InjectionSpec
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid ClusterRoleBinding",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.RBACRevoke,
				Parameters: map[string]string{
					"bindingName": "my-operator-binding",
					"bindingType": "ClusterRoleBinding",
				},
			},
			wantErr: false,
		},
		{
			name: "valid RoleBinding",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.RBACRevoke,
				Parameters: map[string]string{
					"bindingName": "my-operator-binding",
					"bindingType": "RoleBinding",
				},
			},
			wantErr: false,
		},
		{
			name: "missing bindingName",
			spec: v1alpha1.InjectionSpec{
				Type:       v1alpha1.RBACRevoke,
				Parameters: map[string]string{"bindingType": "ClusterRoleBinding"},
			},
			wantErr: true,
			errMsg:  "bindingName",
		},
		{
			name: "invalid bindingType",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.RBACRevoke,
				Parameters: map[string]string{
					"bindingName": "test",
					"bindingType": "Invalid",
				},
			},
			wantErr: true,
			errMsg:  "bindingType",
		},
		{
			name: "missing bindingType defaults valid",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.RBACRevoke,
				Parameters: map[string]string{
					"bindingName": "test",
				},
			},
			wantErr: true,
			errMsg:  "bindingType",
		},
		{
			name: "nil parameters",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.RBACRevoke,
			},
			wantErr: true,
			errMsg:  "bindingName",
		},
		{
			name: "invalid binding name",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.RBACRevoke,
				Parameters: map[string]string{
					"bindingName": "INVALID NAME!",
					"bindingType": "ClusterRoleBinding",
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

func TestRBACRevokeInjectAndCleanup(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, rbacv1.AddToScheme(scheme))

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "my-operator-binding"},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "my-operator-role",
		},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "my-operator", Namespace: "opendatahub"},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crb).Build()
	injector := NewRBACRevokeInjector(k8sClient)

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.RBACRevoke,
		Parameters: map[string]string{
			"bindingName": "my-operator-binding",
			"bindingType": "ClusterRoleBinding",
		},
	}

	cleanup, events, err := injector.Inject(context.Background(), spec, "opendatahub")
	require.NoError(t, err)
	assert.NotEmpty(t, events)
	assert.NotNil(t, cleanup)

	// Verify subjects were cleared
	modified := &rbacv1.ClusterRoleBinding{}
	require.NoError(t, k8sClient.Get(context.Background(),
		client.ObjectKey{Name: "my-operator-binding"}, modified))
	assert.Empty(t, modified.Subjects)

	// Cleanup should restore
	require.NoError(t, cleanup(context.Background()))
	restored := &rbacv1.ClusterRoleBinding{}
	require.NoError(t, k8sClient.Get(context.Background(),
		client.ObjectKey{Name: "my-operator-binding"}, restored))
	assert.Len(t, restored.Subjects, 1)
	assert.Equal(t, "my-operator", restored.Subjects[0].Name)
}

func TestRBACRevokeInjectRoleBinding(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, rbacv1.AddToScheme(scheme))

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-role-binding",
			Namespace: "opendatahub",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "my-role",
		},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "my-sa", Namespace: "opendatahub"},
			{Kind: "ServiceAccount", Name: "other-sa", Namespace: "opendatahub"},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rb).Build()
	injector := NewRBACRevokeInjector(k8sClient)

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.RBACRevoke,
		Parameters: map[string]string{
			"bindingName": "my-role-binding",
			"bindingType": "RoleBinding",
		},
	}

	ctx := context.Background()
	cleanup, events, err := injector.Inject(ctx, spec, "opendatahub")
	require.NoError(t, err)
	assert.NotEmpty(t, events)
	assert.NotNil(t, cleanup)

	// Verify subjects were cleared
	modified := &rbacv1.RoleBinding{}
	require.NoError(t, k8sClient.Get(ctx,
		client.ObjectKey{Name: "my-role-binding", Namespace: "opendatahub"}, modified))
	assert.Empty(t, modified.Subjects)

	// Cleanup should restore all subjects
	require.NoError(t, cleanup(ctx))
	restored := &rbacv1.RoleBinding{}
	require.NoError(t, k8sClient.Get(ctx,
		client.ObjectKey{Name: "my-role-binding", Namespace: "opendatahub"}, restored))
	assert.Len(t, restored.Subjects, 2)
	assert.Equal(t, "my-sa", restored.Subjects[0].Name)
	assert.Equal(t, "other-sa", restored.Subjects[1].Name)
}

func TestRBACRevokeNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, rbacv1.AddToScheme(scheme))

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	injector := NewRBACRevokeInjector(k8sClient)

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.RBACRevoke,
		Parameters: map[string]string{
			"bindingName": "nonexistent-binding",
			"bindingType": "ClusterRoleBinding",
		},
	}

	_, _, err := injector.Inject(context.Background(), spec, "default")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent-binding")
}

func TestRBACRevokeRevertClusterRoleBinding(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, rbacv1.AddToScheme(scheme))

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "revert-crb"},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "my-role",
		},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "my-sa", Namespace: "opendatahub"},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crb).Build()
	injector := NewRBACRevokeInjector(k8sClient)
	ctx := context.Background()

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.RBACRevoke,
		Parameters: map[string]string{
			"bindingName": "revert-crb",
			"bindingType": "ClusterRoleBinding",
		},
	}

	// Inject
	_, _, err := injector.Inject(ctx, spec, "")
	require.NoError(t, err)

	// Verify subjects cleared
	modified := &rbacv1.ClusterRoleBinding{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "revert-crb"}, modified))
	assert.Empty(t, modified.Subjects)

	// Revert
	err = injector.Revert(ctx, spec, "")
	require.NoError(t, err)

	// Verify subjects restored
	restored := &rbacv1.ClusterRoleBinding{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "revert-crb"}, restored))
	require.Len(t, restored.Subjects, 1)
	assert.Equal(t, "my-sa", restored.Subjects[0].Name)

	// Idempotent
	err = injector.Revert(ctx, spec, "")
	assert.NoError(t, err)
}

func TestRBACRevokeRevertRoleBinding(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, rbacv1.AddToScheme(scheme))

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "revert-rb",
			Namespace: "opendatahub",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "my-role",
		},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "my-sa", Namespace: "opendatahub"},
			{Kind: "ServiceAccount", Name: "other-sa", Namespace: "opendatahub"},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rb).Build()
	injector := NewRBACRevokeInjector(k8sClient)
	ctx := context.Background()

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.RBACRevoke,
		Parameters: map[string]string{
			"bindingName": "revert-rb",
			"bindingType": "RoleBinding",
		},
	}

	// Inject
	_, _, err := injector.Inject(ctx, spec, "opendatahub")
	require.NoError(t, err)

	// Verify subjects cleared
	modified := &rbacv1.RoleBinding{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "revert-rb", Namespace: "opendatahub"}, modified))
	assert.Empty(t, modified.Subjects)

	// Verify chaos annotations/labels present
	_, hasAnnotation := modified.Annotations[safety.RollbackAnnotationKey]
	assert.True(t, hasAnnotation, "rollback annotation should be present after injection")
	assert.Equal(t, safety.ManagedByValue, modified.Labels[safety.ManagedByLabel])

	// Revert
	err = injector.Revert(ctx, spec, "opendatahub")
	require.NoError(t, err)

	// Verify subjects restored
	restored := &rbacv1.RoleBinding{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "revert-rb", Namespace: "opendatahub"}, restored))
	require.Len(t, restored.Subjects, 2)
	assert.Equal(t, "my-sa", restored.Subjects[0].Name)
	assert.Equal(t, "other-sa", restored.Subjects[1].Name)

	// Verify chaos annotations removed
	_, hasAnnotation = restored.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasAnnotation, "rollback annotation should be removed after revert")
	_, hasManagedBy := restored.Labels[safety.ManagedByLabel]
	assert.False(t, hasManagedBy, "managed-by label should be removed after revert")
	_, hasChaosType := restored.Labels[safety.ChaosTypeLabel]
	assert.False(t, hasChaosType, "chaos-type label should be removed after revert")

	// Idempotent — second Revert is a no-op
	err = injector.Revert(ctx, spec, "opendatahub")
	assert.NoError(t, err)
}

func TestRBACRevokeInjectStoresRollbackAnnotation(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, rbacv1.AddToScheme(scheme))

	originalSubjects := []rbacv1.Subject{
		{Kind: "ServiceAccount", Name: "operator-sa", Namespace: "opendatahub"},
		{Kind: "User", Name: "admin-user"},
	}

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rollback-binding"},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "test-role",
		},
		Subjects: originalSubjects,
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crb).Build()
	injector := NewRBACRevokeInjector(k8sClient)

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.RBACRevoke,
		Parameters: map[string]string{
			"bindingName": "test-rollback-binding",
			"bindingType": "ClusterRoleBinding",
		},
	}

	ctx := context.Background()
	cleanup, events, err := injector.Inject(ctx, spec, "")
	require.NoError(t, err)
	assert.NotEmpty(t, events)
	assert.NotNil(t, cleanup)

	// Verify the rollback annotation exists and contains the original subjects
	modified := &rbacv1.ClusterRoleBinding{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "test-rollback-binding"}, modified))
	assert.Empty(t, modified.Subjects)

	rollbackData, ok := modified.Annotations[safety.RollbackAnnotationKey]
	require.True(t, ok, "rollback annotation should be present")

	var storedSubjects []rbacv1.Subject
	require.NoError(t, safety.UnwrapRollbackData(rollbackData, &storedSubjects))
	assert.Len(t, storedSubjects, 2)
	assert.Equal(t, "operator-sa", storedSubjects[0].Name)
	assert.Equal(t, "admin-user", storedSubjects[1].Name)

	// Verify chaos labels are set
	assert.Equal(t, safety.ManagedByValue, modified.Labels[safety.ManagedByLabel])
	assert.Equal(t, string(v1alpha1.RBACRevoke), modified.Labels[safety.ChaosTypeLabel])

	// Run cleanup and verify annotation and labels are removed
	require.NoError(t, cleanup(ctx))

	restored := &rbacv1.ClusterRoleBinding{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "test-rollback-binding"}, restored))
	assert.Len(t, restored.Subjects, 2)
	assert.Equal(t, "operator-sa", restored.Subjects[0].Name)
	assert.Equal(t, "admin-user", restored.Subjects[1].Name)

	// Rollback annotation should be removed
	_, hasAnnotation := restored.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasAnnotation, "rollback annotation should be removed after cleanup")

	// Chaos labels should be removed
	_, hasManagedBy := restored.Labels[safety.ManagedByLabel]
	assert.False(t, hasManagedBy, "managed-by label should be removed after cleanup")
	_, hasChaosType := restored.Labels[safety.ChaosTypeLabel]
	assert.False(t, hasChaosType, "chaos-type label should be removed after cleanup")
}
