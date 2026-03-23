package injection

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/sdk"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClientFaultInjector manages the odh-chaos-config ConfigMap lifecycle
// to inject in-process faults via the SDK ChaosClient.
type ClientFaultInjector struct {
	client client.Client
}

// NewClientFaultInjector creates a new ClientFaultInjector.
func NewClientFaultInjector(c client.Client) *ClientFaultInjector {
	return &ClientFaultInjector{client: c}
}

// Validate checks that the ClientFault parameters are well-formed.
func (s *ClientFaultInjector) Validate(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error {
	return validateClientFaultParams(spec)
}

// Inject creates or updates the chaos ConfigMap with fault configuration
// and returns a cleanup function that restores the original state.
func (s *ClientFaultInjector) Inject(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) (CleanupFunc, []v1alpha1.InjectionEvent, error) {
	cmName := spec.Parameters["configMapName"]
	if cmName == "" {
		cmName = sdk.ChaosConfigMapName
	}

	key := types.NamespacedName{Name: cmName, Namespace: namespace}

	// Build the FaultConfig from parameters
	faultConfig, err := buildFaultConfig(spec.Parameters["faults"])
	if err != nil {
		return nil, nil, fmt.Errorf("building fault config: %w", err)
	}

	configJSON, err := json.Marshal(faultConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling fault config: %w", err)
	}

	// Check if ConfigMap already exists
	existing := &corev1.ConfigMap{}
	var originalData map[string]string
	existed := false

	if err := s.client.Get(ctx, key, existing); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, nil, fmt.Errorf("getting ConfigMap %s: %w", key, err)
		}
	} else {
		existed = true
		originalData = make(map[string]string, len(existing.Data))
		for k, v := range existing.Data {
			originalData[k] = v
		}
	}

	if existed {
		// Update existing ConfigMap
		if existing.Data == nil {
			existing.Data = make(map[string]string)
		}
		existing.Data[sdk.ChaosConfigKey] = string(configJSON)
		rollbackStr, err := safety.WrapRollbackData(originalData)
		if err != nil {
			return nil, nil, fmt.Errorf("serializing rollback data: %w", err)
		}
		safety.ApplyChaosMetadata(existing, rollbackStr, string(v1alpha1.ClientFault))
		if err := s.client.Update(ctx, existing); err != nil {
			return nil, nil, fmt.Errorf("updating ConfigMap %s: %w", key, err)
		}
	} else {
		// Create new ConfigMap
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cmName,
				Namespace: namespace,
			},
			Data: map[string]string{
				sdk.ChaosConfigKey: string(configJSON),
			},
		}
		createdByChaosMarker, err := safety.WrapRollbackData(map[string]string{"chaos.opendatahub.io/created": "true"})
		if err != nil {
			return nil, nil, fmt.Errorf("serializing chaos marker: %w", err)
		}
		safety.ApplyChaosMetadata(cm, createdByChaosMarker, string(v1alpha1.ClientFault))
		if err := s.client.Create(ctx, cm); err != nil {
			return nil, nil, fmt.Errorf("creating ConfigMap %s: %w", key, err)
		}
	}

	// Build cleanup function
	cleanup := func(ctx context.Context) error {
		if !existed {
			// Delete the ConfigMap we created
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmName,
					Namespace: namespace,
				},
			}
			if err := s.client.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("deleting ConfigMap %s: %w", key, err)
			}
			return nil
		}

		// Restore original ConfigMap data
		cm := &corev1.ConfigMap{}
		if err := s.client.Get(ctx, key, cm); err != nil {
			if apierrors.IsNotFound(err) {
				return nil // CM was deleted externally, nothing to restore
			}
			return fmt.Errorf("getting ConfigMap %s for restore: %w", key, err)
		}
		cm.Data = originalData
		safety.RemoveChaosMetadata(cm, string(v1alpha1.ClientFault))
		if err := s.client.Update(ctx, cm); err != nil {
			return fmt.Errorf("restoring ConfigMap %s: %w", key, err)
		}
		return nil
	}

	events := []v1alpha1.InjectionEvent{
		NewEvent(v1alpha1.ClientFault, key.String(), "activated",
			map[string]string{"configMap": cmName, "namespace": namespace}),
	}

	return cleanup, events, nil
}

// Revert restores the original ConfigMap state or deletes the ConfigMap if it was
// created by the injection. It is idempotent: if no rollback annotation is present
// or the ConfigMap is gone, it returns nil.
func (s *ClientFaultInjector) Revert(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) error {
	cmName := spec.Parameters["configMapName"]
	if cmName == "" {
		cmName = sdk.ChaosConfigMapName
	}

	key := types.NamespacedName{Name: cmName, Namespace: namespace}

	cm := &corev1.ConfigMap{}
	if err := s.client.Get(ctx, key, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // already gone
		}
		return fmt.Errorf("getting ConfigMap %s for revert: %w", key, err)
	}

	// Check for rollback annotation
	rollbackStr, ok := cm.GetAnnotations()[safety.RollbackAnnotationKey]
	if !ok {
		return nil // no chaos metadata, already reverted
	}

	if rollbackStr == "" {
		// No rollback data — nothing to do (idempotent).
		return nil
	}

	// Try to unwrap rollback data to determine if CM was created by chaos or pre-existed
	var rollbackMap map[string]string
	if err := safety.UnwrapRollbackData(rollbackStr, &rollbackMap); err != nil {
		return fmt.Errorf("unwrapping rollback data for ConfigMap %s: %w", key, err)
	}

	// If the marker indicates the CM was created by chaos, delete it
	if rollbackMap["chaos.opendatahub.io/created"] == "true" {
		if err := s.client.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("deleting ConfigMap %s during revert: %w", key, err)
		}
		return nil
	}

	// Non-marker rollback data means the ConfigMap existed before — restore original data
	cm.Data = rollbackMap
	safety.RemoveChaosMetadata(cm, string(v1alpha1.ClientFault))

	return s.client.Update(ctx, cm)
}

// clientFaultSpec mirrors sdk.FaultSpec for JSON marshaling without time.Duration issues.
// This type must remain JSON-compatible with sdk.FaultConfig. If sdk.FaultSpec changes, update this type.
type clientFaultSpec struct {
	ErrorRate float64 `json:"errorRate"`
	Error     string  `json:"error"`
	Delay     string  `json:"delay,omitempty"`
	MaxDelay  string  `json:"maxDelay,omitempty"`
}

// clientFaultConfig is the JSON-serializable fault configuration.
type clientFaultConfig struct {
	Active bool                       `json:"active"`
	Faults map[string]clientFaultSpec `json:"faults"`
}

// buildFaultConfig parses the faults JSON parameter and builds a
// FaultConfig ready for ConfigMap serialization.
func buildFaultConfig(faultsJSON string) (*clientFaultConfig, error) {
	var faults map[string]clientFaultSpec
	if err := json.Unmarshal([]byte(faultsJSON), &faults); err != nil {
		return nil, err
	}

	for op, spec := range faults {
		if spec.Delay != "" {
			if _, err := time.ParseDuration(spec.Delay); err != nil {
				return nil, fmt.Errorf("invalid delay for operation %q: %w", op, err)
			}
		}
		if spec.MaxDelay != "" {
			if _, err := time.ParseDuration(spec.MaxDelay); err != nil {
				return nil, fmt.Errorf("invalid maxDelay for operation %q: %w", op, err)
			}
		}
	}

	return &clientFaultConfig{
		Active: true,
		Faults: faults,
	}, nil
}
