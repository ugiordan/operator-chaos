package experiment

import (
	"os"
	"path/filepath"
	"testing"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Len(t, errs, 6, "should report exactly 6 validation errors")
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
					"signal": "SIGKILL",
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
	assert.Len(t, errs, 6, "should report all missing required fields")
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
