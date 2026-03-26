package convert

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
)

func TestFromCR(t *testing.T) {
	now := metav1.Now()
	recoveryTime := "45s"

	cr := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "omc-podkill",
			Namespace:         "opendatahub",
			CreationTimestamp: now,
			Labels: map[string]string{
				"chaos.opendatahub.io/suite-name":        "omc-full-suite",
				"chaos.opendatahub.io/suite-run-id":      "run-123",
				"chaos.opendatahub.io/operator-version":  "v2.10.0",
			},
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target: v1alpha1.TargetSpec{
				Operator:  "opendatahub-operator",
				Component: "odh-model-controller",
			},
			Injection: v1alpha1.InjectionSpec{
				Type:        v1alpha1.PodKill,
				DangerLevel: v1alpha1.DangerLevelLow,
			},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:     v1alpha1.PhaseComplete,
			Verdict:   v1alpha1.Resilient,
			StartTime: &now,
			EvaluationResult: &v1alpha1.EvaluationSummary{
				Verdict:      v1alpha1.Resilient,
				RecoveryTime: recoveryTime,
			},
		},
	}

	exp, err := FromCR(cr)
	require.NoError(t, err)

	assert.Equal(t, "omc-podkill", exp.Name)
	assert.Equal(t, "opendatahub", exp.Namespace)
	assert.Equal(t, "opendatahub-operator", exp.Operator)
	assert.Equal(t, "odh-model-controller", exp.Component)
	assert.Equal(t, "PodKill", exp.InjectionType)
	assert.Equal(t, "Complete", exp.Phase)
	assert.Equal(t, "Resilient", exp.Verdict)
	assert.Equal(t, "low", exp.DangerLevel)
	require.NotNil(t, exp.RecoveryMs)
	assert.Equal(t, int64(45000), *exp.RecoveryMs)
	assert.Equal(t, "omc-full-suite", exp.SuiteName)
	assert.Equal(t, "run-123", exp.SuiteRunID)
	assert.Equal(t, "v2.10.0", exp.OperatorVersion)
	assert.Contains(t, exp.ID, "opendatahub/omc-podkill/")
}

func TestFromCR_NoStartTime_UsesCreationTimestamp(t *testing.T) {
	now := metav1.Now()
	cr := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test",
			Namespace:         "ns",
			CreationTimestamp: now,
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "op", Component: "comp"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{Phase: v1alpha1.PhasePending},
	}

	exp, err := FromCR(cr)
	require.NoError(t, err)
	assert.Contains(t, exp.ID, now.Time.Format(time.RFC3339))
}
