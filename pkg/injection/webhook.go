package injection

import (
	"context"
	"fmt"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WebhookDisruptInjector disrupts Kubernetes admission webhooks by modifying
// their configuration (e.g., changing FailurePolicy from Ignore to Fail).
// Currently supports ValidatingWebhookConfiguration.
type WebhookDisruptInjector struct {
	client client.Client
}

// NewWebhookDisruptInjector creates a new WebhookDisruptInjector.
func NewWebhookDisruptInjector(c client.Client) *WebhookDisruptInjector {
	return &WebhookDisruptInjector{client: c}
}

func (w *WebhookDisruptInjector) Validate(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error {
	return validateWebhookDisruptParams(spec)
}

// Inject performs the webhook disruption:
// 1. Fetches the ValidatingWebhookConfiguration by name
// 2. Saves the original failure policies for all webhooks in the configuration
// 3. Sets the failure policy on all webhooks to the specified value
// 4. Returns a cleanup function that restores the original failure policies
func (w *WebhookDisruptInjector) Inject(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) (CleanupFunc, []v1alpha1.InjectionEvent, error) {
	webhookName := spec.Parameters["webhookName"]

	// Fetch the ValidatingWebhookConfiguration
	webhookConfig := &admissionv1.ValidatingWebhookConfiguration{}
	if err := w.client.Get(ctx, client.ObjectKey{Name: webhookName}, webhookConfig); err != nil {
		return nil, nil, fmt.Errorf("getting ValidatingWebhookConfiguration %q: %w", webhookName, err)
	}

	// Determine target failure policy
	targetPolicyStr := spec.Parameters["value"]
	if targetPolicyStr == "" {
		targetPolicyStr = "Fail"
	}
	targetPolicy, err := parseFailurePolicy(targetPolicyStr)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing failure policy value: %w", err)
	}

	// Save original failure policies for each webhook as a map of name -> policy value
	originalPolicies := make(map[string]string, len(webhookConfig.Webhooks))
	for _, wh := range webhookConfig.Webhooks {
		if wh.FailurePolicy != nil {
			originalPolicies[wh.Name] = string(*wh.FailurePolicy)
		} else {
			originalPolicies[wh.Name] = ""
		}
	}

	// Serialize original policies with integrity checksum for crash-safe rollback
	rollbackStr, err := safety.WrapRollbackData(originalPolicies)
	if err != nil {
		return nil, nil, fmt.Errorf("serializing original policies for ValidatingWebhookConfiguration %q: %w", webhookName, err)
	}

	// Store rollback annotation and chaos labels
	safety.ApplyChaosMetadata(webhookConfig, rollbackStr, string(v1alpha1.WebhookDisrupt))

	// Modify all webhooks to use the target failure policy
	for i := range webhookConfig.Webhooks {
		p := targetPolicy
		webhookConfig.Webhooks[i].FailurePolicy = &p
	}

	// Update the webhook configuration
	if err := w.client.Update(ctx, webhookConfig); err != nil {
		return nil, nil, fmt.Errorf("updating ValidatingWebhookConfiguration %q: %w", webhookName, err)
	}

	events := []v1alpha1.InjectionEvent{
		NewEvent(v1alpha1.WebhookDisrupt, webhookName, "setFailurePolicy",
			map[string]string{
				"webhookName":   webhookName,
				"failurePolicy": targetPolicyStr,
				"webhookCount":  fmt.Sprintf("%d", len(webhookConfig.Webhooks)),
			}),
	}

	// Cleanup uses the Revert method to restore from annotation data,
	// avoiding stale closure variables if the webhook is externally modified.
	cleanup := func(ctx context.Context) error {
		return w.Revert(ctx, spec, namespace)
	}

	return cleanup, events, nil
}

// Revert restores the original failure policies on the ValidatingWebhookConfiguration.
// It is idempotent: if no rollback annotation is present, it returns nil.
func (w *WebhookDisruptInjector) Revert(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) error {
	webhookName := spec.Parameters["webhookName"]

	webhookConfig := &admissionv1.ValidatingWebhookConfiguration{}
	if err := w.client.Get(ctx, client.ObjectKey{Name: webhookName}, webhookConfig); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("getting ValidatingWebhookConfiguration %q for revert: %w", webhookName, err)
	}

	// Check for rollback annotation — if absent, already reverted
	rollbackStr, ok := webhookConfig.GetAnnotations()[safety.RollbackAnnotationKey]
	if !ok {
		return nil
	}

	var originalPolicies map[string]string
	if err := safety.UnwrapRollbackData(rollbackStr, &originalPolicies); err != nil {
		return fmt.Errorf("unwrapping rollback data for ValidatingWebhookConfiguration %q: %w", webhookName, err)
	}

	// Restore original failure policies
	for i, wh := range webhookConfig.Webhooks {
		if policyStr, ok := originalPolicies[wh.Name]; ok {
			if policyStr == "" {
				webhookConfig.Webhooks[i].FailurePolicy = nil
			} else {
				p := admissionv1.FailurePolicyType(policyStr)
				webhookConfig.Webhooks[i].FailurePolicy = &p
			}
		}
	}

	safety.RemoveChaosMetadata(webhookConfig, string(v1alpha1.WebhookDisrupt))

	return w.client.Update(ctx, webhookConfig)
}

// parseFailurePolicy converts a string to an admissionv1.FailurePolicyType.
func parseFailurePolicy(s string) (admissionv1.FailurePolicyType, error) {
	switch admissionv1.FailurePolicyType(s) {
	case admissionv1.Fail:
		return admissionv1.Fail, nil
	case admissionv1.Ignore:
		return admissionv1.Ignore, nil
	default:
		return "", fmt.Errorf("invalid failure policy %q; must be 'Fail' or 'Ignore'", s)
	}
}
