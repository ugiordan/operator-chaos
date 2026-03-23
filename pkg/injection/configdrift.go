package injection

import (
	"context"
	"fmt"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	dataKey := spec.Parameters["key"]

	if resourceType == "Secret" {
		return d.injectSecret(ctx, spec, key, dataKey, spec.Parameters["value"], namespace)
	}
	return d.injectConfigMap(ctx, spec, key, dataKey, spec.Parameters["value"])
}

func (d *ConfigDriftInjector) injectSecret(ctx context.Context, spec v1alpha1.InjectionSpec, key types.NamespacedName, dataKey, newValue, namespace string) (CleanupFunc, []v1alpha1.InjectionEvent, error) {
	secret := &corev1.Secret{}
	if err := d.client.Get(ctx, key, secret); err != nil {
		return nil, nil, fmt.Errorf("getting Secret %s: %w", key, err)
	}
	// Read original value safely before nil-check
	var originalValue string
	var keyExists bool
	if secret.Data != nil {
		if val, ok := secret.Data[dataKey]; ok {
			originalValue = string(val)
			keyExists = true
		}
	}
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data[dataKey] = []byte(newValue)

	rollbackSecretName := "chaos-rollback-" + key.Name + "-" + dataKey

	// Build labels with chaos-experiment for traceability
	rollbackLabels := safety.ChaosLabels(string(v1alpha1.ConfigDrift))
	rollbackLabels["chaos.opendatahub.io/experiment"] = key.Name

	// Build annotations with original secret reference for traceability
	rollbackAnnotations := map[string]string{
		"chaos.opendatahub.io/original-secret": key.Name,
	}

	rollbackSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        rollbackSecretName,
			Namespace:   namespace,
			Labels:      rollbackLabels,
			Annotations: rollbackAnnotations,
		},
		Data: map[string][]byte{
			dataKey:     []byte(originalValue),
			"keyExists": []byte(fmt.Sprintf("%t", keyExists)),
		},
	}
	if err := d.client.Create(ctx, rollbackSecret); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Update the existing rollback secret to ensure it has current data,
			// not stale data from a prior experiment that didn't clean up.
			existingRB := &corev1.Secret{}
			if getErr := d.client.Get(ctx, client.ObjectKeyFromObject(rollbackSecret), existingRB); getErr != nil {
				return nil, nil, fmt.Errorf("getting existing rollback Secret %s: %w", rollbackSecretName, getErr)
			}
			existingRB.Data = rollbackSecret.Data
			if updateErr := d.client.Update(ctx, existingRB); updateErr != nil {
				return nil, nil, fmt.Errorf("updating rollback Secret %s: %w", rollbackSecretName, updateErr)
			}
		} else {
			return nil, nil, fmt.Errorf("creating rollback Secret %s: %w", rollbackSecretName, err)
		}
	}

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

	// Cleanup uses the Revert method to restore from rollback secret data,
	// avoiding stale closure variables if the Secret is externally modified.
	cleanup := func(ctx context.Context) error {
		return d.revertSecret(ctx, spec, namespace)
	}

	events := []v1alpha1.InjectionEvent{
		NewEvent(v1alpha1.ConfigDrift, key.String(), "drifted",
			map[string]string{"resourceType": "Secret", "key": dataKey}),
	}
	return cleanup, events, nil
}

func (d *ConfigDriftInjector) injectConfigMap(ctx context.Context, spec v1alpha1.InjectionSpec, key types.NamespacedName, dataKey, newValue string) (CleanupFunc, []v1alpha1.InjectionEvent, error) {
	cm := &corev1.ConfigMap{}
	if err := d.client.Get(ctx, key, cm); err != nil {
		return nil, nil, fmt.Errorf("getting ConfigMap %s: %w", key, err)
	}
	var originalValue string
	var keyExists bool
	if cm.Data != nil {
		if val, ok := cm.Data[dataKey]; ok {
			originalValue = val
			keyExists = true
		}
	}
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data[dataKey] = newValue

	rollbackInfo := map[string]string{
		"resourceType":  "ConfigMap",
		"key":           dataKey,
		"originalValue": originalValue,
		"keyExists":     fmt.Sprintf("%t", keyExists),
	}
	rollbackStr, err := safety.WrapRollbackData(rollbackInfo)
	if err != nil {
		return nil, nil, fmt.Errorf("serializing rollback data for ConfigMap %s: %w", key, err)
	}
	safety.ApplyChaosMetadata(cm, rollbackStr, string(v1alpha1.ConfigDrift))

	if err := d.client.Update(ctx, cm); err != nil {
		return nil, nil, fmt.Errorf("updating ConfigMap %s/%s: %w", key.Namespace, key.Name, err)
	}

	// Cleanup uses the Revert method to restore from annotation data,
	// avoiding stale closure variables if the ConfigMap is externally modified.
	cleanup := func(ctx context.Context) error {
		return d.revertConfigMap(ctx, spec, key.Namespace)
	}

	events := []v1alpha1.InjectionEvent{
		NewEvent(v1alpha1.ConfigDrift, key.String(), "drifted",
			map[string]string{"resourceType": "ConfigMap", "key": dataKey}),
	}
	return cleanup, events, nil
}

// Revert restores the original ConfigMap or Secret data from rollback annotations.
// It is idempotent: if no rollback annotation is present, it returns nil.
func (d *ConfigDriftInjector) Revert(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) error {
	resourceType := spec.Parameters["resourceType"]
	if resourceType == "" {
		resourceType = "ConfigMap"
	}

	if resourceType == "Secret" {
		return d.revertSecret(ctx, spec, namespace)
	}
	return d.revertConfigMap(ctx, spec, namespace)
}

func (d *ConfigDriftInjector) revertSecret(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) error {
	key := types.NamespacedName{Name: spec.Parameters["name"], Namespace: namespace}
	dataKey := spec.Parameters["key"]

	secret := &corev1.Secret{}
	if err := d.client.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("getting Secret %s for revert: %w", key, err)
	}

	// Check for rollback annotation — if absent, already reverted
	rollbackStr, ok := secret.GetAnnotations()[safety.RollbackAnnotationKey]
	if !ok {
		return nil
	}

	var rollbackInfo map[string]string
	if err := safety.UnwrapRollbackData(rollbackStr, &rollbackInfo); err != nil {
		return fmt.Errorf("unwrapping rollback data for Secret %s: %w", key, err)
	}

	rollbackSecretName, ok := rollbackInfo["rollbackSecretRef"]
	if !ok || rollbackSecretName == "" {
		return fmt.Errorf("rollback data missing 'rollbackSecretRef' for Secret %s", key)
	}

	// Fetch the rollback Secret containing original value
	rbSecret := &corev1.Secret{}
	rbKey := types.NamespacedName{Name: rollbackSecretName, Namespace: namespace}
	if err := d.client.Get(ctx, rbKey, rbSecret); err != nil {
		if apierrors.IsNotFound(err) {
			// Rollback secret gone; remove metadata and return
			safety.RemoveChaosMetadata(secret, string(v1alpha1.ConfigDrift))
			return d.client.Update(ctx, secret)
		}
		return fmt.Errorf("reading rollback Secret %s: %w", rollbackSecretName, err)
	}

	// Restore original data, respecting whether the key originally existed
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	if rbSecret.Data == nil {
		return fmt.Errorf("rollback Secret %s has nil Data field", rollbackSecretName)
	}
	if string(rbSecret.Data["keyExists"]) == "false" {
		delete(secret.Data, dataKey)
	} else {
		restoredValue, hasKey := rbSecret.Data[dataKey]
		if !hasKey {
			return fmt.Errorf("rollback Secret %s missing expected key %q", rollbackSecretName, dataKey)
		}
		secret.Data[dataKey] = restoredValue
	}

	safety.RemoveChaosMetadata(secret, string(v1alpha1.ConfigDrift))

	if err := d.client.Update(ctx, secret); err != nil {
		return fmt.Errorf("restoring Secret %s: %w", key, err)
	}

	// Clean up rollback Secret
	if err := d.client.Delete(ctx, rbSecret); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("deleting rollback Secret %s: %w", rollbackSecretName, err)
	}
	return nil
}

func (d *ConfigDriftInjector) revertConfigMap(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) error {
	key := types.NamespacedName{Name: spec.Parameters["name"], Namespace: namespace}
	dataKey := spec.Parameters["key"]

	cm := &corev1.ConfigMap{}
	if err := d.client.Get(ctx, key, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("getting ConfigMap %s for revert: %w", key, err)
	}

	// Check for rollback annotation — if absent, already reverted
	rollbackStr, ok := cm.GetAnnotations()[safety.RollbackAnnotationKey]
	if !ok {
		return nil
	}

	var rollbackInfo map[string]string
	if err := safety.UnwrapRollbackData(rollbackStr, &rollbackInfo); err != nil {
		return fmt.Errorf("unwrapping rollback data for ConfigMap %s: %w", key, err)
	}

	// Restore original value, respecting whether the key originally existed
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	if rollbackInfo["keyExists"] == "false" {
		delete(cm.Data, dataKey)
	} else {
		cm.Data[dataKey] = rollbackInfo["originalValue"]
	}

	safety.RemoveChaosMetadata(cm, string(v1alpha1.ConfigDrift))

	return d.client.Update(ctx, cm)
}
