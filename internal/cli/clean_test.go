package cli

import (
	"context"
	"testing"
	"time"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newTestScheme registers all resource types used by the clean functions.
func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = admissionregv1.AddToScheme(scheme)
	_ = coordinationv1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	return scheme
}

// mustMarshal marshals v to JSON wrapped in an integrity envelope (test helper only).
func mustMarshal(t *testing.T, v interface{}) string {
	t.Helper()
	wrapped, err := safety.WrapRollbackData(v)
	require.NoError(t, err)
	return wrapped
}

// chaosLabelsFor returns a full set of chaos labels for the given injection type.
func chaosLabelsFor(injectionType v1alpha1.InjectionType) map[string]string {
	return safety.ChaosLabels(string(injectionType))
}

// ---------------------------------------------------------------------------
// Task 1: cleanFinalizerFromResource and cleanOrphanedFinalizers
// ---------------------------------------------------------------------------

func TestCleanFinalizerFromResource_WithFinalizerRollback(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	rollbackData := mustMarshal(t, map[string]string{
		"finalizer": "chaos.opendatahub.io/block",
	})

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-config",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels:     chaosLabelsFor(v1alpha1.FinalizerBlock),
			Finalizers: []string{"chaos.opendatahub.io/block"},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	result := cleanFinalizerFromResource(ctx, k8sClient, cm, cm.Name, cm.Namespace)
	assert.True(t, result, "should return true when finalizer is cleaned")

	// Verify: finalizer removed, annotation removed, labels removed
	updated := &corev1.ConfigMap{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "my-config", Namespace: "default"}, updated))

	assert.NotContains(t, updated.Finalizers, "chaos.opendatahub.io/block",
		"chaos finalizer should be removed")
	_, hasRollback := updated.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasRollback, "rollback annotation should be removed")
	_, hasManagedBy := updated.Labels[safety.ManagedByLabel]
	assert.False(t, hasManagedBy, "managed-by label should be removed")
	_, hasChaosType := updated.Labels[safety.ChaosTypeLabel]
	assert.False(t, hasChaosType, "chaos-type label should be removed")
}

func TestCleanFinalizerFromResource_WithoutAnnotations(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-annotations",
			Namespace: "default",
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	result := cleanFinalizerFromResource(ctx, k8sClient, cm, cm.Name, cm.Namespace)
	assert.False(t, result, "should return false when no annotations present")
}

func TestCleanFinalizerFromResource_WithNonFinalizerRollback(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	// ConfigDrift rollback data does not have a "finalizer" key
	rollbackData := mustMarshal(t, map[string]string{
		"resourceType":  "ConfigMap",
		"key":           "some-key",
		"originalValue": "some-value",
	})

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "config-drift-resource",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels: chaosLabelsFor(v1alpha1.ConfigDrift),
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	result := cleanFinalizerFromResource(ctx, k8sClient, cm, cm.Name, cm.Namespace)
	assert.False(t, result, "should return false for non-finalizer rollback data")
}

func TestCleanOrphanedFinalizers_MultipleResourceTypes(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	rollbackData := mustMarshal(t, map[string]string{
		"finalizer": "chaos.opendatahub.io/block",
	})
	labels := chaosLabelsFor(v1alpha1.FinalizerBlock)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deploy",
			Namespace: "test-ns",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels:     labels,
			Finalizers: []string{"chaos.opendatahub.io/block"},
		},
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "test-ns",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels:     labels,
			Finalizers: []string{"chaos.opendatahub.io/block"},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-ns",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels:     labels,
			Finalizers: []string{"chaos.opendatahub.io/block"},
		},
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels:     labels,
			Finalizers: []string{"chaos.opendatahub.io/block"},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Port: 80}},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dep, cm, secret, svc).
		Build()

	cleaned := cleanOrphanedFinalizers(ctx, k8sClient, "test-ns")
	assert.Equal(t, 4, cleaned, "should clean all 4 resource types")

	// Verify each resource was actually cleaned
	updatedDep := &appsv1.Deployment{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "test-deploy", Namespace: "test-ns"}, updatedDep))
	assert.NotContains(t, updatedDep.Finalizers, "chaos.opendatahub.io/block")
	_, hasRollback := updatedDep.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasRollback)

	updatedCM := &corev1.ConfigMap{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "test-cm", Namespace: "test-ns"}, updatedCM))
	assert.NotContains(t, updatedCM.Finalizers, "chaos.opendatahub.io/block")

	updatedSecret := &corev1.Secret{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "test-secret", Namespace: "test-ns"}, updatedSecret))
	assert.NotContains(t, updatedSecret.Finalizers, "chaos.opendatahub.io/block")

	updatedSvc := &corev1.Service{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "test-svc", Namespace: "test-ns"}, updatedSvc))
	assert.NotContains(t, updatedSvc.Finalizers, "chaos.opendatahub.io/block")
}

func TestCleanOrphanedFinalizers_NamespaceWithNoOrphans(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	// Resources without rollback annotations
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "clean-cm",
			Namespace: "clean-ns",
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	cleaned := cleanOrphanedFinalizers(ctx, k8sClient, "clean-ns")
	assert.Equal(t, 0, cleaned, "should return 0 when no orphans exist")
}

func TestCleanOrphanedFinalizers_EmptyNamespace(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	cleaned := cleanOrphanedFinalizers(ctx, k8sClient, "empty-ns")
	assert.Equal(t, 0, cleaned, "should return 0 for empty namespace")
}

// ---------------------------------------------------------------------------
// Task 2: cleanConfigDrift, cleanWebhookConfigurations, cleanRBACBindings,
//         cleanTTLExpired
// ---------------------------------------------------------------------------

func TestCleanConfigDrift_ConfigMapWithDriftRollback(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	rollbackData := mustMarshal(t, map[string]string{
		"resourceType":  "ConfigMap",
		"key":           "app.conf",
		"originalValue": "original-data",
	})

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "drifted-cm",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels: chaosLabelsFor(v1alpha1.ConfigDrift),
		},
		Data: map[string]string{
			"app.conf": "corrupted-data",
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	restored := cleanConfigDrift(ctx, k8sClient, "default")
	assert.Equal(t, 1, restored, "should restore 1 ConfigMap")

	updated := &corev1.ConfigMap{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "drifted-cm", Namespace: "default"}, updated))
	assert.Equal(t, "original-data", updated.Data["app.conf"], "data should be restored to original value")
	_, hasRollback := updated.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasRollback, "rollback annotation should be removed")
	_, hasManagedBy := updated.Labels[safety.ManagedByLabel]
	assert.False(t, hasManagedBy, "managed-by label should be removed")
	_, hasChaosType := updated.Labels[safety.ChaosTypeLabel]
	assert.False(t, hasChaosType, "chaos-type label should be removed")
}

func TestCleanConfigDrift_SecretWithDriftRollback(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	rollbackData := mustMarshal(t, map[string]string{
		"resourceType":  "Secret",
		"key":           "password",
		"originalValue": "original-secret",
	})

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "drifted-secret",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels: chaosLabelsFor(v1alpha1.ConfigDrift),
		},
		Data: map[string][]byte{
			"password": []byte("corrupted-secret"),
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	restored := cleanConfigDrift(ctx, k8sClient, "default")
	assert.Equal(t, 1, restored, "should restore 1 Secret")

	updated := &corev1.Secret{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "drifted-secret", Namespace: "default"}, updated))
	assert.Equal(t, []byte("original-secret"), updated.Data["password"], "data should be restored to original value")
	_, hasRollback := updated.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasRollback, "rollback annotation should be removed")
}

func TestCleanConfigDrift_NoAnnotations(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "clean-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"key": "value",
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	restored := cleanConfigDrift(ctx, k8sClient, "default")
	assert.Equal(t, 0, restored, "should return 0 when no drift rollback annotations exist")
}

func TestCleanConfigDrift_ConfigMapWithNilData(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	rollbackData := mustMarshal(t, map[string]string{
		"resourceType":  "ConfigMap",
		"key":           "new-key",
		"originalValue": "original-value",
	})

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nil-data-cm",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels: chaosLabelsFor(v1alpha1.ConfigDrift),
		},
		// Data is nil
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	restored := cleanConfigDrift(ctx, k8sClient, "default")
	assert.Equal(t, 1, restored, "should restore even when Data is nil")

	updated := &corev1.ConfigMap{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "nil-data-cm", Namespace: "default"}, updated))
	assert.Equal(t, "original-value", updated.Data["new-key"], "data should be initialized and set")
}

func TestCleanWebhookConfigurations_WithRollback(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	failPolicy := admissionregv1.Fail
	rollbackData := mustMarshal(t, map[string]string{
		"test.webhook.io": "Ignore",
	})

	webhook := &admissionregv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-webhook",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels: chaosLabelsFor(v1alpha1.WebhookDisrupt),
		},
		Webhooks: []admissionregv1.ValidatingWebhook{
			{
				Name:                    "test.webhook.io",
				FailurePolicy:           &failPolicy,
				ClientConfig:            admissionregv1.WebhookClientConfig{URL: strPtr("https://example.com")},
				SideEffects:             sideEffectPtr(admissionregv1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1"},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(webhook).Build()

	restored := cleanWebhookConfigurations(ctx, k8sClient)
	assert.Equal(t, 1, restored, "should restore 1 webhook configuration")

	updated := &admissionregv1.ValidatingWebhookConfiguration{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "my-webhook"}, updated))

	require.NotNil(t, updated.Webhooks[0].FailurePolicy)
	assert.Equal(t, admissionregv1.Ignore, *updated.Webhooks[0].FailurePolicy,
		"failure policy should be restored to Ignore")
	_, hasRollback := updated.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasRollback, "rollback annotation should be removed")
	_, hasManagedBy := updated.Labels[safety.ManagedByLabel]
	assert.False(t, hasManagedBy, "managed-by label should be removed")
	_, hasChaosType := updated.Labels[safety.ChaosTypeLabel]
	assert.False(t, hasChaosType, "chaos-type label should be removed")
}

func TestCleanWebhookConfigurations_NoRollback(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	ignorePolicy := admissionregv1.Ignore
	webhook := &admissionregv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "clean-webhook",
		},
		Webhooks: []admissionregv1.ValidatingWebhook{
			{
				Name:                    "clean.webhook.io",
				FailurePolicy:           &ignorePolicy,
				ClientConfig:            admissionregv1.WebhookClientConfig{URL: strPtr("https://example.com")},
				SideEffects:             sideEffectPtr(admissionregv1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1"},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(webhook).Build()

	restored := cleanWebhookConfigurations(ctx, k8sClient)
	assert.Equal(t, 0, restored, "should return 0 when no rollback annotations exist")

	// Verify the webhook was not modified
	updated := &admissionregv1.ValidatingWebhookConfiguration{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "clean-webhook"}, updated))
	require.NotNil(t, updated.Webhooks[0].FailurePolicy)
	assert.Equal(t, admissionregv1.Ignore, *updated.Webhooks[0].FailurePolicy,
		"failure policy should remain unchanged")
}

func TestCleanWebhookConfigurations_MultipleWebhooksInConfig(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	failPolicy := admissionregv1.Fail
	rollbackData := mustMarshal(t, map[string]string{
		"first.webhook.io":  "Ignore",
		"second.webhook.io": "Fail",
	})

	webhook := &admissionregv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "multi-webhook",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels: chaosLabelsFor(v1alpha1.WebhookDisrupt),
		},
		Webhooks: []admissionregv1.ValidatingWebhook{
			{
				Name:                    "first.webhook.io",
				FailurePolicy:           &failPolicy,
				ClientConfig:            admissionregv1.WebhookClientConfig{URL: strPtr("https://example.com/first")},
				SideEffects:             sideEffectPtr(admissionregv1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1"},
			},
			{
				Name:                    "second.webhook.io",
				FailurePolicy:           &failPolicy,
				ClientConfig:            admissionregv1.WebhookClientConfig{URL: strPtr("https://example.com/second")},
				SideEffects:             sideEffectPtr(admissionregv1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1"},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(webhook).Build()

	restored := cleanWebhookConfigurations(ctx, k8sClient)
	assert.Equal(t, 1, restored, "should restore 1 webhook configuration")

	updated := &admissionregv1.ValidatingWebhookConfiguration{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "multi-webhook"}, updated))

	require.NotNil(t, updated.Webhooks[0].FailurePolicy)
	assert.Equal(t, admissionregv1.Ignore, *updated.Webhooks[0].FailurePolicy,
		"first webhook should be restored to Ignore")

	require.NotNil(t, updated.Webhooks[1].FailurePolicy)
	assert.Equal(t, admissionregv1.Fail, *updated.Webhooks[1].FailurePolicy,
		"second webhook should be restored to Fail")
}

func TestCleanRBACBindings_ClusterRoleBindingWithRollback(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	originalSubjects := []rbacv1.Subject{
		{Kind: "ServiceAccount", Name: "my-sa", Namespace: "default"},
		{Kind: "User", Name: "admin"},
	}
	rollbackData := mustMarshal(t, originalSubjects)

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "chaos-crb",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels: chaosLabelsFor(v1alpha1.RBACRevoke),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "some-role",
		},
		Subjects: []rbacv1.Subject{}, // emptied by chaos injection
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crb).Build()

	restored := cleanRBACBindings(ctx, k8sClient, "")
	assert.Equal(t, 1, restored, "should restore 1 ClusterRoleBinding")

	updated := &rbacv1.ClusterRoleBinding{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "chaos-crb"}, updated))

	assert.Len(t, updated.Subjects, 2, "subjects should be restored")
	assert.Equal(t, "my-sa", updated.Subjects[0].Name)
	assert.Equal(t, "admin", updated.Subjects[1].Name)

	_, hasRollback := updated.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasRollback, "rollback annotation should be removed")
	_, hasManagedBy := updated.Labels[safety.ManagedByLabel]
	assert.False(t, hasManagedBy, "managed-by label should be removed")
}

func TestCleanRBACBindings_RoleBindingWithRollback(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	originalSubjects := []rbacv1.Subject{
		{Kind: "ServiceAccount", Name: "app-sa", Namespace: "app-ns"},
	}
	rollbackData := mustMarshal(t, originalSubjects)

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-rb",
			Namespace: "app-ns",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels: chaosLabelsFor(v1alpha1.RBACRevoke),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "some-role",
		},
		Subjects: []rbacv1.Subject{}, // emptied by chaos injection
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rb).Build()

	restored := cleanRBACBindings(ctx, k8sClient, "app-ns")
	assert.Equal(t, 1, restored, "should restore 1 RoleBinding")

	updated := &rbacv1.RoleBinding{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "chaos-rb", Namespace: "app-ns"}, updated))

	assert.Len(t, updated.Subjects, 1, "subjects should be restored")
	assert.Equal(t, "app-sa", updated.Subjects[0].Name)

	_, hasRollback := updated.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasRollback, "rollback annotation should be removed")
}

func TestCleanRBACBindings_NoRollback(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "clean-crb",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "some-role",
		},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "sa", Namespace: "default"},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crb).Build()
	restored := cleanRBACBindings(ctx, k8sClient, "")
	assert.Equal(t, 0, restored, "should return 0 when no rollback annotations exist")
}

func TestCleanRBACBindings_BothClusterAndNamespacedRoleBindings(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	crbSubjects := []rbacv1.Subject{
		{Kind: "User", Name: "cluster-admin"},
	}
	crbRollback := mustMarshal(t, crbSubjects)

	rbSubjects := []rbacv1.Subject{
		{Kind: "ServiceAccount", Name: "ns-sa", Namespace: "test-ns"},
	}
	rbRollback := mustMarshal(t, rbSubjects)

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "crb-with-rollback",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: crbRollback,
			},
			Labels: chaosLabelsFor(v1alpha1.RBACRevoke),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "some-role",
		},
		Subjects: []rbacv1.Subject{},
	}

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rb-with-rollback",
			Namespace: "test-ns",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rbRollback,
			},
			Labels: chaosLabelsFor(v1alpha1.RBACRevoke),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "some-role",
		},
		Subjects: []rbacv1.Subject{},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crb, rb).Build()

	restored := cleanRBACBindings(ctx, k8sClient, "test-ns")
	assert.Equal(t, 2, restored, "should restore both CRB and RB")
}

func TestCleanTTLExpired_ExpiredNetworkPolicy(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	expiredTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)

	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "expired-policy",
			Namespace: "default",
			Annotations: map[string]string{
				safety.TTLAnnotationKey: expiredTime,
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(np).Build()

	cleaned := cleanTTLExpired(ctx, k8sClient, "default")
	assert.Equal(t, 1, cleaned, "should delete 1 expired NetworkPolicy")

	// Verify deletion
	policies := &networkingv1.NetworkPolicyList{}
	require.NoError(t, k8sClient.List(ctx, policies, client.InNamespace("default")))
	assert.Empty(t, policies.Items, "expired NetworkPolicy should be deleted")
}

func TestCleanTTLExpired_FutureNetworkPolicy(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	futureTime := time.Now().Add(1 * time.Hour).Format(time.RFC3339)

	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "future-policy",
			Namespace: "default",
			Annotations: map[string]string{
				safety.TTLAnnotationKey: futureTime,
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(np).Build()

	cleaned := cleanTTLExpired(ctx, k8sClient, "default")
	assert.Equal(t, 0, cleaned, "should not delete NetworkPolicy with future TTL")

	// Verify the policy still exists
	policies := &networkingv1.NetworkPolicyList{}
	require.NoError(t, k8sClient.List(ctx, policies, client.InNamespace("default")))
	assert.Len(t, policies.Items, 1, "NetworkPolicy with future TTL should not be deleted")
}

func TestCleanTTLExpired_NoTTLAnnotation(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-ttl-policy",
			Namespace: "default",
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(np).Build()

	cleaned := cleanTTLExpired(ctx, k8sClient, "default")
	assert.Equal(t, 0, cleaned, "should return 0 when no TTL annotation exists")
}

func TestCleanTTLExpired_MixedPolicies(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	expiredTime := time.Now().Add(-30 * time.Minute).Format(time.RFC3339)
	futureTime := time.Now().Add(30 * time.Minute).Format(time.RFC3339)

	expired := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "expired-np",
			Namespace: "test-ns",
			Annotations: map[string]string{
				safety.TTLAnnotationKey: expiredTime,
			},
		},
	}

	future := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "future-np",
			Namespace: "test-ns",
			Annotations: map[string]string{
				safety.TTLAnnotationKey: futureTime,
			},
		},
	}

	noTTL := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-ttl-np",
			Namespace: "test-ns",
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(expired, future, noTTL).Build()

	cleaned := cleanTTLExpired(ctx, k8sClient, "test-ns")
	assert.Equal(t, 1, cleaned, "should delete only the expired policy")

	policies := &networkingv1.NetworkPolicyList{}
	require.NoError(t, k8sClient.List(ctx, policies, client.InNamespace("test-ns")))
	assert.Len(t, policies.Items, 2, "2 policies should remain (future + no-ttl)")
}

// ---------------------------------------------------------------------------
// Task 3: runClean integration and cleanSummary
// ---------------------------------------------------------------------------

func TestRunClean_NilClient(t *testing.T) {
	ctx := context.Background()

	summary := runClean(ctx, nil, "default")
	assert.Equal(t, 0, summary.total(), "nil client should return zero summary")
	assert.Equal(t, 0, summary.NetworkPolicies)
	assert.Equal(t, 0, summary.Leases)
	assert.Equal(t, 0, summary.ClusterRoles)
	assert.Equal(t, 0, summary.RoleBindings)
	assert.Equal(t, 0, summary.TTLExpired)
	assert.Equal(t, 0, summary.WebhooksRestored)
	assert.Equal(t, 0, summary.RBACBindingsFixed)
	assert.Equal(t, 0, summary.FinalizersRemoved)
	assert.Equal(t, 0, summary.ConfigDriftsFixed)
	assert.Equal(t, 0, summary.CRDMutationsFixed)
}

func TestRunClean_FullIntegration(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()
	ns := "integration-ns"
	chaosLabels := map[string]string{
		safety.ManagedByLabel: safety.ManagedByValue,
	}

	// 1. NetworkPolicy with chaos label (for cleanNetworkPolicies)
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-np",
			Namespace: ns,
			Labels:    chaosLabels,
		},
	}

	// 2. Lease with chaos label (for cleanLeases)
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-lease",
			Namespace: ns,
			Labels:    chaosLabels,
		},
	}

	// 3. ClusterRole with chaos label (for cleanClusterRoles)
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "chaos-clusterrole",
			Labels: chaosLabels,
		},
	}

	// 4. RoleBinding with chaos label (for cleanRoleBindings)
	rbChaos := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-rolebinding",
			Namespace: ns,
			Labels:    chaosLabels,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "some-role",
		},
	}

	// 5. TTL-expired NetworkPolicy (for cleanTTLExpired)
	expiredTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	ttlNP := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ttl-expired-np",
			Namespace: ns,
			Annotations: map[string]string{
				safety.TTLAnnotationKey: expiredTime,
			},
		},
	}

	// 6. Webhook with rollback (for cleanWebhookConfigurations)
	failPolicy := admissionregv1.Fail
	webhookRollback := mustMarshal(t, map[string]string{
		"test.webhook.io": "Ignore",
	})
	webhook := &admissionregv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "chaos-webhook",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: webhookRollback,
			},
			Labels: chaosLabelsFor(v1alpha1.WebhookDisrupt),
		},
		Webhooks: []admissionregv1.ValidatingWebhook{
			{
				Name:                    "test.webhook.io",
				FailurePolicy:           &failPolicy,
				ClientConfig:            admissionregv1.WebhookClientConfig{URL: strPtr("https://example.com")},
				SideEffects:             sideEffectPtr(admissionregv1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1"},
			},
		},
	}

	// 7. ClusterRoleBinding with RBAC rollback (for cleanRBACBindings)
	rbacSubjects := []rbacv1.Subject{
		{Kind: "ServiceAccount", Name: "original-sa", Namespace: ns},
	}
	rbacRollback := mustMarshal(t, rbacSubjects)
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "chaos-crb",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rbacRollback,
			},
			Labels: chaosLabelsFor(v1alpha1.RBACRevoke),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "some-role",
		},
		Subjects: []rbacv1.Subject{},
	}

	// 8. ConfigMap with finalizer rollback (for cleanOrphanedFinalizers)
	finalizerRollback := mustMarshal(t, map[string]string{
		"finalizer": "chaos.opendatahub.io/block",
	})
	cmFinalizer := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "finalizer-cm",
			Namespace: ns,
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: finalizerRollback,
			},
			Labels:     chaosLabelsFor(v1alpha1.FinalizerBlock),
			Finalizers: []string{"chaos.opendatahub.io/block"},
		},
	}

	// 9. ConfigMap with drift rollback (for cleanConfigDrift)
	driftRollback := mustMarshal(t, map[string]string{
		"resourceType":  "ConfigMap",
		"key":           "app.conf",
		"originalValue": "original-data",
	})
	cmDrift := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "drift-cm",
			Namespace: ns,
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: driftRollback,
			},
			Labels: chaosLabelsFor(v1alpha1.ConfigDrift),
		},
		Data: map[string]string{
			"app.conf": "corrupted",
		},
	}

	// 10. Secret with CRD mutation rollback (for cleanCRDMutations)
	crdMutationRollback := mustMarshal(t, map[string]interface{}{
		"apiVersion":    "apps/v1",
		"kind":          "Deployment",
		"field":         "replicas",
		"originalValue": 3,
	})
	crdMutationSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "crd-mutation-secret",
			Namespace: ns,
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: crdMutationRollback,
			},
			Labels: chaosLabelsFor(v1alpha1.CRDMutation),
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(np, lease, clusterRole, rbChaos, ttlNP, webhook, crb, cmFinalizer, cmDrift, crdMutationSecret).
		Build()

	summary := runClean(ctx, k8sClient, ns)

	assert.Equal(t, 1, summary.NetworkPolicies, "should clean 1 NetworkPolicy")
	assert.Equal(t, 1, summary.Leases, "should clean 1 Lease")
	assert.Equal(t, 1, summary.ClusterRoles, "should clean 1 ClusterRole")
	assert.Equal(t, 1, summary.RoleBindings, "should clean 1 RoleBinding")
	assert.Equal(t, 1, summary.TTLExpired, "should clean 1 TTL-expired policy")
	assert.Equal(t, 1, summary.WebhooksRestored, "should restore 1 webhook")
	assert.Equal(t, 1, summary.RBACBindingsFixed, "should fix 1 RBAC binding")
	assert.Equal(t, 1, summary.FinalizersRemoved, "should remove 1 finalizer")
	assert.Equal(t, 1, summary.ConfigDriftsFixed, "should fix 1 config drift")
	assert.Equal(t, 1, summary.CRDMutationsFixed, "should fix 1 CRD mutation")
	assert.Equal(t, 10, summary.total(), "total should be 10")
}

func TestRunClean_EmptyCluster(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	summary := runClean(ctx, k8sClient, "default")
	assert.Equal(t, 0, summary.total(), "empty cluster should return zero summary")
}

func TestCleanSummary_TotalWithMixedCounts(t *testing.T) {
	s := cleanSummary{
		NetworkPolicies:   2,
		Leases:            1,
		ClusterRoles:      3,
		RoleBindings:      0,
		TTLExpired:        1,
		WebhooksRestored:  2,
		RBACBindingsFixed: 1,
		FinalizersRemoved: 4,
		ConfigDriftsFixed: 2,
		CRDMutationsFixed: 1,
	}
	assert.Equal(t, 17, s.total(), "total should sum all fields")
}

func TestCleanSummary_TotalWithAllZeros(t *testing.T) {
	s := cleanSummary{}
	assert.Equal(t, 0, s.total(), "total of zero summary should be 0")
}

// ---------------------------------------------------------------------------
// Additional edge-case tests
// ---------------------------------------------------------------------------

func TestCleanNetworkPolicies_DeletesChaosLabeledPolicies(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	chaosLabels := client.MatchingLabels{safety.ManagedByLabel: safety.ManagedByValue}

	chaosNP := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-np",
			Namespace: "default",
			Labels:    map[string]string{safety.ManagedByLabel: safety.ManagedByValue},
		},
	}

	normalNP := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "normal-np",
			Namespace: "default",
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(chaosNP, normalNP).Build()

	cleaned := cleanNetworkPolicies(ctx, k8sClient, "default", chaosLabels)
	assert.Equal(t, 1, cleaned, "should delete only chaos-labeled policy")

	// Verify normal policy still exists
	policies := &networkingv1.NetworkPolicyList{}
	require.NoError(t, k8sClient.List(ctx, policies, client.InNamespace("default")))
	assert.Len(t, policies.Items, 1, "only normal policy should remain")
	assert.Equal(t, "normal-np", policies.Items[0].Name)
}

func TestCleanLeases_DeletesChaosLeases(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	chaosLabels := client.MatchingLabels{safety.ManagedByLabel: safety.ManagedByValue}

	chaosLease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-lease",
			Namespace: "default",
			Labels:    map[string]string{safety.ManagedByLabel: safety.ManagedByValue},
		},
	}

	normalLease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "normal-lease",
			Namespace: "default",
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(chaosLease, normalLease).Build()

	cleaned := cleanLeases(ctx, k8sClient, "default", chaosLabels)
	assert.Equal(t, 1, cleaned, "should delete only chaos lease")
}

func TestCleanClusterRoles_DeletesChaosClusterRoles(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	chaosLabels := client.MatchingLabels{safety.ManagedByLabel: safety.ManagedByValue}

	chaosRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "chaos-role",
			Labels: map[string]string{safety.ManagedByLabel: safety.ManagedByValue},
		},
	}

	normalRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "normal-role",
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(chaosRole, normalRole).Build()

	cleaned := cleanClusterRoles(ctx, k8sClient, chaosLabels)
	assert.Equal(t, 1, cleaned, "should delete only chaos cluster role")
}

func TestCleanFinalizerFromResource_WithRollbackAnnotationButNoFinalizerKey(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	// Rollback data with annotation present but missing "finalizer" key
	rollbackData := mustMarshal(t, map[string]string{
		"something": "else",
	})

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "weird-annotation",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	result := cleanFinalizerFromResource(ctx, k8sClient, cm, cm.Name, cm.Namespace)
	assert.False(t, result, "should return false when rollback data has no 'finalizer' key")
}

func TestCleanFinalizerFromResource_WithMalformedJSON(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bad-json",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: "{{not-valid-json",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	result := cleanFinalizerFromResource(ctx, k8sClient, cm, cm.Name, cm.Namespace)
	assert.False(t, result, "should return false for malformed JSON in rollback annotation")
}

func TestCleanConfigDrift_SecretWithNilData(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	rollbackData := mustMarshal(t, map[string]string{
		"resourceType":  "Secret",
		"key":           "token",
		"originalValue": "original-token",
	})

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nil-data-secret",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels: chaosLabelsFor(v1alpha1.ConfigDrift),
		},
		// Data is nil
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	restored := cleanConfigDrift(ctx, k8sClient, "default")
	assert.Equal(t, 1, restored, "should restore even when Secret.Data is nil")

	updated := &corev1.Secret{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "nil-data-secret", Namespace: "default"}, updated))
	assert.Equal(t, []byte("original-token"), updated.Data["token"])
}

func TestCleanConfigDrift_ConfigMapWithWrongResourceType(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	// ConfigMap with a rollback annotation that says resourceType=Secret (mismatch)
	rollbackData := mustMarshal(t, map[string]string{
		"resourceType":  "Secret",
		"key":           "key",
		"originalValue": "value",
	})

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mismatched-cm",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	restored := cleanConfigDrift(ctx, k8sClient, "default")
	assert.Equal(t, 0, restored, "should skip ConfigMap with resourceType=Secret rollback")
}

func TestCleanWebhookConfigurations_EmptyFailurePolicyRestored(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	failPolicy := admissionregv1.Fail
	// Rollback data with empty string for policy = nil restoration
	rollbackData := mustMarshal(t, map[string]string{
		"test.webhook.io": "",
	})

	webhook := &admissionregv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nil-policy-webhook",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels: chaosLabelsFor(v1alpha1.WebhookDisrupt),
		},
		Webhooks: []admissionregv1.ValidatingWebhook{
			{
				Name:                    "test.webhook.io",
				FailurePolicy:           &failPolicy,
				ClientConfig:            admissionregv1.WebhookClientConfig{URL: strPtr("https://example.com")},
				SideEffects:             sideEffectPtr(admissionregv1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1"},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(webhook).Build()

	restored := cleanWebhookConfigurations(ctx, k8sClient)
	assert.Equal(t, 1, restored)

	updated := &admissionregv1.ValidatingWebhookConfiguration{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "nil-policy-webhook"}, updated))
	assert.Nil(t, updated.Webhooks[0].FailurePolicy,
		"failure policy should be nil when rollback value is empty string")
}

// ---------------------------------------------------------------------------
// Task 4: cleanOrphanedFinalizers expanded resource types
// ---------------------------------------------------------------------------

func TestCleanOrphanedFinalizers_StatefulSets(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	rollbackData := mustMarshal(t, map[string]string{
		"finalizer": "chaos.opendatahub.io/block",
	})
	labels := chaosLabelsFor(v1alpha1.FinalizerBlock)

	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-statefulset",
			Namespace: "test-ns",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels:     labels,
			Finalizers: []string{"chaos.opendatahub.io/block"},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ss).Build()

	cleaned := cleanOrphanedFinalizers(ctx, k8sClient, "test-ns")
	assert.Equal(t, 1, cleaned, "should clean StatefulSet finalizer")

	updated := &appsv1.StatefulSet{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "test-statefulset", Namespace: "test-ns"}, updated))
	assert.NotContains(t, updated.Finalizers, "chaos.opendatahub.io/block")
	_, hasRollback := updated.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasRollback, "rollback annotation should be removed")
	_, hasManagedBy := updated.Labels[safety.ManagedByLabel]
	assert.False(t, hasManagedBy, "managed-by label should be removed")
}

func TestCleanOrphanedFinalizers_DaemonSets(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	rollbackData := mustMarshal(t, map[string]string{
		"finalizer": "chaos.opendatahub.io/block",
	})
	labels := chaosLabelsFor(v1alpha1.FinalizerBlock)

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-daemonset",
			Namespace: "test-ns",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels:     labels,
			Finalizers: []string{"chaos.opendatahub.io/block"},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ds).Build()

	cleaned := cleanOrphanedFinalizers(ctx, k8sClient, "test-ns")
	assert.Equal(t, 1, cleaned, "should clean DaemonSet finalizer")

	updated := &appsv1.DaemonSet{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "test-daemonset", Namespace: "test-ns"}, updated))
	assert.NotContains(t, updated.Finalizers, "chaos.opendatahub.io/block")
	_, hasRollback := updated.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasRollback, "rollback annotation should be removed")
}

func TestCleanOrphanedFinalizers_Jobs(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	rollbackData := mustMarshal(t, map[string]string{
		"finalizer": "chaos.opendatahub.io/block",
	})
	labels := chaosLabelsFor(v1alpha1.FinalizerBlock)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "test-ns",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels:     labels,
			Finalizers: []string{"chaos.opendatahub.io/block"},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()

	cleaned := cleanOrphanedFinalizers(ctx, k8sClient, "test-ns")
	assert.Equal(t, 1, cleaned, "should clean Job finalizer")

	updated := &batchv1.Job{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "test-job", Namespace: "test-ns"}, updated))
	assert.NotContains(t, updated.Finalizers, "chaos.opendatahub.io/block")
	_, hasRollback := updated.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasRollback, "rollback annotation should be removed")
}

func TestCleanOrphanedFinalizers_AllSevenResourceTypes(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	rollbackData := mustMarshal(t, map[string]string{
		"finalizer": "chaos.opendatahub.io/block",
	})
	labels := chaosLabelsFor(v1alpha1.FinalizerBlock)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-deploy", Namespace: "test-ns",
			Annotations: map[string]string{safety.RollbackAnnotationKey: rollbackData},
			Labels: labels, Finalizers: []string{"chaos.opendatahub.io/block"},
		},
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cm", Namespace: "test-ns",
			Annotations: map[string]string{safety.RollbackAnnotationKey: rollbackData},
			Labels: labels, Finalizers: []string{"chaos.opendatahub.io/block"},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-secret", Namespace: "test-ns",
			Annotations: map[string]string{safety.RollbackAnnotationKey: rollbackData},
			Labels: labels, Finalizers: []string{"chaos.opendatahub.io/block"},
		},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-svc", Namespace: "test-ns",
			Annotations: map[string]string{safety.RollbackAnnotationKey: rollbackData},
			Labels: labels, Finalizers: []string{"chaos.opendatahub.io/block"},
		},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}},
	}
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ss", Namespace: "test-ns",
			Annotations: map[string]string{safety.RollbackAnnotationKey: rollbackData},
			Labels: labels, Finalizers: []string{"chaos.opendatahub.io/block"},
		},
	}
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ds", Namespace: "test-ns",
			Annotations: map[string]string{safety.RollbackAnnotationKey: rollbackData},
			Labels: labels, Finalizers: []string{"chaos.opendatahub.io/block"},
		},
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-job", Namespace: "test-ns",
			Annotations: map[string]string{safety.RollbackAnnotationKey: rollbackData},
			Labels: labels, Finalizers: []string{"chaos.opendatahub.io/block"},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dep, cm, secret, svc, ss, ds, job).
		Build()

	cleaned := cleanOrphanedFinalizers(ctx, k8sClient, "test-ns")
	assert.Equal(t, 7, cleaned, "should clean all 7 resource types")
}

// ---------------------------------------------------------------------------
// Task 5: cleanCRDMutations and cleanCRDMutationFromResource
// ---------------------------------------------------------------------------

func TestCleanCRDMutationFromResource_WithCRDMutationRollback(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	rollbackData := mustMarshal(t, map[string]interface{}{
		"apiVersion":    "apps/v1",
		"kind":          "Deployment",
		"field":         "replicas",
		"originalValue": 3,
	})

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mutated-deploy",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels: chaosLabelsFor(v1alpha1.CRDMutation),
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep).Build()

	result := cleanCRDMutationFromResource(ctx, k8sClient, dep, dep.Name, dep.Namespace)
	assert.True(t, result, "should return true when CRD mutation rollback is found")

	updated := &appsv1.Deployment{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "mutated-deploy", Namespace: "default"}, updated))

	_, hasRollback := updated.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasRollback, "rollback annotation should be removed")
	_, hasManagedBy := updated.Labels[safety.ManagedByLabel]
	assert.False(t, hasManagedBy, "managed-by label should be removed")
	_, hasChaosType := updated.Labels[safety.ChaosTypeLabel]
	assert.False(t, hasChaosType, "chaos-type label should be removed")
}

func TestCleanCRDMutationFromResource_WithFinalizerRollback(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	// Finalizer rollback does NOT have "apiVersion" and "kind" keys
	rollbackData := mustMarshal(t, map[string]string{
		"finalizer": "chaos.opendatahub.io/block",
	})

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "finalizer-cm",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels: chaosLabelsFor(v1alpha1.FinalizerBlock),
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	result := cleanCRDMutationFromResource(ctx, k8sClient, cm, cm.Name, cm.Namespace)
	assert.False(t, result, "should return false for non-CRDMutation rollback data")
}

func TestCleanCRDMutationFromResource_WithConfigDriftRollback(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	// ConfigDrift rollback does NOT have "apiVersion" and "kind" keys
	rollbackData := mustMarshal(t, map[string]string{
		"resourceType":  "ConfigMap",
		"key":           "foo",
		"originalValue": "bar",
	})

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "drift-cm",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels: chaosLabelsFor(v1alpha1.ConfigDrift),
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	result := cleanCRDMutationFromResource(ctx, k8sClient, cm, cm.Name, cm.Namespace)
	assert.False(t, result, "should return false for ConfigDrift rollback data")
}

func TestCleanCRDMutationFromResource_WithNoAnnotations(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "clean-cm",
			Namespace: "default",
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	result := cleanCRDMutationFromResource(ctx, k8sClient, cm, cm.Name, cm.Namespace)
	assert.False(t, result, "should return false when no annotations present")
}

func TestCleanCRDMutationFromResource_WithMalformedJSON(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bad-json-cm",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: "{{not-valid-json",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	result := cleanCRDMutationFromResource(ctx, k8sClient, cm, cm.Name, cm.Namespace)
	assert.False(t, result, "should return false for malformed JSON")
}

func TestCleanCRDMutationFromResource_WithOnlyAPIVersionKey(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	// Has apiVersion but not kind — should not match CRDMutation signature
	rollbackData := mustMarshal(t, map[string]interface{}{
		"apiVersion": "v1",
		"field":      "something",
	})

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "partial-cm",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	result := cleanCRDMutationFromResource(ctx, k8sClient, cm, cm.Name, cm.Namespace)
	assert.False(t, result, "should return false when only apiVersion is present (missing kind)")
}

func TestCleanCRDMutations_MultipleResourceTypes(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	rollbackData := mustMarshal(t, map[string]interface{}{
		"apiVersion":    "apps/v1",
		"kind":          "Deployment",
		"field":         "replicas",
		"originalValue": 3,
	})
	labels := chaosLabelsFor(v1alpha1.CRDMutation)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mutated-deploy", Namespace: "test-ns",
			Annotations: map[string]string{safety.RollbackAnnotationKey: rollbackData},
			Labels:      labels,
		},
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mutated-cm", Namespace: "test-ns",
			Annotations: map[string]string{safety.RollbackAnnotationKey: rollbackData},
			Labels:      labels,
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mutated-secret", Namespace: "test-ns",
			Annotations: map[string]string{safety.RollbackAnnotationKey: rollbackData},
			Labels:      labels,
		},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mutated-svc", Namespace: "test-ns",
			Annotations: map[string]string{safety.RollbackAnnotationKey: rollbackData},
			Labels:      labels,
		},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}},
	}
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mutated-ss", Namespace: "test-ns",
			Annotations: map[string]string{safety.RollbackAnnotationKey: rollbackData},
			Labels:      labels,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dep, cm, secret, svc, ss).
		Build()

	cleaned := cleanCRDMutations(ctx, k8sClient, "test-ns")
	assert.Equal(t, 5, cleaned, "should clean all 5 resource types with CRD mutation rollback")

	// Verify each resource was cleaned
	updatedDep := &appsv1.Deployment{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "mutated-deploy", Namespace: "test-ns"}, updatedDep))
	_, hasRollback := updatedDep.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasRollback)

	updatedCM := &corev1.ConfigMap{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "mutated-cm", Namespace: "test-ns"}, updatedCM))
	_, hasRollback = updatedCM.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasRollback)

	updatedSS := &appsv1.StatefulSet{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "mutated-ss", Namespace: "test-ns"}, updatedSS))
	_, hasRollback = updatedSS.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasRollback)
}

func TestCleanCRDMutations_SkipsNonCRDMutationRollback(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	// Finalizer rollback on a Deployment — should NOT be cleaned by CRD mutation scan
	finalizerRollback := mustMarshal(t, map[string]string{
		"finalizer": "chaos.opendatahub.io/block",
	})

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "finalizer-deploy",
			Namespace: "test-ns",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: finalizerRollback,
			},
			Labels: chaosLabelsFor(v1alpha1.FinalizerBlock),
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep).Build()

	cleaned := cleanCRDMutations(ctx, k8sClient, "test-ns")
	assert.Equal(t, 0, cleaned, "should skip non-CRDMutation rollback annotations")

	// Verify the annotation was NOT removed
	updated := &appsv1.Deployment{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "finalizer-deploy", Namespace: "test-ns"}, updated))
	_, hasRollback := updated.Annotations[safety.RollbackAnnotationKey]
	assert.True(t, hasRollback, "rollback annotation should still be present")
}

func TestCleanCRDMutations_EmptyNamespace(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	cleaned := cleanCRDMutations(ctx, k8sClient, "empty-ns")
	assert.Equal(t, 0, cleaned, "should return 0 for empty namespace")
}

// ---------------------------------------------------------------------------
// Task 9: Integrity checksum backward compatibility
// ---------------------------------------------------------------------------

func TestCleanConfigDrift_WithEnvelopeFormat(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	// Use WrapRollbackData to produce the new envelope format
	rollbackStr, err := safety.WrapRollbackData(map[string]string{
		"resourceType":  "ConfigMap",
		"key":           "app.conf",
		"originalValue": "original-data",
	})
	require.NoError(t, err)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "envelope-cm",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackStr,
			},
			Labels: chaosLabelsFor(v1alpha1.ConfigDrift),
		},
		Data: map[string]string{
			"app.conf": "corrupted-data",
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	restored := cleanConfigDrift(ctx, k8sClient, "default")
	assert.Equal(t, 1, restored, "should restore ConfigMap from envelope format")

	updated := &corev1.ConfigMap{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "envelope-cm", Namespace: "default"}, updated))
	assert.Equal(t, "original-data", updated.Data["app.conf"])
}

func TestCleanFinalizerFromResource_WithEnvelopeFormat(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	rollbackStr, err := safety.WrapRollbackData(map[string]string{
		"finalizer": "chaos.opendatahub.io/block",
	})
	require.NoError(t, err)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "envelope-finalizer-cm",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackStr,
			},
			Labels:     chaosLabelsFor(v1alpha1.FinalizerBlock),
			Finalizers: []string{"chaos.opendatahub.io/block"},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	result := cleanFinalizerFromResource(ctx, k8sClient, cm, cm.Name, cm.Namespace)
	assert.True(t, result, "should handle envelope format for finalizer rollback")

	updated := &corev1.ConfigMap{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "envelope-finalizer-cm", Namespace: "default"}, updated))
	assert.NotContains(t, updated.Finalizers, "chaos.opendatahub.io/block")
}

func TestCleanCRDMutationFromResource_WithEnvelopeFormat(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	rollbackStr, err := safety.WrapRollbackData(map[string]interface{}{
		"apiVersion":    "apps/v1",
		"kind":          "Deployment",
		"field":         "replicas",
		"originalValue": 3,
	})
	require.NoError(t, err)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "envelope-deploy",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackStr,
			},
			Labels: chaosLabelsFor(v1alpha1.CRDMutation),
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep).Build()
	result := cleanCRDMutationFromResource(ctx, k8sClient, dep, dep.Name, dep.Namespace)
	assert.True(t, result, "should handle envelope format for CRD mutation rollback")
}

// ---------------------------------------------------------------------------
// Task 10: Secret rollback via dedicated Secret
// ---------------------------------------------------------------------------

func TestCleanConfigDrift_SecretWithRollbackSecretRef(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	// Create the rollback Secret that holds the original value
	rollbackSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-rollback-drifted-secret-password",
			Namespace: "default",
			Labels:    chaosLabelsFor(v1alpha1.ConfigDrift),
		},
		Data: map[string][]byte{
			"password": []byte("original-secret"),
		},
	}

	// Create rollback annotation using WrapRollbackData with rollbackSecretRef
	rollbackStr, err := safety.WrapRollbackData(map[string]string{
		"resourceType":      "Secret",
		"key":               "password",
		"rollbackSecretRef": "chaos-rollback-drifted-secret-password",
	})
	require.NoError(t, err)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "drifted-secret",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackStr,
			},
			Labels: chaosLabelsFor(v1alpha1.ConfigDrift),
		},
		Data: map[string][]byte{
			"password": []byte("corrupted-secret"),
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(secret, rollbackSecret).Build()

	restored := cleanConfigDrift(ctx, k8sClient, "default")
	assert.Equal(t, 1, restored, "should restore 1 Secret")

	// Verify the Secret value was restored
	updated := &corev1.Secret{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "drifted-secret", Namespace: "default"}, updated))
	assert.Equal(t, []byte("original-secret"), updated.Data["password"],
		"data should be restored from rollback Secret")

	_, hasRollback := updated.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasRollback, "rollback annotation should be removed")

	// Verify the rollback Secret was deleted
	deletedSecret := &corev1.Secret{}
	err = k8sClient.Get(ctx, client.ObjectKey{Name: "chaos-rollback-drifted-secret-password", Namespace: "default"}, deletedSecret)
	assert.Error(t, err, "rollback Secret should be deleted after cleanup")
}

func TestCleanConfigDrift_SecretWithLegacyOriginalValue(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	// Legacy format: originalValue is stored directly (no rollbackSecretRef)
	rollbackData := mustMarshal(t, map[string]string{
		"resourceType":  "Secret",
		"key":           "token",
		"originalValue": "original-token",
	})

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-secret",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackData,
			},
			Labels: chaosLabelsFor(v1alpha1.ConfigDrift),
		},
		Data: map[string][]byte{
			"token": []byte("corrupted-token"),
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	restored := cleanConfigDrift(ctx, k8sClient, "default")
	assert.Equal(t, 1, restored, "should restore Secret using legacy originalValue")

	updated := &corev1.Secret{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: "legacy-secret", Namespace: "default"}, updated))
	assert.Equal(t, []byte("original-token"), updated.Data["token"],
		"should restore using legacy originalValue path")
}

func TestCleanConfigDrift_SecretRollbackSecretMissing(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	// rollbackSecretRef points to a non-existent Secret
	rollbackStr, err := safety.WrapRollbackData(map[string]string{
		"resourceType":      "Secret",
		"key":               "password",
		"rollbackSecretRef": "chaos-rollback-nonexistent",
	})
	require.NoError(t, err)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "broken-secret",
			Namespace: "default",
			Annotations: map[string]string{
				safety.RollbackAnnotationKey: rollbackStr,
			},
			Labels: chaosLabelsFor(v1alpha1.ConfigDrift),
		},
		Data: map[string][]byte{
			"password": []byte("corrupted"),
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	restored := cleanConfigDrift(ctx, k8sClient, "default")
	assert.Equal(t, 0, restored, "should fail gracefully when rollback Secret is missing")
}

// ---------------------------------------------------------------------------
// helpers (package-level to avoid redeclaration with other test files)
// ---------------------------------------------------------------------------

func strPtr(s string) *string { return &s }

func sideEffectPtr(se admissionregv1.SideEffectClass) *admissionregv1.SideEffectClass { return &se }

// ---------------------------------------------------------------------------
// Watch mode tests
// ---------------------------------------------------------------------------

func TestNewCleanCommand_WatchFlagDefaults(t *testing.T) {
	cmd := newCleanCommand()

	watchFlag := cmd.Flags().Lookup("watch")
	require.NotNil(t, watchFlag, "--watch flag should be registered")
	assert.Equal(t, "false", watchFlag.DefValue, "--watch should default to false")

	intervalFlag := cmd.Flags().Lookup("interval")
	require.NotNil(t, intervalFlag, "--interval flag should be registered")
	assert.Equal(t, "1m0s", intervalFlag.DefValue, "--interval should default to 60s")
}

func TestRunCleanWatch_ExitsOnContextCancel(t *testing.T) {
	scheme := newTestScheme()
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately so the watch loop exits after the first scan.
	cancel()

	err := runCleanWatch(ctx, k8sClient, "default", 100*time.Millisecond)
	assert.NoError(t, err, "runCleanWatch should return nil on context cancellation")
}

func TestRunCleanWatch_RunsMultipleScans(t *testing.T) {
	scheme := newTestScheme()

	// Seed a chaos NetworkPolicy so the first scan finds something.
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-np",
			Namespace: "default",
			Labels:    chaosLabelsFor(v1alpha1.NetworkPartition),
		},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(np).
		Build()

	ctx, cancel := context.WithCancel(context.Background())

	// Use a very short interval so we get at least 2 scans quickly.
	done := make(chan error, 1)
	go func() {
		done <- runCleanWatch(ctx, k8sClient, "default", 50*time.Millisecond)
	}()

	// Wait enough time for at least 2 scans, then cancel.
	time.Sleep(200 * time.Millisecond)
	cancel()

	err := <-done
	assert.NoError(t, err, "runCleanWatch should return nil after cancellation")

	// Verify the NetworkPolicy was cleaned up by the first scan.
	var remaining networkingv1.NetworkPolicyList
	require.NoError(t, k8sClient.List(context.Background(), &remaining, client.InNamespace("default")))
	assert.Empty(t, remaining.Items, "chaos NetworkPolicy should have been cleaned")
}

func TestCleanSummaryDiff(t *testing.T) {
	// Just verify it doesn't panic with various inputs.
	cleanSummaryDiff(cleanSummary{}, cleanSummary{NetworkPolicies: 2})
	cleanSummaryDiff(cleanSummary{NetworkPolicies: 3}, cleanSummary{})
	cleanSummaryDiff(cleanSummary{Leases: 1}, cleanSummary{Leases: 1})
}

func TestRunCleanWatch_RejectsNonPositiveInterval(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(newTestScheme()).Build()

	err := runCleanWatch(context.Background(), k8sClient, "default", 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--interval must be positive")

	err = runCleanWatch(context.Background(), k8sClient, "default", -1*time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--interval must be positive")
}
