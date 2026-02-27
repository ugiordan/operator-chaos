package model

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadKnowledge(t *testing.T) {
	k, err := LoadKnowledge("../../testdata/knowledge/test-operator.yaml")
	require.NoError(t, err)

	assert.Equal(t, "test-operator", k.Operator.Name)
	assert.Equal(t, "test-ns", k.Operator.Namespace)
	assert.Len(t, k.Components, 2)
}

func TestGetComponent(t *testing.T) {
	k, err := LoadKnowledge("../../testdata/knowledge/test-operator.yaml")
	require.NoError(t, err)

	comp := k.GetComponent("dashboard")
	require.NotNil(t, comp)
	assert.Equal(t, "dashboard", comp.Name)
	assert.Len(t, comp.ManagedResources, 2)
	assert.Equal(t, "Deployment", comp.ManagedResources[0].Kind)
}

func TestGetComponentNotFound(t *testing.T) {
	k, err := LoadKnowledge("../../testdata/knowledge/test-operator.yaml")
	require.NoError(t, err)

	comp := k.GetComponent("nonexistent")
	assert.Nil(t, comp)
}

func TestKnowledgeRecoveryDefaults(t *testing.T) {
	k, err := LoadKnowledge("../../testdata/knowledge/test-operator.yaml")
	require.NoError(t, err)

	assert.Equal(t, 300*time.Second, k.Recovery.ReconcileTimeout.Duration)
	assert.Equal(t, 10, k.Recovery.MaxReconcileCycles)
}

func TestLoadKnowledgeRejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.yaml")
	data := make([]byte, 2*1024*1024) // 2MB, exceeds 1MB limit
	require.NoError(t, os.WriteFile(path, data, 0644))

	_, err := LoadKnowledge(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

func TestManagedResourceExpectedSpec(t *testing.T) {
	k, err := LoadKnowledge("../../testdata/knowledge/test-operator.yaml")
	require.NoError(t, err)

	comp := k.GetComponent("dashboard")
	require.NotNil(t, comp)

	deploy := comp.ManagedResources[0]
	assert.Equal(t, "Deployment", deploy.Kind)
	assert.NotNil(t, deploy.ExpectedSpec)
}
