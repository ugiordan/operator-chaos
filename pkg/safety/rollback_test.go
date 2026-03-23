package safety

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func TestRollbackAnnotationKey(t *testing.T) {
	assert.Equal(t, "chaos.opendatahub.io/rollback-data", RollbackAnnotationKey)
}

func TestManagedByLabel(t *testing.T) {
	assert.Equal(t, "odh-chaos", ManagedByValue)
}

func TestChaosLabels(t *testing.T) {
	labels := ChaosLabels("PodKill")
	assert.Equal(t, "odh-chaos", labels[ManagedByLabel])
	assert.Equal(t, "PodKill", labels[ChaosTypeLabel])
}

func TestChaosLabelsContainsExpectedKeys(t *testing.T) {
	labels := ChaosLabels("NetworkPartition")
	assert.Len(t, labels, 2)
	assert.Contains(t, labels, ManagedByLabel)
	assert.Contains(t, labels, ChaosTypeLabel)
	assert.Equal(t, "NetworkPartition", labels[ChaosTypeLabel])
}

func TestChaosLabelsDifferentTypes(t *testing.T) {
	types := []string{"PodKill", "NetworkPartition", "WebhookDisrupt", "RBACRevoke", "FinalizerBlock"}
	for _, injType := range types {
		labels := ChaosLabels(injType)
		assert.Equal(t, ManagedByValue, labels[ManagedByLabel])
		assert.Equal(t, injType, labels[ChaosTypeLabel])
	}
}

// ---------------------------------------------------------------------------
// WrapRollbackData / UnwrapRollbackData tests
// ---------------------------------------------------------------------------

func TestWrapRollbackData_ProducesValidEnvelope(t *testing.T) {
	input := map[string]string{
		"resourceType":  "ConfigMap",
		"key":           "app.conf",
		"originalValue": "original-data",
	}

	wrapped, err := WrapRollbackData(input)
	require.NoError(t, err)

	// Parse as envelope to verify structure
	var envelope rollbackEnvelope
	require.NoError(t, json.Unmarshal([]byte(wrapped), &envelope))

	assert.NotEmpty(t, envelope.Checksum, "checksum should be present")
	assert.NotEmpty(t, envelope.Data, "data should be present")

	// Verify the checksum is correct
	hash := sha256.Sum256(envelope.Data)
	expected := hex.EncodeToString(hash[:])
	assert.Equal(t, expected, envelope.Checksum, "checksum should match sha256 of data")

	// Verify the inner data is correct
	var innerData map[string]string
	require.NoError(t, json.Unmarshal(envelope.Data, &innerData))
	assert.Equal(t, "ConfigMap", innerData["resourceType"])
	assert.Equal(t, "app.conf", innerData["key"])
	assert.Equal(t, "original-data", innerData["originalValue"])
}

func TestUnwrapRollbackData_NewEnvelopeFormat(t *testing.T) {
	input := map[string]string{
		"finalizer": "chaos.opendatahub.io/block",
	}

	wrapped, err := WrapRollbackData(input)
	require.NoError(t, err)

	var result map[string]string
	require.NoError(t, UnwrapRollbackData(wrapped, &result))
	assert.Equal(t, "chaos.opendatahub.io/block", result["finalizer"])
}

func TestUnwrapRollbackData_LegacyFormat_Rejected(t *testing.T) {
	// Legacy format (no integrity envelope) is no longer supported
	legacy, err := json.Marshal(map[string]string{
		"resourceType":  "ConfigMap",
		"key":           "app.conf",
		"originalValue": "original-data",
	})
	require.NoError(t, err)

	var result map[string]string
	err = UnwrapRollbackData(string(legacy), &result)
	require.Error(t, err, "legacy format should be rejected")
	assert.Contains(t, err.Error(), "legacy format is no longer supported")
}

func TestUnwrapRollbackData_LegacyArrayFormat_Rejected(t *testing.T) {
	// RBAC rollback stores an array of subjects directly — legacy format rejected
	type Subject struct {
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Namespace string `json:"namespace,omitempty"`
	}

	original := []Subject{
		{Kind: "ServiceAccount", Name: "my-sa", Namespace: "default"},
		{Kind: "User", Name: "admin"},
	}
	legacy, err := json.Marshal(original)
	require.NoError(t, err)

	var result []Subject
	err = UnwrapRollbackData(string(legacy), &result)
	require.Error(t, err, "legacy array format should be rejected")
	assert.Contains(t, err.Error(), "not valid JSON")
}

func TestUnwrapRollbackData_ChecksumMismatch(t *testing.T) {
	input := map[string]string{
		"key": "value",
	}

	wrapped, err := WrapRollbackData(input)
	require.NoError(t, err)

	// Tamper with the data
	var envelope rollbackEnvelope
	require.NoError(t, json.Unmarshal([]byte(wrapped), &envelope))
	envelope.Data = json.RawMessage(`{"key":"tampered"}`)
	tampered, err := json.Marshal(envelope)
	require.NoError(t, err)

	var result map[string]string
	err = UnwrapRollbackData(string(tampered), &result)
	require.Error(t, err, "should fail on checksum mismatch")
	assert.Contains(t, err.Error(), "checksum mismatch")
}

func TestUnwrapRollbackData_MalformedJSON(t *testing.T) {
	var result map[string]string
	err := UnwrapRollbackData("{{not-valid-json", &result)
	assert.Error(t, err, "should fail on malformed JSON")
}

func TestWrapUnwrapRoundTrip_MapStringString(t *testing.T) {
	input := map[string]string{
		"resourceType":      "Secret",
		"key":               "password",
		"rollbackSecretRef": "chaos-rollback-my-secret-password",
	}

	wrapped, err := WrapRollbackData(input)
	require.NoError(t, err)

	var result map[string]string
	require.NoError(t, UnwrapRollbackData(wrapped, &result))
	assert.Equal(t, input, result)
}

func TestWrapUnwrapRoundTrip_MapStringInterface(t *testing.T) {
	input := map[string]interface{}{
		"apiVersion":    "apps/v1",
		"kind":          "Deployment",
		"field":         "replicas",
		"originalValue": float64(3),
	}

	wrapped, err := WrapRollbackData(input)
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, UnwrapRollbackData(wrapped, &result))
	assert.Equal(t, "apps/v1", result["apiVersion"])
	assert.Equal(t, "Deployment", result["kind"])
	assert.Equal(t, "replicas", result["field"])
	assert.Equal(t, float64(3), result["originalValue"])
}

func TestUnwrapRollbackData_LegacyMapWithDataKey_Rejected(t *testing.T) {
	// Edge case: legacy format that happens to have a "data" key but no "checksum"
	// Should be rejected since legacy format is no longer supported
	legacy := `{"data":"some-value","key":"other"}`

	var result map[string]string
	err := UnwrapRollbackData(legacy, &result)
	require.Error(t, err, "legacy format with data key but no checksum should be rejected")
	assert.Contains(t, err.Error(), "legacy format is no longer supported")
}

func TestWrapRollbackData_NilInput(t *testing.T) {
	wrapped, err := WrapRollbackData(nil)
	require.NoError(t, err)

	// Should produce a valid envelope with null data
	var envelope rollbackEnvelope
	require.NoError(t, json.Unmarshal([]byte(wrapped), &envelope))
	assert.NotEmpty(t, envelope.Checksum)
}

func TestWrapRollbackData_EmptyMap(t *testing.T) {
	wrapped, err := WrapRollbackData(map[string]string{})
	require.NoError(t, err)

	var result map[string]string
	require.NoError(t, UnwrapRollbackData(wrapped, &result))
	assert.Empty(t, result)
}

func TestApplyChaosMetadataNilMaps(t *testing.T) {
	obj := &fakeObject{}
	ApplyChaosMetadata(obj, "rollback-data", "PodKill")

	assert.Equal(t, "rollback-data", obj.GetAnnotations()[RollbackAnnotationKey])
	assert.Equal(t, ManagedByValue, obj.GetLabels()[ManagedByLabel])
	assert.Equal(t, "PodKill", obj.GetLabels()[ChaosTypeLabel])
}

func TestApplyChaosMetadataPreservesExisting(t *testing.T) {
	obj := &fakeObject{
		annotations: map[string]string{"existing-annotation": "keep-me"},
		labels:      map[string]string{"existing-label": "keep-me"},
	}
	ApplyChaosMetadata(obj, "rollback-data", "ConfigDrift")

	assert.Equal(t, "keep-me", obj.GetAnnotations()["existing-annotation"])
	assert.Equal(t, "rollback-data", obj.GetAnnotations()[RollbackAnnotationKey])
	assert.Equal(t, "keep-me", obj.GetLabels()["existing-label"])
	assert.Equal(t, ManagedByValue, obj.GetLabels()[ManagedByLabel])
	assert.Equal(t, "ConfigDrift", obj.GetLabels()[ChaosTypeLabel])
}

func TestRemoveChaosMetadataPreservesOthers(t *testing.T) {
	obj := &fakeObject{
		annotations: map[string]string{
			"existing-annotation": "keep-me",
			RollbackAnnotationKey: "data",
		},
		labels: map[string]string{
			"existing-label": "keep-me",
			ManagedByLabel:   ManagedByValue,
			ChaosTypeLabel:   "PodKill",
		},
	}
	RemoveChaosMetadata(obj, "PodKill")

	assert.Equal(t, "keep-me", obj.GetAnnotations()["existing-annotation"])
	_, hasRollback := obj.GetAnnotations()[RollbackAnnotationKey]
	assert.False(t, hasRollback)
	assert.Equal(t, "keep-me", obj.GetLabels()["existing-label"])
	_, hasManagedBy := obj.GetLabels()[ManagedByLabel]
	assert.False(t, hasManagedBy)
	_, hasChaosType := obj.GetLabels()[ChaosTypeLabel]
	assert.False(t, hasChaosType)
}

func TestRemoveChaosMetadataNilMaps(t *testing.T) {
	obj := &fakeObject{}
	// Should be a no-op — no panic
	RemoveChaosMetadata(obj, "PodKill")

	assert.Nil(t, obj.GetAnnotations())
	assert.Nil(t, obj.GetLabels())
}

// fakeObject implements client.Object minimally for testing ApplyChaosMetadata/RemoveChaosMetadata.
type fakeObject struct {
	annotations map[string]string
	labels      map[string]string
}

func (f *fakeObject) GetAnnotations() map[string]string          { return f.annotations }
func (f *fakeObject) SetAnnotations(a map[string]string)         { f.annotations = a }
func (f *fakeObject) GetLabels() map[string]string               { return f.labels }
func (f *fakeObject) SetLabels(l map[string]string)              { f.labels = l }
func (f *fakeObject) GetNamespace() string                       { return "" }
func (f *fakeObject) SetNamespace(string)                        {}
func (f *fakeObject) GetName() string                            { return "" }
func (f *fakeObject) SetName(string)                             {}
func (f *fakeObject) GetGenerateName() string                    { return "" }
func (f *fakeObject) SetGenerateName(string)                     {}
func (f *fakeObject) GetUID() types.UID                          { return "" }
func (f *fakeObject) SetUID(types.UID)                           {}
func (f *fakeObject) GetResourceVersion() string                 { return "" }
func (f *fakeObject) SetResourceVersion(string)                  {}
func (f *fakeObject) GetGeneration() int64                       { return 0 }
func (f *fakeObject) SetGeneration(int64)                        {}
func (f *fakeObject) GetSelfLink() string                        { return "" }
func (f *fakeObject) SetSelfLink(string)                         {}
func (f *fakeObject) GetCreationTimestamp() metav1.Time          { return metav1.Time{} }
func (f *fakeObject) SetCreationTimestamp(metav1.Time)           {}
func (f *fakeObject) GetDeletionTimestamp() *metav1.Time         { return nil }
func (f *fakeObject) SetDeletionTimestamp(*metav1.Time)          {}
func (f *fakeObject) GetDeletionGracePeriodSeconds() *int64      { return nil }
func (f *fakeObject) SetDeletionGracePeriodSeconds(*int64)       {}
func (f *fakeObject) GetFinalizers() []string                    { return nil }
func (f *fakeObject) SetFinalizers([]string)                     {}
func (f *fakeObject) GetOwnerReferences() []metav1.OwnerReference { return nil }
func (f *fakeObject) SetOwnerReferences([]metav1.OwnerReference)  {}
func (f *fakeObject) GetManagedFields() []metav1.ManagedFieldsEntry { return nil }
func (f *fakeObject) SetManagedFields([]metav1.ManagedFieldsEntry)  {}
func (f *fakeObject) GetObjectKind() schema.ObjectKind            { return schema.EmptyObjectKind }
func (f *fakeObject) DeepCopyObject() runtime.Object              { return f }

func TestWrapRollbackData_OversizedRejected(t *testing.T) {
	// Create data that exceeds maxRollbackDataSize (200KB)
	largeData := map[string]string{
		"key": strings.Repeat("x", maxRollbackDataSize+1),
	}
	_, err := WrapRollbackData(largeData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rollback data too large")
}

func FuzzUnwrapRollbackData(f *testing.F) {
	// Seed: valid envelope format
	validWrapped, _ := WrapRollbackData(map[string]string{"key": "value"})
	f.Add(validWrapped)
	// Seed: legacy JSON format
	f.Add(`{"resourceType":"ConfigMap","key":"app.conf"}`)
	// Seed: empty JSON object
	f.Add("{}")
	// Seed: empty string
	f.Add("")
	// Seed: JSON array (legacy array format)
	f.Add(`[{"kind":"ServiceAccount","name":"sa"}]`)
	// Seed: malformed JSON
	f.Add("{{not-json")

	f.Fuzz(func(t *testing.T, raw string) {
		var target map[string]interface{}
		// Must not panic regardless of input
		_ = UnwrapRollbackData(raw, &target)
	})
}
