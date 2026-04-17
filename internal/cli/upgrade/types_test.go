package upgrade

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStepsReturnsUpgradeSteps(t *testing.T) {
	pb := &PlaybookSpec{
		Kind: "UpgradePlaybook",
		Upgrade: &UpgradeSpec{
			Steps: []PlaybookStep{
				{Name: "validate", Type: "validate-version"},
				{Name: "upgrade", Type: "olm"},
			},
		},
	}
	steps := pb.Steps()
	assert.Len(t, steps, 2)
	assert.Equal(t, "validate", steps[0].Name)
}

func TestStepsReturnsChaosSteps(t *testing.T) {
	pb := &PlaybookSpec{
		Kind: "ChaosPlaybook",
		Chaos: &ChaosSpec{
			KnowledgeDir: "knowledge/v3.3/",
			Steps: []PlaybookStep{
				{Name: "preflight", Type: "validate-version"},
				{Name: "pod-kill", Type: "chaos"},
			},
		},
	}
	steps := pb.Steps()
	assert.Len(t, steps, 2)
	assert.Equal(t, "pod-kill", steps[1].Name)
}

func TestStepsReturnsNilWhenEmpty(t *testing.T) {
	pb := &PlaybookSpec{Kind: "Unknown"}
	assert.Nil(t, pb.Steps())
}

func TestPlaybookStepDependsOn(t *testing.T) {
	step := PlaybookStep{
		Name:      "validate-target",
		Type:      "validate-version",
		DependsOn: []string{"trigger-upgrade"},
	}
	assert.Equal(t, []string{"trigger-upgrade"}, step.DependsOn)
}

func TestPlaybookStepPathRef(t *testing.T) {
	step := PlaybookStep{
		Name:    "upgrade-rhods",
		Type:    "olm",
		PathRef: "rhods-operator",
	}
	assert.Equal(t, "rhods-operator", step.PathRef)
}

func TestPlaybookStepSynthetic(t *testing.T) {
	step := PlaybookStep{
		Name:      "health-check-kserve",
		Type:      "validate-version",
		Synthetic: true,
	}
	assert.True(t, step.Synthetic)
}
