package safety

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// RollbackAnnotationKey is the annotation key used to store original resource
	// state for rollback during chaos cleanup.
	RollbackAnnotationKey = "chaos.opendatahub.io/rollback-data"

	// ManagedByLabel is the standard Kubernetes label for tracking resource ownership.
	ManagedByLabel = "app.kubernetes.io/managed-by"

	// ManagedByValue is the value used in managed-by labels for chaos resources.
	ManagedByValue = "odh-chaos"

	// ChaosTypeLabel is the label key used to identify the type of chaos injection
	// that created a resource.
	ChaosTypeLabel = "chaos.opendatahub.io/type"
)

// ChaosLabels returns standard labels for chaos-managed resources.
func ChaosLabels(injectionType string) map[string]string {
	return map[string]string{
		ManagedByLabel: ManagedByValue,
		ChaosTypeLabel: injectionType,
	}
}

// rollbackEnvelope wraps rollback data with a SHA-256 integrity checksum.
type rollbackEnvelope struct {
	Data     json.RawMessage `json:"data"`
	Checksum string          `json:"checksum"`
}

// WrapRollbackData serializes data with an integrity checksum.
// The output format is: {"data": {...actual rollback data...}, "checksum": "<sha256 hex>"}
func WrapRollbackData(data any) (string, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	envelope := rollbackEnvelope{
		Data:     raw,
		Checksum: hex.EncodeToString(hash[:]),
	}
	out, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// UnwrapRollbackData deserializes and verifies checksum integrity.
// Supports legacy format (no envelope) for backward compatibility.
func UnwrapRollbackData(raw string, target any) error {
	var envelope rollbackEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		// Cannot parse as envelope at all — treat as legacy format
		return json.Unmarshal([]byte(raw), target)
	}
	if envelope.Data == nil || envelope.Checksum == "" {
		// Legacy format: valid JSON but no envelope structure
		return json.Unmarshal([]byte(raw), target)
	}
	hash := sha256.Sum256(envelope.Data)
	expected := hex.EncodeToString(hash[:])
	if envelope.Checksum != expected {
		return fmt.Errorf("rollback data checksum mismatch: expected %s, got %s", expected, envelope.Checksum)
	}
	return json.Unmarshal(envelope.Data, target)
}

// ApplyChaosMetadata sets the rollback annotation and chaos labels on a resource.
func ApplyChaosMetadata(obj client.Object, rollbackData string, injectionType string) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[RollbackAnnotationKey] = rollbackData
	obj.SetAnnotations(annotations)

	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	for k, v := range ChaosLabels(injectionType) {
		labels[k] = v
	}
	obj.SetLabels(labels)
}

// RemoveChaosMetadata removes the rollback annotation and chaos labels from a resource.
func RemoveChaosMetadata(obj client.Object, injectionType string) {
	annotations := obj.GetAnnotations()
	delete(annotations, RollbackAnnotationKey)
	obj.SetAnnotations(annotations)

	labels := obj.GetLabels()
	for k := range ChaosLabels(injectionType) {
		delete(labels, k)
	}
	obj.SetLabels(labels)
}
