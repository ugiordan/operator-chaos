package injection

import (
	"context"
	"fmt"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ConfigDriftInjector struct {
	client client.Client
}

func NewConfigDriftInjector(c client.Client) *ConfigDriftInjector {
	return &ConfigDriftInjector{client: c}
}

func (d *ConfigDriftInjector) Validate(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error {
	if _, ok := spec.Parameters["name"]; !ok {
		return fmt.Errorf("ConfigDrift requires 'name' parameter")
	}
	if err := validateK8sName("name", spec.Parameters["name"]); err != nil {
		return err
	}
	if _, ok := spec.Parameters["key"]; !ok {
		return fmt.Errorf("ConfigDrift requires 'key' parameter (data key to modify)")
	}
	if _, ok := spec.Parameters["value"]; !ok {
		return fmt.Errorf("ConfigDrift requires 'value' parameter (corrupted value)")
	}
	// Validate resourceType if specified
	resourceType := spec.Parameters["resourceType"]
	if resourceType != "" && resourceType != "ConfigMap" && resourceType != "Secret" {
		return fmt.Errorf("ConfigDrift resourceType must be 'ConfigMap' or 'Secret', got %q", resourceType)
	}
	// For Secrets, validate that the rollback Secret name won't exceed K8s limit
	if resourceType == "Secret" {
		rollbackName := "chaos-rollback-" + spec.Parameters["name"] + "-" + spec.Parameters["key"]
		if len(rollbackName) > maxNameLength {
			return fmt.Errorf("rollback Secret name %q exceeds %d character limit", rollbackName, maxNameLength)
		}
	}
	return nil
}

func (d *ConfigDriftInjector) Inject(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) (CleanupFunc, []v1alpha1.InjectionEvent, error) {
	key := types.NamespacedName{
		Name:      spec.Parameters["name"],
		Namespace: namespace,
	}

	resourceType := spec.Parameters["resourceType"]
	if resourceType == "" {
		resourceType = "ConfigMap"
	}

	var originalValue string

	if resourceType == "Secret" {
		secret := &corev1.Secret{}
		if err := d.client.Get(ctx, key, secret); err != nil {
			return nil, nil, fmt.Errorf("getting Secret %s: %w", key, err)
		}
		dataKey := spec.Parameters["key"]
		originalValue = string(secret.Data[dataKey])
		secret.Data[dataKey] = []byte(spec.Parameters["value"])

		// Create a dedicated rollback Secret to avoid storing plaintext in annotations.
		// Include the data key in the name to prevent collision when multiple keys are injected.
		rollbackSecretName := "chaos-rollback-" + key.Name + "-" + dataKey
		rollbackSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rollbackSecretName,
				Namespace: namespace,
				Labels:    safety.ChaosLabels(string(v1alpha1.ConfigDrift)),
			},
			Data: map[string][]byte{
				dataKey: []byte(originalValue),
			},
		}
		if err := d.client.Create(ctx, rollbackSecret); err != nil {
			return nil, nil, fmt.Errorf("creating rollback Secret %s: %w", rollbackSecretName, err)
		}

		// Store rollback annotation with reference to rollback Secret (no plaintext value)
		rollbackInfo := map[string]string{
			"resourceType":      "Secret",
			"key":               dataKey,
			"rollbackSecretRef": rollbackSecretName,
		}
		rollbackStr, err := safety.WrapRollbackData(rollbackInfo)
		if err != nil {
			return nil, nil, fmt.Errorf("serializing rollback data for Secret %s: %w", key, err)
		}
		safety.ApplyChaosMetadata(secret, rollbackStr, string(v1alpha1.ConfigDrift))

		if err := d.client.Update(ctx, secret); err != nil {
			return nil, nil, fmt.Errorf("updating Secret: %w", err)
		}
		cleanup := func(ctx context.Context) error {
			s := &corev1.Secret{}
			if err := d.client.Get(ctx, key, s); err != nil {
				return err
			}

			// Read original value from the rollback Secret
			rbSecret := &corev1.Secret{}
			rbKey := types.NamespacedName{Name: rollbackSecretName, Namespace: namespace}
			if err := d.client.Get(ctx, rbKey, rbSecret); err != nil {
				return fmt.Errorf("reading rollback Secret %s: %w", rollbackSecretName, err)
			}
			s.Data[dataKey] = rbSecret.Data[dataKey]

			// Remove rollback annotation and chaos labels
			safety.RemoveChaosMetadata(s, string(v1alpha1.ConfigDrift))

			if err := d.client.Update(ctx, s); err != nil {
				return err
			}

			// Delete the rollback Secret
			return d.client.Delete(ctx, rbSecret)
		}
		events := []v1alpha1.InjectionEvent{
			NewEvent(v1alpha1.ConfigDrift, key.String(), "drifted",
				map[string]string{"resourceType": "Secret", "key": dataKey}),
		}
		return cleanup, events, nil
	}

	// Default: ConfigMap
	cm := &corev1.ConfigMap{}
	if err := d.client.Get(ctx, key, cm); err != nil {
		return nil, nil, fmt.Errorf("getting ConfigMap %s: %w", key, err)
	}
	dataKey := spec.Parameters["key"]
	originalValue = cm.Data[dataKey]
	cm.Data[dataKey] = spec.Parameters["value"]

	// Store rollback annotation for crash-safe recovery
	rollbackInfo := map[string]string{
		"resourceType":  "ConfigMap",
		"key":           dataKey,
		"originalValue": originalValue,
	}
	rollbackStr, err := safety.WrapRollbackData(rollbackInfo)
	if err != nil {
		return nil, nil, fmt.Errorf("serializing rollback data for ConfigMap %s: %w", key, err)
	}
	safety.ApplyChaosMetadata(cm, rollbackStr, string(v1alpha1.ConfigDrift))

	if err := d.client.Update(ctx, cm); err != nil {
		return nil, nil, fmt.Errorf("updating ConfigMap: %w", err)
	}

	cleanup := func(ctx context.Context) error {
		c := &corev1.ConfigMap{}
		if err := d.client.Get(ctx, key, c); err != nil {
			return err
		}
		c.Data[dataKey] = originalValue

		// Remove rollback annotation and chaos labels
		safety.RemoveChaosMetadata(c, string(v1alpha1.ConfigDrift))

		return d.client.Update(ctx, c)
	}

	events := []v1alpha1.InjectionEvent{
		NewEvent(v1alpha1.ConfigDrift, key.String(), "drifted",
			map[string]string{"resourceType": "ConfigMap", "key": dataKey}),
	}

	return cleanup, events, nil
}
