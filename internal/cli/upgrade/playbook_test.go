package upgrade

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestPlaybook(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "playbook.yaml")
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
	return path
}

const validPlaybook = `
apiVersion: chaos.opendatahub.io/v1alpha1
kind: UpgradePlaybook
metadata:
  name: test-upgrade
  description: "Test upgrade playbook"
upgrade:
  source:
    knowledgeDir: knowledge/rhoai/v2.10/
    version: "2.10"
  target:
    knowledgeDir: knowledge/rhoai/v3.3/
    version: "3.3"
  path:
    operator: rhods-operator
    namespace: redhat-ods-operator
    hops:
      - channel: stable-3.3
        maxWait: 20m
  steps:
    - name: validate-source
      type: validate-version
    - name: trigger-upgrade
      type: olm
    - name: validate-target
      type: validate-version
`

func TestLoadPlaybookValid(t *testing.T) {
	path := writeTestPlaybook(t, validPlaybook)

	pb, err := LoadPlaybook(path)
	require.NoError(t, err)
	assert.Equal(t, "test-upgrade", pb.Metadata.Name)
	require.NotNil(t, pb.Upgrade)
	assert.Equal(t, "2.10", pb.Upgrade.Source.Version)
	assert.Equal(t, "3.3", pb.Upgrade.Target.Version)
	// After backward compat migration, Path is nil and Paths has one entry
	assert.Nil(t, pb.Upgrade.Path)
	require.Len(t, pb.Upgrade.Paths, 1)
	assert.Equal(t, "rhods-operator", pb.Upgrade.Paths[0].Operator)
	assert.Len(t, pb.Upgrade.Paths[0].Hops, 1)
	assert.Len(t, pb.Upgrade.Steps, 3)
}

func TestLoadPlaybookAllStepTypes(t *testing.T) {
	content := `
apiVersion: chaos.opendatahub.io/v1alpha1
kind: UpgradePlaybook
metadata:
  name: full-test
  description: "All step types"
upgrade:
  source:
    knowledgeDir: knowledge/rhoai/v2.10/
    version: "2.10"
  target:
    knowledgeDir: knowledge/rhoai/v3.3/
    version: "3.3"
  path:
    operator: rhods-operator
    namespace: redhat-ods-operator
    hops:
      - channel: stable-3.3
  steps:
    - name: pre-check
      type: validate-version
    - name: migrate
      type: kubectl
      commands:
        - "oc apply -f migration.yaml"
      verify:
        type: resourceExists
        apiVersion: v1
        kind: ConfigMap
        namespace: test-ns
        labelSelector: app=test
    - name: confirm
      type: manual
      description: "Check things"
      autoCheck: "oc get pods"
    - name: upgrade
      type: olm
    - name: chaos
      type: chaos
      experiments:
        - experiments/pod-kill.yaml
      knowledge: knowledge/rhoai/v3.3/
`
	path := writeTestPlaybook(t, content)

	pb, err := LoadPlaybook(path)
	require.NoError(t, err)
	assert.Len(t, pb.Upgrade.Steps, 5)
	assert.Equal(t, "kubectl", pb.Upgrade.Steps[1].Type)
	assert.Len(t, pb.Upgrade.Steps[1].Commands, 1)
	assert.NotNil(t, pb.Upgrade.Steps[1].Verify)
	assert.Equal(t, "manual", pb.Upgrade.Steps[2].Type)
	assert.Equal(t, "oc get pods", pb.Upgrade.Steps[2].AutoCheck)
}

func TestValidatePlaybookMissingName(t *testing.T) {
	content := `
apiVersion: chaos.opendatahub.io/v1alpha1
kind: UpgradePlaybook
metadata:
  description: "No name"
upgrade:
  source:
    knowledgeDir: knowledge/rhoai/v2.10/
    version: "2.10"
  target:
    knowledgeDir: knowledge/rhoai/v3.3/
    version: "3.3"
  path:
    operator: rhods-operator
    namespace: redhat-ods-operator
    hops:
      - channel: stable-3.3
  steps:
    - name: step1
      type: olm
`
	path := writeTestPlaybook(t, content)
	pb, err := LoadPlaybook(path)
	require.NoError(t, err)

	errs := ValidatePlaybook(pb)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0], "metadata.name")
}

func TestValidatePlaybookDuplicateStepNames(t *testing.T) {
	content := `
apiVersion: chaos.opendatahub.io/v1alpha1
kind: UpgradePlaybook
metadata:
  name: test
  description: "Duplicate steps"
upgrade:
  source:
    knowledgeDir: knowledge/rhoai/v2.10/
    version: "2.10"
  target:
    knowledgeDir: knowledge/rhoai/v3.3/
    version: "3.3"
  path:
    operator: rhods-operator
    namespace: redhat-ods-operator
    hops:
      - channel: stable-3.3
  steps:
    - name: step1
      type: olm
    - name: step1
      type: validate-version
`
	path := writeTestPlaybook(t, content)
	pb, err := LoadPlaybook(path)
	require.NoError(t, err)

	errs := ValidatePlaybook(pb)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0], "duplicate step name")
}

func TestValidatePlaybookUnknownStepType(t *testing.T) {
	content := `
apiVersion: chaos.opendatahub.io/v1alpha1
kind: UpgradePlaybook
metadata:
  name: test
  description: "Bad type"
upgrade:
  source:
    knowledgeDir: knowledge/rhoai/v2.10/
    version: "2.10"
  target:
    knowledgeDir: knowledge/rhoai/v3.3/
    version: "3.3"
  path:
    operator: rhods-operator
    namespace: redhat-ods-operator
    hops:
      - channel: stable-3.3
  steps:
    - name: step1
      type: unknown-type
`
	path := writeTestPlaybook(t, content)
	pb, err := LoadPlaybook(path)
	require.NoError(t, err)

	errs := ValidatePlaybook(pb)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0], "unknown step type")
}

func TestValidatePlaybookMissingPath(t *testing.T) {
	content := `
apiVersion: chaos.opendatahub.io/v1alpha1
kind: UpgradePlaybook
metadata:
  name: test
  description: "No path"
upgrade:
  source:
    knowledgeDir: knowledge/rhoai/v2.10/
    version: "2.10"
  target:
    knowledgeDir: knowledge/rhoai/v3.3/
    version: "3.3"
  steps:
    - name: step1
      type: olm
`
	path := writeTestPlaybook(t, content)
	pb, err := LoadPlaybook(path)
	require.NoError(t, err)

	errs := ValidatePlaybook(pb)
	assert.NotEmpty(t, errs)
}

func TestHasShellCommands(t *testing.T) {
	content := `
apiVersion: chaos.opendatahub.io/v1alpha1
kind: UpgradePlaybook
metadata:
  name: test
  description: "Has shell"
upgrade:
  source:
    knowledgeDir: knowledge/rhoai/v2.10/
    version: "2.10"
  target:
    knowledgeDir: knowledge/rhoai/v3.3/
    version: "3.3"
  path:
    operator: rhods-operator
    namespace: redhat-ods-operator
    hops:
      - channel: stable-3.3
  steps:
    - name: migrate
      type: kubectl
      commands:
        - "oc apply -f foo.yaml"
    - name: upgrade
      type: olm
`
	path := writeTestPlaybook(t, content)
	pb, err := LoadPlaybook(path)
	require.NoError(t, err)

	assert.True(t, HasShellCommands(pb))
}

func TestHasShellCommandsFalse(t *testing.T) {
	path := writeTestPlaybook(t, validPlaybook)
	pb, err := LoadPlaybook(path)
	require.NoError(t, err)

	assert.False(t, HasShellCommands(pb))
}

func TestResolveKnowledgeDirExplicit(t *testing.T) {
	pb := &PlaybookSpec{
		Upgrade: &UpgradeSpec{
			Source: VersionRef{KnowledgeDir: "knowledge/v1/"},
			Target: VersionRef{KnowledgeDir: "knowledge/v2/"},
			Steps: []PlaybookStep{
				{Name: "step1", Type: "validate-version", KnowledgeDir: "knowledge/custom/"},
			},
		},
	}
	result := ResolveKnowledgeDir(pb.Upgrade.Steps[0], pb)
	assert.Equal(t, "knowledge/custom/", result)
}

func TestResolveKnowledgeDirBeforeOLM(t *testing.T) {
	pb := &PlaybookSpec{
		Upgrade: &UpgradeSpec{
			Source: VersionRef{KnowledgeDir: "knowledge/v1/"},
			Target: VersionRef{KnowledgeDir: "knowledge/v2/"},
			Steps: []PlaybookStep{
				{Name: "pre-check", Type: "validate-version"},
				{Name: "upgrade", Type: "olm"},
				{Name: "post-check", Type: "validate-version"},
			},
		},
	}
	result := ResolveKnowledgeDir(pb.Upgrade.Steps[0], pb)
	assert.Equal(t, "knowledge/v1/", result)
}

func TestResolveKnowledgeDirAfterOLM(t *testing.T) {
	pb := &PlaybookSpec{
		Upgrade: &UpgradeSpec{
			Source: VersionRef{KnowledgeDir: "knowledge/v1/"},
			Target: VersionRef{KnowledgeDir: "knowledge/v2/"},
			Steps: []PlaybookStep{
				{Name: "pre-check", Type: "validate-version"},
				{Name: "upgrade", Type: "olm"},
				{Name: "post-check", Type: "validate-version"},
			},
		},
	}
	result := ResolveKnowledgeDir(pb.Upgrade.Steps[2], pb)
	assert.Equal(t, "knowledge/v2/", result)
}

func TestResolveKnowledgeDirNoOLMStep(t *testing.T) {
	pb := &PlaybookSpec{
		Upgrade: &UpgradeSpec{
			Source: VersionRef{KnowledgeDir: "knowledge/v1/"},
			Target: VersionRef{KnowledgeDir: "knowledge/v2/"},
			Steps: []PlaybookStep{
				{Name: "check", Type: "validate-version"},
				{Name: "chaos", Type: "chaos"},
			},
		},
	}
	// With no OLM step, should default to target
	result := ResolveKnowledgeDir(pb.Upgrade.Steps[0], pb)
	assert.Equal(t, "knowledge/v2/", result)
	result = ResolveKnowledgeDir(pb.Upgrade.Steps[1], pb)
	assert.Equal(t, "knowledge/v2/", result)
}

// Task 3 tests

func TestLoadChaosPlaybook(t *testing.T) {
	content := `
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosPlaybook
metadata:
  name: chaos-test
  description: "Chaos playbook"
chaos:
  knowledgeDir: knowledge/rhoai/v3.3/
  steps:
    - name: pod-kill
      type: chaos
      experiments:
        - experiments/pod-kill.yaml
    - name: verify
      type: validate-version
`
	path := writeTestPlaybook(t, content)
	pb, err := LoadPlaybook(path)
	require.NoError(t, err)

	assert.Equal(t, "ChaosPlaybook", pb.Kind)
	require.NotNil(t, pb.Chaos)
	assert.Equal(t, "knowledge/rhoai/v3.3/", pb.Chaos.KnowledgeDir)
	assert.Len(t, pb.Steps(), 2)
	assert.Equal(t, "pod-kill", pb.Steps()[0].Name)
	assert.Nil(t, pb.Upgrade)
}

func TestLoadUpgradePlaybookSinglePathBackwardCompat(t *testing.T) {
	// Uses old-style single `path:` field
	path := writeTestPlaybook(t, validPlaybook)
	pb, err := LoadPlaybook(path)
	require.NoError(t, err)

	// Path should be migrated to Paths
	assert.Nil(t, pb.Upgrade.Path, "old Path field should be nil after migration")
	require.Len(t, pb.Upgrade.Paths, 1, "Path should be converted to single-element Paths")
	assert.Equal(t, "rhods-operator", pb.Upgrade.Paths[0].Operator)
	assert.Equal(t, "redhat-ods-operator", pb.Upgrade.Paths[0].Namespace)
	assert.Len(t, pb.Upgrade.Paths[0].Hops, 1)
}

func TestLoadUpgradePlaybookMultiPath(t *testing.T) {
	content := `
apiVersion: chaos.opendatahub.io/v1alpha1
kind: UpgradePlaybook
metadata:
  name: multi-path-test
  description: "Multi-path upgrade"
upgrade:
  source:
    knowledgeDir: knowledge/rhoai/v2.10/
    version: "2.10"
  target:
    knowledgeDir: knowledge/rhoai/v3.3/
    version: "3.3"
  paths:
    - name: rhods
      operator: rhods-operator
      namespace: redhat-ods-operator
      hops:
        - channel: stable-3.3
    - name: serverless
      operator: serverless-operator
      namespace: openshift-serverless
      dependsOn:
        - rhods
      hops:
        - channel: stable-1.33
  steps:
    - name: upgrade-rhods
      type: olm
      pathRef: rhods
    - name: upgrade-serverless
      type: olm
      pathRef: serverless
      dependsOn:
        - upgrade-rhods
`
	path := writeTestPlaybook(t, content)
	pb, err := LoadPlaybook(path)
	require.NoError(t, err)

	assert.Nil(t, pb.Upgrade.Path)
	require.Len(t, pb.Upgrade.Paths, 2)
	assert.Equal(t, "rhods", pb.Upgrade.Paths[0].Name)
	assert.Equal(t, "serverless", pb.Upgrade.Paths[1].Name)
	assert.Equal(t, []string{"rhods"}, pb.Upgrade.Paths[1].DependsOn)
	assert.Equal(t, "rhods", pb.Upgrade.Steps[0].PathRef)
	assert.Equal(t, "serverless", pb.Upgrade.Steps[1].PathRef)
	assert.Equal(t, []string{"upgrade-rhods"}, pb.Upgrade.Steps[1].DependsOn)
}

// Task 4 tests

func TestValidateChaosPlaybookValid(t *testing.T) {
	pb := &PlaybookSpec{
		APIVersion: "chaos.opendatahub.io/v1alpha1",
		Kind:       "ChaosPlaybook",
		Metadata: PlaybookMetadata{
			Name:        "chaos-test",
			Description: "A chaos test",
		},
		Chaos: &ChaosSpec{
			KnowledgeDir: "knowledge/rhoai/v3.3/",
			Steps: []PlaybookStep{
				{Name: "pod-kill", Type: "chaos"},
			},
		},
	}
	errs := ValidatePlaybook(pb)
	assert.Empty(t, errs)
}

func TestValidateChaosPlaybookMissingKnowledgeDir(t *testing.T) {
	pb := &PlaybookSpec{
		APIVersion: "chaos.opendatahub.io/v1alpha1",
		Kind:       "ChaosPlaybook",
		Metadata: PlaybookMetadata{
			Name:        "chaos-test",
			Description: "A chaos test",
		},
		Chaos: &ChaosSpec{
			Steps: []PlaybookStep{
				{Name: "pod-kill", Type: "chaos"},
			},
		},
	}
	errs := ValidatePlaybook(pb)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0], "chaos.knowledgeDir")
}

func TestValidateUnknownKind(t *testing.T) {
	pb := &PlaybookSpec{
		Kind: "SomethingElse",
		Metadata: PlaybookMetadata{
			Name:        "test",
			Description: "test",
		},
	}
	errs := ValidatePlaybook(pb)
	assert.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if assert.ObjectsAreEqual(e, "") {
			continue
		}
		if contains(e, "unknown kind") {
			found = true
		}
	}
	assert.True(t, found, "expected error about unknown kind, got: %v", errs)
}

func TestValidateUpgradePlaybookMultiPathMissingPathRef(t *testing.T) {
	pb := &PlaybookSpec{
		APIVersion: "chaos.opendatahub.io/v1alpha1",
		Kind:       "UpgradePlaybook",
		Metadata: PlaybookMetadata{
			Name:        "multi-path",
			Description: "Multi-path test",
		},
		Upgrade: &UpgradeSpec{
			Source: VersionRef{KnowledgeDir: "knowledge/v1/", Version: "1.0"},
			Target: VersionRef{KnowledgeDir: "knowledge/v2/", Version: "2.0"},
			Paths: []UpgradePath{
				{Name: "op1", Operator: "op1", Namespace: "ns1", Hops: []Hop{{Channel: "c1"}}},
				{Name: "op2", Operator: "op2", Namespace: "ns2", Hops: []Hop{{Channel: "c2"}}},
			},
			Steps: []PlaybookStep{
				{Name: "upgrade-op1", Type: "olm"}, // missing pathRef
				{Name: "upgrade-op2", Type: "olm", PathRef: "op2"},
			},
		},
	}
	errs := ValidatePlaybook(pb)
	assert.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if contains(e, "pathRef is required") {
			found = true
		}
	}
	assert.True(t, found, "expected pathRef error, got: %v", errs)
}

func TestValidateDependsOnReferencesExistingStep(t *testing.T) {
	pb := &PlaybookSpec{
		APIVersion: "chaos.opendatahub.io/v1alpha1",
		Kind:       "ChaosPlaybook",
		Metadata: PlaybookMetadata{
			Name:        "deps-test",
			Description: "Test dependsOn validation",
		},
		Chaos: &ChaosSpec{
			KnowledgeDir: "knowledge/v3.3/",
			Steps: []PlaybookStep{
				{Name: "step-a", Type: "chaos"},
				{Name: "step-b", Type: "chaos", DependsOn: []string{"nonexistent-step"}},
			},
		},
	}
	errs := ValidatePlaybook(pb)
	assert.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if contains(e, "non-existent step") {
			found = true
		}
	}
	assert.True(t, found, "expected dependsOn error, got: %v", errs)
}

func TestResolveKnowledgeDirChaosPlaybook(t *testing.T) {
	pb := &PlaybookSpec{
		Kind: "ChaosPlaybook",
		Chaos: &ChaosSpec{
			KnowledgeDir: "knowledge/chaos/",
			Steps: []PlaybookStep{
				{Name: "pod-kill", Type: "chaos"},
			},
		},
	}
	result := ResolveKnowledgeDir(pb.Chaos.Steps[0], pb)
	assert.Equal(t, "knowledge/chaos/", result)
}

// contains checks if s contains substr (helper to avoid importing strings).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
