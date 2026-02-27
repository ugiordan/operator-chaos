package safety

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
