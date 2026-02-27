package v1alpha1

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func TestChaosExperimentYAMLRoundTrip(t *testing.T) {
	exp := ChaosExperiment{
		Metadata: Metadata{
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
				RecoveryTimeout: Duration{60 * time.Second},
			},
			Injection: InjectionSpec{
				Type:  PodKill,
				Count: 1,
				TTL:   Duration{300 * time.Second},
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

	assert.Equal(t, exp.Metadata.Name, loaded.Metadata.Name)
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

	assert.Equal(t, "dashboard-pod-kill-recovery", exp.Metadata.Name)
	assert.Equal(t, PodKill, exp.Spec.Injection.Type)
	assert.Equal(t, 1, exp.Spec.Injection.Count)
	assert.Equal(t, 1, exp.Spec.BlastRadius.MaxPodsAffected)
	assert.NotEmpty(t, exp.Spec.Hypothesis.Description)
}

func TestInjectionTypes(t *testing.T) {
	types := []InjectionType{
		PodKill, NetworkPartition, CRDMutation, ConfigDrift,
		WebhookDisrupt, RBACRevoke, FinalizerBlock,
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
