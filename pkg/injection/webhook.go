package injection

import (
	"context"
	"fmt"

	v1alpha1 "github.com/opendatahub-io/operator-chaos/api/v1alpha1"
	"github.com/opendatahub-io/operator-chaos/pkg/safety"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WebhookDisruptInjector disrupts Kubernetes admission webhooks by modifying
// their configuration (e.g., changing FailurePolicy from Ignore to Fail).
// Supports both ValidatingWebhookConfiguration and MutatingWebhookConfiguration
// via the "webhookType" parameter ("validating" or "mutating", defaults to "validating").
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
// 1. Fetches the webhook configuration (Validating or Mutating based on webhookType param)
// 2. Saves the original failure policies for all webhooks in the configuration
// 3. Sets the failure policy on all webhooks to the specified value
// 4. Returns a cleanup function that restores the original failure policies
func (w *WebhookDisruptInjector) Inject(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) (CleanupFunc, []v1alpha1.InjectionEvent, error) {
	webhookName := spec.Parameters["webhookName"]
	webhookType := resolveWebhookType(spec.Parameters["webhookType"])

	// Determine target failure policy
	targetPolicyStr := spec.Parameters["value"]
	if targetPolicyStr == "" {
		targetPolicyStr = "Fail"
	}
	targetPolicy, err := parseFailurePolicy(targetPolicyStr)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing failure policy value: %w", err)
	}

	var originalPolicies map[string]string
	var webhookCount int

	if webhookType == "mutating" {
		originalPolicies, webhookCount, err = w.injectMutating(ctx, webhookName, targetPolicy)
	} else {
		originalPolicies, webhookCount, err = w.injectValidating(ctx, webhookName, targetPolicy)
	}
	if err != nil {
		return nil, nil, err
	}

	_ = originalPolicies // rollback data is stored as annotations on the resource

	events := []v1alpha1.InjectionEvent{
		NewEvent(v1alpha1.WebhookDisrupt, webhookName, "setFailurePolicy",
			map[string]string{
				"webhookName":   webhookName,
				"webhookType":   webhookType,
				"failurePolicy": targetPolicyStr,
				"webhookCount":  fmt.Sprintf("%d", webhookCount),
			}),
	}

	cleanup := func(ctx context.Context) error {
		return w.Revert(ctx, spec, namespace)
	}

	return cleanup, events, nil
}

func (w *WebhookDisruptInjector) injectValidating(ctx context.Context, webhookName string, targetPolicy admissionv1.FailurePolicyType) (map[string]string, int, error) {
	webhookConfig := &admissionv1.ValidatingWebhookConfiguration{}
	if err := w.client.Get(ctx, client.ObjectKey{Name: webhookName}, webhookConfig); err != nil {
		return nil, 0, fmt.Errorf("getting ValidatingWebhookConfiguration %q: %w", webhookName, err)
	}

	originalPolicies := make(map[string]string, len(webhookConfig.Webhooks))
	for _, wh := range webhookConfig.Webhooks {
		if wh.FailurePolicy != nil {
			originalPolicies[wh.Name] = string(*wh.FailurePolicy)
		} else {
			originalPolicies[wh.Name] = ""
		}
	}

	rollbackStr, err := safety.WrapRollbackData(originalPolicies)
	if err != nil {
		return nil, 0, fmt.Errorf("serializing original policies for ValidatingWebhookConfiguration %q: %w", webhookName, err)
	}

	safety.ApplyChaosMetadata(webhookConfig, rollbackStr, string(v1alpha1.WebhookDisrupt))

	for i := range webhookConfig.Webhooks {
		p := targetPolicy
		webhookConfig.Webhooks[i].FailurePolicy = &p
	}

	if err := w.client.Update(ctx, webhookConfig); err != nil {
		return nil, 0, fmt.Errorf("updating ValidatingWebhookConfiguration %q: %w", webhookName, err)
	}

	return originalPolicies, len(webhookConfig.Webhooks), nil
}

func (w *WebhookDisruptInjector) injectMutating(ctx context.Context, webhookName string, targetPolicy admissionv1.FailurePolicyType) (map[string]string, int, error) {
	webhookConfig := &admissionv1.MutatingWebhookConfiguration{}
	if err := w.client.Get(ctx, client.ObjectKey{Name: webhookName}, webhookConfig); err != nil {
		return nil, 0, fmt.Errorf("getting MutatingWebhookConfiguration %q: %w", webhookName, err)
	}

	originalPolicies := make(map[string]string, len(webhookConfig.Webhooks))
	for _, wh := range webhookConfig.Webhooks {
		if wh.FailurePolicy != nil {
			originalPolicies[wh.Name] = string(*wh.FailurePolicy)
		} else {
			originalPolicies[wh.Name] = ""
		}
	}

	rollbackStr, err := safety.WrapRollbackData(originalPolicies)
	if err != nil {
		return nil, 0, fmt.Errorf("serializing original policies for MutatingWebhookConfiguration %q: %w", webhookName, err)
	}

	safety.ApplyChaosMetadata(webhookConfig, rollbackStr, string(v1alpha1.WebhookDisrupt))

	for i := range webhookConfig.Webhooks {
		p := targetPolicy
		webhookConfig.Webhooks[i].FailurePolicy = &p
	}

	if err := w.client.Update(ctx, webhookConfig); err != nil {
		return nil, 0, fmt.Errorf("updating MutatingWebhookConfiguration %q: %w", webhookName, err)
	}

	return originalPolicies, len(webhookConfig.Webhooks), nil
}

// Revert restores the original failure policies on the webhook configuration.
// It is idempotent: if no rollback annotation is present, it returns nil.
func (w *WebhookDisruptInjector) Revert(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) error {
	webhookName := spec.Parameters["webhookName"]
	webhookType := resolveWebhookType(spec.Parameters["webhookType"])

	if webhookType == "mutating" {
		return w.revertMutating(ctx, webhookName)
	}
	return w.revertValidating(ctx, webhookName)
}

func (w *WebhookDisruptInjector) revertValidating(ctx context.Context, webhookName string) error {
	webhookConfig := &admissionv1.ValidatingWebhookConfiguration{}
	if err := w.client.Get(ctx, client.ObjectKey{Name: webhookName}, webhookConfig); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("getting ValidatingWebhookConfiguration %q for revert: %w", webhookName, err)
	}

	rollbackStr, ok := webhookConfig.GetAnnotations()[safety.RollbackAnnotationKey]
	if !ok {
		return nil
	}

	var originalPolicies map[string]string
	if err := safety.UnwrapRollbackData(rollbackStr, &originalPolicies); err != nil {
		return fmt.Errorf("unwrapping rollback data for ValidatingWebhookConfiguration %q: %w", webhookName, err)
	}

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

func (w *WebhookDisruptInjector) revertMutating(ctx context.Context, webhookName string) error {
	webhookConfig := &admissionv1.MutatingWebhookConfiguration{}
	if err := w.client.Get(ctx, client.ObjectKey{Name: webhookName}, webhookConfig); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("getting MutatingWebhookConfiguration %q for revert: %w", webhookName, err)
	}

	rollbackStr, ok := webhookConfig.GetAnnotations()[safety.RollbackAnnotationKey]
	if !ok {
		return nil
	}

	var originalPolicies map[string]string
	if err := safety.UnwrapRollbackData(rollbackStr, &originalPolicies); err != nil {
		return fmt.Errorf("unwrapping rollback data for MutatingWebhookConfiguration %q: %w", webhookName, err)
	}

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

// resolveWebhookType returns the webhook type, defaulting to "validating".
func resolveWebhookType(t string) string {
	if t == "mutating" {
		return "mutating"
	}
	return "validating"
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
