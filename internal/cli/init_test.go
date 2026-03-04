package cli

import (
	"bytes"
	"strings"
	"testing"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// executeInit runs the init command with the given args and returns stdout output.
func executeInit(t *testing.T, args ...string) string {
	t.Helper()
	cmd := newInitCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())
	return buf.String()
}

// parseExperiment parses YAML output into a ChaosExperiment.
func parseExperiment(t *testing.T, yamlStr string) *v1alpha1.ChaosExperiment {
	t.Helper()
	var exp v1alpha1.ChaosExperiment
	require.NoError(t, yaml.Unmarshal([]byte(yamlStr), &exp))
	return &exp
}

func TestInit_DefaultType_PodKill(t *testing.T) {
	out := executeInit(t, "--component", "dashboard")
	exp := parseExperiment(t, out)

	assert.Equal(t, v1alpha1.PodKill, exp.Spec.Injection.Type)
	assert.Contains(t, exp.Spec.Injection.Parameters, "labelSelector")
	assert.NotEmpty(t, exp.Spec.Injection.Parameters["labelSelector"])

	errs := experiment.Validate(exp)
	assert.Empty(t, errs, "generated PodKill template should pass validation, got: %v", errs)
}

func TestInit_NetworkPartition(t *testing.T) {
	out := executeInit(t, "--component", "dashboard", "--type", "NetworkPartition")
	exp := parseExperiment(t, out)

	assert.Equal(t, v1alpha1.NetworkPartition, exp.Spec.Injection.Type)
	assert.Contains(t, exp.Spec.Injection.Parameters, "labelSelector")
	assert.NotEmpty(t, exp.Spec.Injection.Parameters["labelSelector"])

	errs := experiment.Validate(exp)
	assert.Empty(t, errs, "generated NetworkPartition template should pass validation, got: %v", errs)
}

func TestInit_CRDMutation(t *testing.T) {
	out := executeInit(t, "--component", "dashboard", "--type", "CRDMutation")
	exp := parseExperiment(t, out)

	assert.Equal(t, v1alpha1.CRDMutation, exp.Spec.Injection.Type)
	assert.Contains(t, exp.Spec.Injection.Parameters, "apiVersion")
	assert.Contains(t, exp.Spec.Injection.Parameters, "kind")
	assert.Contains(t, exp.Spec.Injection.Parameters, "name")
	assert.Contains(t, exp.Spec.Injection.Parameters, "field")
	assert.Contains(t, exp.Spec.Injection.Parameters, "value")
	// Should NOT contain labelSelector
	assert.NotContains(t, exp.Spec.Injection.Parameters, "labelSelector")

	errs := experiment.Validate(exp)
	assert.Empty(t, errs, "generated CRDMutation template should pass validation, got: %v", errs)
}

func TestInit_ConfigDrift(t *testing.T) {
	out := executeInit(t, "--component", "dashboard", "--type", "ConfigDrift")
	exp := parseExperiment(t, out)

	assert.Equal(t, v1alpha1.ConfigDrift, exp.Spec.Injection.Type)
	assert.Contains(t, exp.Spec.Injection.Parameters, "name")
	assert.Contains(t, exp.Spec.Injection.Parameters, "key")
	assert.Contains(t, exp.Spec.Injection.Parameters, "value")
	// Should NOT contain labelSelector
	assert.NotContains(t, exp.Spec.Injection.Parameters, "labelSelector")

	errs := experiment.Validate(exp)
	assert.Empty(t, errs, "generated ConfigDrift template should pass validation, got: %v", errs)
}

func TestInit_WebhookDisrupt(t *testing.T) {
	out := executeInit(t, "--component", "dashboard", "--type", "WebhookDisrupt")
	exp := parseExperiment(t, out)

	assert.Equal(t, v1alpha1.WebhookDisrupt, exp.Spec.Injection.Type)
	assert.Contains(t, exp.Spec.Injection.Parameters, "webhookName")
	assert.Contains(t, exp.Spec.Injection.Parameters, "action")
	// Should NOT contain labelSelector
	assert.NotContains(t, exp.Spec.Injection.Parameters, "labelSelector")

	errs := experiment.Validate(exp)
	assert.Empty(t, errs, "generated WebhookDisrupt template should pass validation, got: %v", errs)
}

func TestInit_RBACRevoke(t *testing.T) {
	out := executeInit(t, "--component", "dashboard", "--type", "RBACRevoke")
	exp := parseExperiment(t, out)

	assert.Equal(t, v1alpha1.RBACRevoke, exp.Spec.Injection.Type)
	assert.Contains(t, exp.Spec.Injection.Parameters, "bindingName")
	assert.Contains(t, exp.Spec.Injection.Parameters, "bindingType")
	// Should NOT contain labelSelector
	assert.NotContains(t, exp.Spec.Injection.Parameters, "labelSelector")

	errs := experiment.Validate(exp)
	assert.Empty(t, errs, "generated RBACRevoke template should pass validation, got: %v", errs)
}

func TestInit_FinalizerBlock(t *testing.T) {
	out := executeInit(t, "--component", "dashboard", "--type", "FinalizerBlock")
	exp := parseExperiment(t, out)

	assert.Equal(t, v1alpha1.FinalizerBlock, exp.Spec.Injection.Type)
	assert.Contains(t, exp.Spec.Injection.Parameters, "kind")
	assert.Contains(t, exp.Spec.Injection.Parameters, "name")
	// Should NOT contain labelSelector
	assert.NotContains(t, exp.Spec.Injection.Parameters, "labelSelector")

	errs := experiment.Validate(exp)
	assert.Empty(t, errs, "generated FinalizerBlock template should pass validation, got: %v", errs)
}

func TestInit_NoObservationSection(t *testing.T) {
	out := executeInit(t, "--component", "dashboard")
	assert.False(t, strings.Contains(out, "observation:"),
		"generated template should not contain observation section")
}

func TestInit_NoSteadyStateSection(t *testing.T) {
	out := executeInit(t, "--component", "dashboard")
	assert.False(t, strings.Contains(out, "steadyState:"),
		"generated template should not contain steadyState section")
}

func TestInit_SharedStructure(t *testing.T) {
	out := executeInit(t, "--component", "dashboard", "--operator", "my-operator", "--namespace", "my-ns")
	exp := parseExperiment(t, out)

	assert.Equal(t, "chaos.opendatahub.io/v1alpha1", exp.APIVersion)
	assert.Equal(t, "ChaosExperiment", exp.Kind)
	assert.Contains(t, exp.Metadata.Name, "dashboard")
	assert.Equal(t, "dashboard", exp.Spec.Target.Component)
	assert.Equal(t, "my-operator", exp.Spec.Target.Operator)
	assert.NotEmpty(t, exp.Spec.Hypothesis.Description)
	assert.Contains(t, exp.Spec.BlastRadius.AllowedNamespaces, "my-ns")
}

func TestInit_MissingComponent(t *testing.T) {
	cmd := newInitCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err, "should fail when --component is not provided")
}

func TestInit_DangerLevelForDangerousTypes(t *testing.T) {
	// CRDMutation, WebhookDisrupt, and RBACRevoke are dangerous types
	dangerousTypes := []string{"CRDMutation", "WebhookDisrupt", "RBACRevoke"}
	for _, typ := range dangerousTypes {
		t.Run(typ, func(t *testing.T) {
			out := executeInit(t, "--component", "dashboard", "--type", typ)
			exp := parseExperiment(t, out)
			assert.Equal(t, v1alpha1.DangerLevelHigh, exp.Spec.Injection.DangerLevel,
				"%s should have dangerLevel: high", typ)
			assert.True(t, exp.Spec.BlastRadius.AllowDangerous,
				"%s should have allowDangerous: true in blastRadius", typ)
		})
	}

	// PodKill, NetworkPartition should NOT have dangerLevel set
	safeTypes := []string{"PodKill", "NetworkPartition"}
	for _, typ := range safeTypes {
		t.Run(typ, func(t *testing.T) {
			out := executeInit(t, "--component", "dashboard", "--type", typ)
			exp := parseExperiment(t, out)
			assert.Empty(t, string(exp.Spec.Injection.DangerLevel),
				"%s should not have dangerLevel set", typ)
			assert.False(t, exp.Spec.BlastRadius.AllowDangerous,
				"%s should not have allowDangerous set", typ)
		})
	}
}
