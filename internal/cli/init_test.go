package cli

import (
	"bytes"
	"strings"
	"testing"

	v1alpha1 "github.com/opendatahub-io/operator-chaos/api/v1alpha1"
	"github.com/opendatahub-io/operator-chaos/pkg/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// executeInit runs the init command with the given args and returns stdout output.
// If --operator is not provided, it defaults to "test-operator".
func executeInit(t *testing.T, args ...string) string {
	t.Helper()
	hasOperator := false
	for _, a := range args {
		if a == "--operator" {
			hasOperator = true
			break
		}
	}
	if !hasOperator {
		args = append(args, "--operator", "test-operator")
	}
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

	assert.Equal(t, "chaos.operatorchaos.io/v1alpha1", exp.APIVersion)
	assert.Equal(t, "ChaosExperiment", exp.Kind)
	assert.Contains(t, exp.Name, "dashboard")
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

func TestInit_ClientFault(t *testing.T) {
	out := executeInit(t, "--component", "dashboard", "--type", "ClientFault")
	exp := parseExperiment(t, out)

	assert.Equal(t, v1alpha1.ClientFault, exp.Spec.Injection.Type)
	assert.Contains(t, exp.Spec.Injection.Parameters, "faults")
	assert.NotEmpty(t, exp.Spec.Injection.Parameters["faults"])
	assert.Equal(t, v1alpha1.DangerLevelMedium, exp.Spec.Injection.DangerLevel)

	errs := experiment.Validate(exp)
	assert.Empty(t, errs, "generated ClientFault template should pass validation, got: %v", errs)
}

func TestInit_TierPerInjectionType(t *testing.T) {
	cases := map[string]int32{
		"PodKill":           1,
		"ConfigDrift":       2,
		"NetworkPartition":  2,
		"CRDMutation":       3,
		"FinalizerBlock":    3,
		"OwnerRefOrphan":    3,
		"LabelStomping":     3,
		"ClientFault":       3,
		"WebhookDisrupt":    4,
		"RBACRevoke":        4,
		"WebhookLatency":    4,
		"NamespaceDeletion":  5,
		"QuotaExhaustion":   5,
	}
	for injType, expectedTier := range cases {
		t.Run(injType, func(t *testing.T) {
			out := executeInit(t, "--component", "test", "--type", injType)
			exp := parseExperiment(t, out)
			assert.Equal(t, expectedTier, exp.Spec.Tier,
				"%s should generate tier %d", injType, expectedTier)
		})
	}
}

func TestInit_OwnerRefOrphan(t *testing.T) {
	out := executeInit(t, "--component", "dashboard", "--type", "OwnerRefOrphan")
	exp := parseExperiment(t, out)

	assert.Equal(t, v1alpha1.OwnerRefOrphan, exp.Spec.Injection.Type)
	assert.Contains(t, exp.Spec.Injection.Parameters, "apiVersion")
	assert.Contains(t, exp.Spec.Injection.Parameters, "kind")
	assert.Contains(t, exp.Spec.Injection.Parameters, "name")
	assert.NotContains(t, exp.Spec.Injection.Parameters, "labelSelector")

	errs := experiment.Validate(exp)
	assert.Empty(t, errs, "generated OwnerRefOrphan template should pass validation, got: %v", errs)
}

func TestInit_LabelStomping(t *testing.T) {
	out := executeInit(t, "--component", "dashboard", "--type", "LabelStomping")
	exp := parseExperiment(t, out)

	assert.Equal(t, v1alpha1.LabelStomping, exp.Spec.Injection.Type)
	assert.Contains(t, exp.Spec.Injection.Parameters, "apiVersion")
	assert.Contains(t, exp.Spec.Injection.Parameters, "kind")
	assert.Contains(t, exp.Spec.Injection.Parameters, "name")
	assert.Contains(t, exp.Spec.Injection.Parameters, "labelKey")
	assert.Contains(t, exp.Spec.Injection.Parameters, "action")
	assert.Equal(t, v1alpha1.DangerLevelMedium, exp.Spec.Injection.DangerLevel)
	assert.NotContains(t, exp.Spec.Injection.Parameters, "labelSelector")
	assert.NotContains(t, exp.Spec.Injection.Parameters["labelKey"], "kubernetes.io/",
		"default skeleton should not use system labels that require dangerLevel: high")

	errs := experiment.Validate(exp)
	assert.Empty(t, errs, "generated LabelStomping template should pass validation, got: %v", errs)
}

func TestInit_WebhookLatency(t *testing.T) {
	out := executeInit(t, "--component", "dashboard", "--type", "WebhookLatency")
	exp := parseExperiment(t, out)

	assert.Equal(t, v1alpha1.WebhookLatency, exp.Spec.Injection.Type)
	assert.Contains(t, exp.Spec.Injection.Parameters, "webhookName")
	assert.Contains(t, exp.Spec.Injection.Parameters, "apiGroups")
	assert.Contains(t, exp.Spec.Injection.Parameters, "delay")
	assert.Equal(t, v1alpha1.DangerLevelHigh, exp.Spec.Injection.DangerLevel)
	assert.True(t, exp.Spec.BlastRadius.AllowDangerous)
	assert.NotContains(t, exp.Spec.Injection.Parameters, "labelSelector")

	errs := experiment.Validate(exp)
	assert.Empty(t, errs, "generated WebhookLatency template should pass validation, got: %v", errs)
}

func TestInit_NamespaceDeletion(t *testing.T) {
	out := executeInit(t, "--component", "dashboard", "--type", "NamespaceDeletion")
	exp := parseExperiment(t, out)

	assert.Equal(t, v1alpha1.NamespaceDeletion, exp.Spec.Injection.Type)
	assert.Contains(t, exp.Spec.Injection.Parameters, "namespace")
	assert.Equal(t, v1alpha1.DangerLevelHigh, exp.Spec.Injection.DangerLevel)
	assert.True(t, exp.Spec.BlastRadius.AllowDangerous)
	assert.NotContains(t, exp.Spec.Injection.Parameters, "labelSelector")

	errs := experiment.Validate(exp)
	assert.Empty(t, errs, "generated NamespaceDeletion template should pass validation, got: %v", errs)
}

func TestInit_QuotaExhaustion(t *testing.T) {
	out := executeInit(t, "--component", "dashboard", "--type", "QuotaExhaustion")
	exp := parseExperiment(t, out)

	assert.Equal(t, v1alpha1.QuotaExhaustion, exp.Spec.Injection.Type)
	assert.Contains(t, exp.Spec.Injection.Parameters, "quotaName")
	assert.Contains(t, exp.Spec.Injection.Parameters, "cpu")
	assert.Equal(t, v1alpha1.DangerLevelHigh, exp.Spec.Injection.DangerLevel)
	assert.True(t, exp.Spec.BlastRadius.AllowDangerous)
	assert.NotContains(t, exp.Spec.Injection.Parameters, "labelSelector")

	errs := experiment.Validate(exp)
	assert.Empty(t, errs, "generated QuotaExhaustion template should pass validation, got: %v", errs)
}

func TestInit_DangerLevelForDangerousTypes(t *testing.T) {
	dangerousTypes := []string{"WebhookDisrupt", "RBACRevoke", "WebhookLatency", "NamespaceDeletion", "QuotaExhaustion"}
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
