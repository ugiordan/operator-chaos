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

// ConfigDriftInjector injects faults by modifying data in ConfigMaps or Secrets.
type ConfigDriftInjector struct {
	client client.Client
}

// NewConfigDriftInjector creates a new ConfigDriftInjector using the given Kubernetes client.
func NewConfigDriftInjector(c client.Client) *ConfigDriftInjector {
	return &ConfigDriftInjector{client: c}
}

func (d *ConfigDriftInjector) Validate(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error {
	return validateConfigDriftParams(spec)
}

// Inject overwrites a key in the target ConfigMap or Secret and returns a cleanup function that restores the original value.
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
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
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
			return nil, nil, fmt.Errorf("updating Secret %s/%s: %w", key.Namespace, key.Name, err)
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
			if s.Data == nil {
				s.Data = make(map[string][]byte)
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
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
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
		return nil, nil, fmt.Errorf("updating ConfigMap %s/%s: %w", key.Namespace, key.Name, err)
	}

	cleanup := func(ctx context.Context) error {
		c := &corev1.ConfigMap{}
		if err := d.client.Get(ctx, key, c); err != nil {
			return err
		}
		if c.Data == nil {
			c.Data = make(map[string]string)
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
