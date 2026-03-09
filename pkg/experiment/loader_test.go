package experiment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func TestLoadExperiment(t *testing.T) {
	exp, err := Load("../../testdata/experiments/valid-experiment.yaml")
	require.NoError(t, err)
	assert.Equal(t, "dashboard-pod-kill-recovery", exp.Metadata.Name)
}

func TestLoadExperimentFileNotFound(t *testing.T) {
	_, err := Load("nonexistent.yaml")
	assert.Error(t, err)
}

func TestValidateExperiment(t *testing.T) {
	exp, err := Load("../../testdata/experiments/valid-experiment.yaml")
	require.NoError(t, err)

	errs := Validate(exp)
	assert.Empty(t, errs)
}

func TestValidateExperimentMissingFields(t *testing.T) {
	exp, err := Load("../../testdata/experiments/invalid-experiment.yaml")
	require.NoError(t, err)

	errs := Validate(exp)
	assert.NotEmpty(t, errs)
}

func TestLoadRejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.yaml")
	data := make([]byte, 2*1024*1024) // 2MB, exceeds 1MB limit
	require.NoError(t, os.WriteFile(path, data, 0644))

	_, err := Load(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

func TestValidateBlastRadius(t *testing.T) {
	exp, err := Load("../../testdata/experiments/valid-experiment.yaml")
	require.NoError(t, err)

	// Valid: maxPodsAffected > 0 and allowedNamespaces not empty
	errs := Validate(exp)
	assert.Empty(t, errs)

	// Invalid: no allowed namespaces
	exp.Spec.BlastRadius.AllowedNamespaces = nil
	errs = Validate(exp)
	assert.NotEmpty(t, errs)
}

// --- New comprehensive validation tests ---

func validExperiment() *v1alpha1.ChaosExperiment {
	return &v1alpha1.ChaosExperiment{
		Metadata: v1alpha1.Metadata{
			Name: "test-experiment",
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target: v1alpha1.TargetSpec{
				Operator:  "test-operator",
				Component: "test-component",
			},
			Injection: v1alpha1.InjectionSpec{
				Type: v1alpha1.PodKill,
				Parameters: map[string]string{
					"labelSelector": "app=test",
				},
			},
			Hypothesis: v1alpha1.HypothesisSpec{
				Description: "Test hypothesis",
			},
			BlastRadius: v1alpha1.BlastRadiusSpec{
				MaxPodsAffected:   1,
				AllowedNamespaces: []string{"test-ns"},
			},
		},
	}
}

func TestValidate_ValidExperiment(t *testing.T) {
	exp := validExperiment()
	errs := Validate(exp)
	assert.Empty(t, errs, "a minimal valid experiment should have no validation errors")
}

func TestValidate_MissingName(t *testing.T) {
	exp := validExperiment()
	exp.Metadata.Name = ""
	errs := Validate(exp)
	assert.Contains(t, errs, "metadata.name is required")
}

func TestValidate_MissingOperator(t *testing.T) {
	exp := validExperiment()
	exp.Spec.Target.Operator = ""
	errs := Validate(exp)
	assert.Contains(t, errs, "spec.target.operator is required")
}

func TestValidate_MissingComponent(t *testing.T) {
	exp := validExperiment()
	exp.Spec.Target.Component = ""
	errs := Validate(exp)
	assert.Contains(t, errs, "spec.target.component is required")
}

func TestValidate_MissingInjectionType(t *testing.T) {
	exp := validExperiment()
	exp.Spec.Injection.Type = ""
	errs := Validate(exp)
	assert.Contains(t, errs, "spec.injection.type is required")
}

func TestValidate_MissingHypothesis(t *testing.T) {
	exp := validExperiment()
	exp.Spec.Hypothesis.Description = ""
	errs := Validate(exp)
	assert.Contains(t, errs, "spec.hypothesis.description is required")
}

func TestValidate_MissingAllowedNamespaces(t *testing.T) {
	exp := validExperiment()
	exp.Spec.BlastRadius.AllowedNamespaces = nil
	errs := Validate(exp)
	assert.Contains(t, errs, "spec.blastRadius.allowedNamespaces must not be empty")
}

func TestValidate_EmptyAllowedNamespaces(t *testing.T) {
	exp := validExperiment()
	exp.Spec.BlastRadius.AllowedNamespaces = []string{}
	errs := Validate(exp)
	assert.Contains(t, errs, "spec.blastRadius.allowedNamespaces must not be empty")
}

func TestValidate_MultipleErrors(t *testing.T) {
	exp := &v1alpha1.ChaosExperiment{}
	errs := Validate(exp)
	// All required fields are empty, so we should get multiple errors
	assert.Contains(t, errs, "metadata.name is required")
	assert.Contains(t, errs, "spec.target.operator is required")
	assert.Contains(t, errs, "spec.target.component is required")
	assert.Contains(t, errs, "spec.injection.type is required")
	assert.Contains(t, errs, "spec.hypothesis.description is required")
	assert.Contains(t, errs, "spec.blastRadius.allowedNamespaces must not be empty")
	assert.Contains(t, errs, "spec.blastRadius.maxPodsAffected must be greater than 0")
	assert.Len(t, errs, 7, "should report exactly 7 validation errors")
}

func TestValidate_ValidWithAllFields(t *testing.T) {
	exp := &v1alpha1.ChaosExperiment{
		APIVersion: "chaos.opendatahub.io/v1alpha1",
		Kind:       "ChaosExperiment",
		Metadata: v1alpha1.Metadata{
			Name:      "full-experiment",
			Namespace: "chaos-ns",
			Labels: map[string]string{
				"component": "dashboard",
				"severity":  "high",
			},
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target: v1alpha1.TargetSpec{
				Operator:  "opendatahub-operator",
				Component: "dashboard",
				Resource:  "Deployment/odh-dashboard",
			},
			Injection: v1alpha1.InjectionSpec{
				Type: v1alpha1.PodKill,
				Parameters: map[string]string{
					"labelSelector": "app=dashboard",
				},
				Count: 1,
			},
			Hypothesis: v1alpha1.HypothesisSpec{
				Description: "Dashboard recovers within 60s",
			},
			BlastRadius: v1alpha1.BlastRadiusSpec{
				MaxPodsAffected:    2,
				AllowedNamespaces:  []string{"ns1", "ns2"},
				ForbiddenResources: []string{"etcd"},
				DryRun:             true,
			},
		},
	}
	errs := Validate(exp)
	assert.Empty(t, errs, "fully populated experiment should be valid")
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/to/file.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stat")
}

func TestLoad_FileTooLarge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.yaml")
	// Create a file slightly over 1MB
	data := make([]byte, 1*1024*1024+1)
	require.NoError(t, os.WriteFile(path, data, 0644))

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum size")
}

func TestLoad_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "malformed.yaml")
	content := []byte("this: is: not: valid: yaml:\n  - broken\n    indent: bad\n  wrong: [")
	require.NoError(t, os.WriteFile(path, content, 0644))

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing experiment file")
}

func TestLoad_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	// Write valid YAML that results in an empty struct
	content := []byte("{}")
	require.NoError(t, os.WriteFile(path, content, 0644))

	exp, err := Load(path)
	require.NoError(t, err)
	// The experiment loads successfully but validation should catch all missing fields
	errs := Validate(exp)
	assert.NotEmpty(t, errs, "empty experiment should fail validation")
	assert.Len(t, errs, 7, "should report all missing required fields")
}

func TestValidate_UnknownInjectionType(t *testing.T) {
	exp := validExperiment()
	exp.Spec.Injection.Type = "Podkill" // typo: lowercase k
	errs := Validate(exp)
	assert.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if strings.Contains(e, "unknown injection type") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected error about unknown injection type, got: %v", errs)
}

func TestValidate_InjectionParamsChecked(t *testing.T) {
	exp := validExperiment()
	exp.Spec.Injection.Type = v1alpha1.PodKill
	// PodKill requires a labelSelector parameter
	exp.Spec.Injection.Parameters = nil
	errs := Validate(exp)
	assert.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if strings.Contains(e, "labelSelector") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected error about labelSelector, got: %v", errs)
}

func TestValidate_InjectionParamsValid(t *testing.T) {
	exp := validExperiment()
	exp.Spec.Injection.Type = v1alpha1.PodKill
	exp.Spec.Injection.Parameters = map[string]string{
		"labelSelector": "app=dashboard",
	}
	errs := Validate(exp)
	assert.Empty(t, errs)
}

func TestLoad_RejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown-field.yaml")
	content := []byte(`
metadata:
  name: test
spec:
  target:
    operator: test
    component: test
  injection:
    type: PodKill
  hypothesis:
    description: "test"
    expectedBehavior: "this field does not exist in the struct"
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - default
`)
	require.NoError(t, os.WriteFile(path, content, 0644))

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestLoad_ValidYAMLReturnsExperiment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	content := []byte(`
metadata:
  name: inline-test
spec:
  target:
    operator: my-operator
    component: my-component
  injection:
    type: PodKill
    parameters:
      labelSelector: "app=test"
  hypothesis:
    description: "test hypothesis"
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - default
`)
	require.NoError(t, os.WriteFile(path, content, 0644))

	exp, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "inline-test", exp.Metadata.Name)
	assert.Equal(t, "my-operator", exp.Spec.Target.Operator)
	assert.Equal(t, "my-component", exp.Spec.Target.Component)
	assert.Equal(t, v1alpha1.InjectionType("PodKill"), exp.Spec.Injection.Type)
	assert.Equal(t, "test hypothesis", exp.Spec.Hypothesis.Description)
	assert.Equal(t, []string{"default"}, exp.Spec.BlastRadius.AllowedNamespaces)

	errs := Validate(exp)
	assert.Empty(t, errs)
}

// parseExperimentBytes unmarshals YAML bytes into a ChaosExperiment, bypassing file I/O.
func parseExperimentBytes(data []byte) (*v1alpha1.ChaosExperiment, error) {
	var exp v1alpha1.ChaosExperiment
	if err := yaml.UnmarshalStrict(data, &exp); err != nil {
		return nil, err
	}
	return &exp, nil
}

func FuzzExperimentParse(f *testing.F) {
	// Seed: valid experiment YAML
	f.Add([]byte(`
metadata:
  name: fuzz-test
spec:
  target:
    operator: op
    component: comp
  injection:
    type: PodKill
    parameters:
      labelSelector: "app=test"
  hypothesis:
    description: "test"
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - default
`))
	// Seed: NetworkPartition injection type
	f.Add([]byte(`
metadata:
  name: fuzz-netpart
spec:
  target:
    operator: op
    component: comp
  injection:
    type: NetworkPartition
    parameters:
      labelSelector: "app=test"
  hypothesis:
    description: "test"
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - default
`))
	// Seed: ConfigDrift injection type
	f.Add([]byte(`
metadata:
  name: fuzz-configdrift
spec:
  target:
    operator: op
    component: comp
  injection:
    type: ConfigDrift
    parameters:
      name: my-configmap
      key: app.conf
      value: corrupted
  hypothesis:
    description: "test"
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - default
`))
	// Seed: WebhookDisrupt injection type
	f.Add([]byte(`
metadata:
  name: fuzz-webhook
spec:
  target:
    operator: op
    component: comp
  injection:
    type: WebhookDisrupt
    parameters:
      webhookName: my-webhook
      action: setFailurePolicy
  hypothesis:
    description: "test"
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - default
`))
	// Seed: RBACRevoke injection type
	f.Add([]byte(`
metadata:
  name: fuzz-rbac
spec:
  target:
    operator: op
    component: comp
  injection:
    type: RBACRevoke
    parameters:
      bindingName: my-binding
      bindingType: ClusterRoleBinding
  hypothesis:
    description: "test"
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - default
`))
	// Seed: empty document
	f.Add([]byte("{}"))
	// Seed: empty bytes
	f.Add([]byte(""))
	// Seed: minimal YAML
	f.Add([]byte("metadata: {}"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Mirror the 1MB file size limit from Load() to prevent YAML bombs.
		if len(data) > maxExperimentFileSize {
			return
		}
		exp, err := parseExperimentBytes(data)
		if err != nil {
			return
		}
		// If parsing succeeded, validation must not panic
		_ = Validate(exp)
	})
}
