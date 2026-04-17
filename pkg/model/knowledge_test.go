package model

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
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

func TestOperatorMetaVersionFields(t *testing.T) {
	yamlData := `
operator:
  name: dashboard
  namespace: redhat-ods-applications
  repository: https://github.com/opendatahub-io/odh-dashboard
  version: "3.3.1"
  platform: rhoai
  olmChannel: stable-3.3
components:
  - name: rhods-dashboard
    controller: DataScienceCluster
    managedResources:
      - apiVersion: apps/v1
        kind: Deployment
        name: rhods-dashboard
        namespace: redhat-ods-applications
recovery:
  reconcileTimeout: "300s"
  maxReconcileCycles: 10
`
	var k OperatorKnowledge
	err := yaml.UnmarshalStrict([]byte(yamlData), &k)
	require.NoError(t, err)
	assert.Equal(t, "3.3.1", k.Operator.Version)
	assert.Equal(t, "rhoai", k.Operator.Platform)
	assert.Equal(t, "stable-3.3", k.Operator.OLMChannel)
}

func TestOperatorMetaVersionFieldsOptional(t *testing.T) {
	// Existing knowledge files without version fields must still load
	k, err := LoadKnowledge("../../knowledge/dashboard.yaml")
	require.NoError(t, err)
	assert.Equal(t, "dashboard", k.Operator.Name)
	assert.Empty(t, k.Operator.Version)
	assert.Empty(t, k.Operator.Platform)
	assert.Empty(t, k.Operator.OLMChannel)
}

// parseKnowledgeBytes unmarshals YAML bytes into an OperatorKnowledge, bypassing file I/O.
func parseKnowledgeBytes(data []byte) (*OperatorKnowledge, error) {
	var k OperatorKnowledge
	if err := yaml.UnmarshalStrict(data, &k); err != nil {
		return nil, err
	}
	return &k, nil
}

func FuzzKnowledgeParse(f *testing.F) {
	// Seed: valid knowledge YAML
	f.Add([]byte(`
operator:
  name: test-operator
  namespace: test-ns
components:
  - name: dashboard
    controller: dashboard-controller
    managedResources:
      - apiVersion: apps/v1
        kind: Deployment
        name: odh-dashboard
recovery:
  reconcileTimeout: "300s"
  maxReconcileCycles: 10
`))
	// Seed: knowledge with webhooks and dependencies
	f.Add([]byte(`
operator:
  name: test-operator
  namespace: test-ns
components:
  - name: dashboard
    controller: dashboard-controller
    managedResources:
      - apiVersion: apps/v1
        kind: Deployment
        name: odh-dashboard
    webhooks:
      - name: validating.dashboard
        type: validating
        path: /validate
    dependencies:
      - model-controller
  - name: model-controller
    controller: kserve-controller
    managedResources:
      - apiVersion: apps/v1
        kind: Deployment
        name: odh-model-controller
recovery:
  reconcileTimeout: "300s"
  maxReconcileCycles: 10
`))
	// Seed: empty document
	f.Add([]byte("{}"))
	// Seed: empty bytes
	f.Add([]byte(""))
	// Seed: minimal
	f.Add([]byte("operator: {}"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Mirror the 1MB file size limit from LoadKnowledge() to prevent YAML bombs.
		if len(data) > maxModelFileSize {
			return
		}
		k, err := parseKnowledgeBytes(data)
		if err != nil {
			return
		}
		// If parsing succeeded, exercise accessors and validation without panicking
		for i := range k.Components {
			_ = k.GetComponent(k.Components[i].Name)
		}
		_ = ValidateKnowledge(k)
	})
}
