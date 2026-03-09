package safety

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestUnwrapRollbackData_LegacyFormat(t *testing.T) {
	// Legacy format is just raw JSON without envelope
	legacy, err := json.Marshal(map[string]string{
		"resourceType":  "ConfigMap",
		"key":           "app.conf",
		"originalValue": "original-data",
	})
	require.NoError(t, err)

	var result map[string]string
	require.NoError(t, UnwrapRollbackData(string(legacy), &result))
	assert.Equal(t, "ConfigMap", result["resourceType"])
	assert.Equal(t, "app.conf", result["key"])
	assert.Equal(t, "original-data", result["originalValue"])
}

func TestUnwrapRollbackData_LegacyArrayFormat(t *testing.T) {
	// RBAC rollback stores an array of subjects directly
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
	require.NoError(t, UnwrapRollbackData(string(legacy), &result))
	assert.Len(t, result, 2)
	assert.Equal(t, "my-sa", result[0].Name)
	assert.Equal(t, "admin", result[1].Name)
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

func TestUnwrapRollbackData_LegacyMapWithDataKey(t *testing.T) {
	// Edge case: legacy format that happens to have a "data" key but no "checksum"
	// Should be treated as legacy format
	legacy := `{"data":"some-value","key":"other"}`

	var result map[string]string
	require.NoError(t, UnwrapRollbackData(legacy, &result))
	assert.Equal(t, "some-value", result["data"])
	assert.Equal(t, "other", result["key"])
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
