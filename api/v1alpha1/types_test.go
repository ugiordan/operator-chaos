package v1alpha1

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

func TestChaosExperimentYAMLRoundTrip(t *testing.T) {
	exp := ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dashboard-pod-kill",
			Labels: map[string]string{
				"component": "dashboard",
			},
		},
		Spec: ChaosExperimentSpec{
			Target: TargetSpec{
				Operator:  "opendatahub-operator",
				Component: "dashboard",
				Resource:  "Deployment/odh-dashboard",
			},
			Hypothesis: HypothesisSpec{
				Description:     "Dashboard recovers from pod kill within 60s",
				RecoveryTimeout: metav1.Duration{Duration: 60 * time.Second},
			},
			Injection: InjectionSpec{
				Type:  PodKill,
				Count: 1,
				TTL:   metav1.Duration{Duration: 300 * time.Second},
				Parameters: map[string]string{
					"signal":        "SIGKILL",
					"labelSelector": "app.kubernetes.io/part-of=dashboard",
				},
			},
			BlastRadius: BlastRadiusSpec{
				MaxPodsAffected:   1,
				AllowedNamespaces: []string{"opendatahub"},
			},
		},
	}

	data, err := yaml.Marshal(exp)
	require.NoError(t, err)

	var loaded ChaosExperiment
	err = yaml.Unmarshal(data, &loaded)
	require.NoError(t, err)

	assert.Equal(t, exp.Name, loaded.Name)
	assert.Equal(t, exp.Spec.Target.Component, loaded.Spec.Target.Component)
	assert.Equal(t, exp.Spec.Injection.Type, loaded.Spec.Injection.Type)
	assert.Equal(t, PodKill, loaded.Spec.Injection.Type)
}

func TestChaosExperimentLoadFromFile(t *testing.T) {
	data, err := os.ReadFile("../../testdata/experiments/valid-experiment.yaml")
	require.NoError(t, err)

	var exp ChaosExperiment
	err = yaml.Unmarshal(data, &exp)
	require.NoError(t, err)

	assert.Equal(t, "dashboard-pod-kill-recovery", exp.Name)
	assert.Equal(t, PodKill, exp.Spec.Injection.Type)
	assert.Equal(t, int32(1), exp.Spec.Injection.Count)
	assert.Equal(t, int32(1), exp.Spec.BlastRadius.MaxPodsAffected)
	assert.NotEmpty(t, exp.Spec.Hypothesis.Description)
}

func TestInjectionTypes(t *testing.T) {
	types := []InjectionType{
		PodKill, NetworkPartition, CRDMutation, ConfigDrift,
		WebhookDisrupt, RBACRevoke, FinalizerBlock, ClientFault,
	}
	for _, it := range types {
		assert.NotEmpty(t, string(it))
	}
}

func TestVerdictValues(t *testing.T) {
	assert.Equal(t, Verdict("Resilient"), Resilient)
	assert.Equal(t, Verdict("Degraded"), Degraded)
	assert.Equal(t, Verdict("Failed"), Failed)
	assert.Equal(t, Verdict("Inconclusive"), Inconclusive)
}

func TestValidateInjectionType_ValidType(t *testing.T) {
	err := ValidateInjectionType(PodKill)
	assert.NoError(t, err)
}

func TestValidateInjectionType_ClientFault(t *testing.T) {
	err := ValidateInjectionType(ClientFault)
	assert.NoError(t, err)
}

func TestValidateInjectionType_EmptyString(t *testing.T) {
	err := ValidateInjectionType("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown injection type")
}

func TestValidateInjectionType_Typo(t *testing.T) {
	err := ValidateInjectionType("Podkill")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown injection type")
}

func TestValidInjectionTypes_Count(t *testing.T) {
	types := ValidInjectionTypes()
	assert.Len(t, types, 8)
}

func TestValidInjectionTypes_Sorted(t *testing.T) {
	types := ValidInjectionTypes()
	strs := make([]string, len(types))
	for i, t := range types {
		strs[i] = string(t)
	}
	assert.True(t, sort.StringsAreSorted(strs), "ValidInjectionTypes() should return a sorted slice, got %v", strs)
}

func TestDangerLevelConstants(t *testing.T) {
	assert.Equal(t, DangerLevel("low"), DangerLevelLow)
	assert.Equal(t, DangerLevel("medium"), DangerLevelMedium)
	assert.Equal(t, DangerLevel("high"), DangerLevelHigh)
}

func TestValidateDangerLevel_ValidLevels(t *testing.T) {
	for _, level := range []DangerLevel{DangerLevelLow, DangerLevelMedium, DangerLevelHigh} {
		err := ValidateDangerLevel(level)
		assert.NoError(t, err, "level %q should be valid", level)
	}
}

func TestValidateDangerLevel_EmptyIsValid(t *testing.T) {
	err := ValidateDangerLevel("")
	assert.NoError(t, err, "empty danger level should be valid (means unset)")
}

func TestValidateDangerLevel_InvalidLevel(t *testing.T) {
	err := ValidateDangerLevel("critical")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown danger level")
}

func TestValidateDangerLevel_CaseSensitive(t *testing.T) {
	err := ValidateDangerLevel("High")
	assert.Error(t, err, "danger levels should be case-sensitive")
}

func TestValidDangerLevels_Count(t *testing.T) {
	levels := ValidDangerLevels()
	assert.Len(t, levels, 3)
}

func TestValidDangerLevels_Sorted(t *testing.T) {
	levels := ValidDangerLevels()
	strs := make([]string, len(levels))
	for i, l := range levels {
		strs[i] = string(l)
	}
	assert.True(t, sort.StringsAreSorted(strs), "ValidDangerLevels() should return a sorted slice, got %v", strs)
}

// --- New CRD-specific tests ---

func TestSchemeRegistration(t *testing.T) {
	s := runtime.NewScheme()
	err := AddToScheme(s)
	require.NoError(t, err)

	gvk := GroupVersion.WithKind("ChaosExperiment")
	obj, err := s.New(gvk)
	require.NoError(t, err)
	assert.IsType(t, &ChaosExperiment{}, obj)

	listGVK := GroupVersion.WithKind("ChaosExperimentList")
	listObj, err := s.New(listGVK)
	require.NoError(t, err)
	assert.IsType(t, &ChaosExperimentList{}, listObj)
}

func TestDeepCopy(t *testing.T) {
	original := &ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-experiment",
			Namespace: "test-ns",
			Labels:    map[string]string{"env": "test"},
		},
		Spec: ChaosExperimentSpec{
			Target:    TargetSpec{Operator: "test-operator", Component: "test-component"},
			Injection: InjectionSpec{Type: PodKill},
		},
	}
	copied := original.DeepCopy()
	copied.Name = "modified"
	copied.Labels["env"] = "modified"
	assert.Equal(t, "test-experiment", original.Name)
	assert.Equal(t, "test", original.Labels["env"])
}

func TestExistingExperimentYAMLCompatibility(t *testing.T) {
	path := "../../experiments/odh-model-controller/pod-kill.yaml"
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("experiment file not found at %s (expected outside CI)", path)
	}
	require.NoError(t, err)

	var exp ChaosExperiment
	err = yaml.UnmarshalStrict(data, &exp)
	require.NoError(t, err)

	assert.NotEmpty(t, exp.Name)
	assert.Equal(t, "chaos.opendatahub.io/v1alpha1", exp.APIVersion)
	assert.Equal(t, "ChaosExperiment", exp.Kind)
	assert.NotEmpty(t, exp.Spec.Target.Operator)
	assert.NotEmpty(t, exp.Spec.Injection.Type)
}

func TestDurationWireFormatCompatibility(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{60 * time.Second, `"1m0s"`},
		{300 * time.Second, `"5m0s"`},
		{30 * time.Second, `"30s"`},
	}
	for _, tc := range tests {
		d := metav1.Duration{Duration: tc.duration}
		data, err := json.Marshal(d)
		require.NoError(t, err)
		assert.Equal(t, tc.expected, string(data))

		var parsed metav1.Duration
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, tc.duration, parsed.Duration)
	}
}

func TestConditionTypeConstants(t *testing.T) {
	assert.NotEmpty(t, ConditionSteadyStateEstablished)
	assert.NotEmpty(t, ConditionFaultInjected)
	assert.NotEmpty(t, ConditionRecoveryObserved)
	assert.NotEmpty(t, ConditionComplete)
}
