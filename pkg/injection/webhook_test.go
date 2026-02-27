package injection

import (
	"context"
	"encoding/json"
	"testing"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestWebhookDisruptValidate(t *testing.T) {
	injector := NewWebhookDisruptInjector(nil)
	blast := v1alpha1.BlastRadiusSpec{MaxPodsAffected: 1, AllowedNamespaces: []string{"test"}}

	tests := []struct {
		name    string
		spec    v1alpha1.InjectionSpec
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid spec with setFailurePolicy action",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.WebhookDisrupt,
				Parameters: map[string]string{
					"webhookName": "my-webhook",
					"action":      "setFailurePolicy",
					"value":       "Fail",
				},
			},
			wantErr: false,
		},
		{
			name: "valid spec without value defaults to Fail",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.WebhookDisrupt,
				Parameters: map[string]string{
					"webhookName": "my-webhook",
					"action":      "setFailurePolicy",
				},
			},
			wantErr: false,
		},
		{
			name: "missing webhookName",
			spec: v1alpha1.InjectionSpec{
				Type:       v1alpha1.WebhookDisrupt,
				Parameters: map[string]string{"action": "setFailurePolicy"},
			},
			wantErr: true,
			errMsg:  "webhookName",
		},
		{
			name: "missing action",
			spec: v1alpha1.InjectionSpec{
				Type:       v1alpha1.WebhookDisrupt,
				Parameters: map[string]string{"webhookName": "my-webhook"},
			},
			wantErr: true,
			errMsg:  "action",
		},
		{
			name: "nil parameters",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.WebhookDisrupt,
			},
			wantErr: true,
			errMsg:  "webhookName",
		},
		{
			name: "unsupported action",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.WebhookDisrupt,
				Parameters: map[string]string{
					"webhookName": "my-webhook",
					"action":      "deleteWebhook",
				},
			},
			wantErr: true,
			errMsg:  "unsupported action",
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

func TestWebhookDisruptInjectAndCleanup(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, admissionv1.AddToScheme(scheme))

	failPolicy := admissionv1.Ignore
	webhook := &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "my-webhook"},
		Webhooks: []admissionv1.ValidatingWebhook{
			{
				Name:                    "test.webhook.io",
				FailurePolicy:           &failPolicy,
				ClientConfig:            admissionv1.WebhookClientConfig{URL: strPtr("https://example.com")},
				SideEffects:             sideEffectPtr(admissionv1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(webhook).Build()
	injector := NewWebhookDisruptInjector(fakeClient)

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.WebhookDisrupt,
		Parameters: map[string]string{
			"webhookName": "my-webhook",
			"action":      "setFailurePolicy",
			"value":       "Fail",
		},
	}

	ctx := context.Background()

	// Inject
	cleanup, events, err := injector.Inject(ctx, spec, "default")
	require.NoError(t, err)
	assert.NotEmpty(t, events)
	assert.NotNil(t, cleanup)
	assert.Equal(t, "setFailurePolicy", events[0].Action)

	// Verify the webhook was modified
	modified := &admissionv1.ValidatingWebhookConfiguration{}
	require.NoError(t, fakeClient.Get(ctx,
		client.ObjectKey{Name: "my-webhook"}, modified))
	require.NotNil(t, modified.Webhooks[0].FailurePolicy)
	assert.Equal(t, admissionv1.Fail, *modified.Webhooks[0].FailurePolicy)

	// Cleanup should restore
	require.NoError(t, cleanup(ctx))
	restored := &admissionv1.ValidatingWebhookConfiguration{}
	require.NoError(t, fakeClient.Get(ctx,
		client.ObjectKey{Name: "my-webhook"}, restored))
	require.NotNil(t, restored.Webhooks[0].FailurePolicy)
	assert.Equal(t, admissionv1.Ignore, *restored.Webhooks[0].FailurePolicy)
}

func TestWebhookDisruptInjectMultipleWebhooks(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, admissionv1.AddToScheme(scheme))

	ignorePolicy := admissionv1.Ignore
	webhook := &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "multi-webhook"},
		Webhooks: []admissionv1.ValidatingWebhook{
			{
				Name:                    "first.webhook.io",
				FailurePolicy:           &ignorePolicy,
				ClientConfig:            admissionv1.WebhookClientConfig{URL: strPtr("https://example.com/first")},
				SideEffects:             sideEffectPtr(admissionv1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1"},
			},
			{
				Name:                    "second.webhook.io",
				FailurePolicy:           &ignorePolicy,
				ClientConfig:            admissionv1.WebhookClientConfig{URL: strPtr("https://example.com/second")},
				SideEffects:             sideEffectPtr(admissionv1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(webhook).Build()
	injector := NewWebhookDisruptInjector(fakeClient)

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.WebhookDisrupt,
		Parameters: map[string]string{
			"webhookName": "multi-webhook",
			"action":      "setFailurePolicy",
			"value":       "Fail",
		},
	}

	ctx := context.Background()

	// Inject - all webhooks in the configuration should be modified
	cleanup, events, err := injector.Inject(ctx, spec, "default")
	require.NoError(t, err)
	assert.NotEmpty(t, events)

	modified := &admissionv1.ValidatingWebhookConfiguration{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{Name: "multi-webhook"}, modified))
	for i, wh := range modified.Webhooks {
		require.NotNil(t, wh.FailurePolicy, "webhook %d should have failure policy set", i)
		assert.Equal(t, admissionv1.Fail, *wh.FailurePolicy, "webhook %d should be Fail", i)
	}

	// Cleanup should restore all webhooks
	require.NoError(t, cleanup(ctx))
	restored := &admissionv1.ValidatingWebhookConfiguration{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{Name: "multi-webhook"}, restored))
	for i, wh := range restored.Webhooks {
		require.NotNil(t, wh.FailurePolicy, "webhook %d should have failure policy set", i)
		assert.Equal(t, admissionv1.Ignore, *wh.FailurePolicy, "webhook %d should be restored to Ignore", i)
	}
}

func TestWebhookDisruptInjectStoresRollbackAnnotation(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, admissionv1.AddToScheme(scheme))

	ignorePolicy := admissionv1.Ignore
	webhook := &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "annotated-webhook"},
		Webhooks: []admissionv1.ValidatingWebhook{
			{
				Name:                    "test.webhook.io",
				FailurePolicy:           &ignorePolicy,
				ClientConfig:            admissionv1.WebhookClientConfig{URL: strPtr("https://example.com")},
				SideEffects:             sideEffectPtr(admissionv1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(webhook).Build()
	injector := NewWebhookDisruptInjector(fakeClient)

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.WebhookDisrupt,
		Parameters: map[string]string{
			"webhookName": "annotated-webhook",
			"action":      "setFailurePolicy",
			"value":       "Fail",
		},
	}

	ctx := context.Background()

	// Inject
	cleanup, _, err := injector.Inject(ctx, spec, "default")
	require.NoError(t, err)

	// Verify the rollback annotation exists with original policy
	modified := &admissionv1.ValidatingWebhookConfiguration{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{Name: "annotated-webhook"}, modified))

	rollbackJSON, ok := modified.Annotations[safety.RollbackAnnotationKey]
	require.True(t, ok, "rollback annotation should be present after injection")

	var rollbackData map[string]string
	require.NoError(t, json.Unmarshal([]byte(rollbackJSON), &rollbackData))
	assert.Equal(t, "Ignore", rollbackData["test.webhook.io"], "rollback data should contain original Ignore policy")

	// Verify chaos labels are present
	assert.Equal(t, safety.ManagedByValue, modified.Labels[safety.ManagedByLabel])
	assert.Equal(t, string(v1alpha1.WebhookDisrupt), modified.Labels[safety.ChaosTypeLabel])

	// Cleanup should remove annotation and labels
	require.NoError(t, cleanup(ctx))
	restored := &admissionv1.ValidatingWebhookConfiguration{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{Name: "annotated-webhook"}, restored))

	_, hasAnnotation := restored.Annotations[safety.RollbackAnnotationKey]
	assert.False(t, hasAnnotation, "rollback annotation should be removed after cleanup")

	_, hasManagedBy := restored.Labels[safety.ManagedByLabel]
	assert.False(t, hasManagedBy, "managed-by label should be removed after cleanup")

	_, hasChaosType := restored.Labels[safety.ChaosTypeLabel]
	assert.False(t, hasChaosType, "chaos-type label should be removed after cleanup")

	// Verify policy was restored
	require.NotNil(t, restored.Webhooks[0].FailurePolicy)
	assert.Equal(t, admissionv1.Ignore, *restored.Webhooks[0].FailurePolicy)
}

func TestWebhookDisruptNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, admissionv1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	injector := NewWebhookDisruptInjector(fakeClient)

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.WebhookDisrupt,
		Parameters: map[string]string{
			"webhookName": "nonexistent-webhook",
			"action":      "setFailurePolicy",
			"value":       "Fail",
		},
	}

	_, _, err := injector.Inject(context.Background(), spec, "default")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent-webhook")
}

func strPtr(s string) *string { return &s }

func sideEffectPtr(se admissionv1.SideEffectClass) *admissionv1.SideEffectClass { return &se }
