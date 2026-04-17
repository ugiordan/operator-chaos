package upgrade

import (
	"testing"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateUpgradePlaybook(t *testing.T) {
	opts := GenerateUpgradeOpts{
		SourceDir:     "knowledge/v2.10",
		TargetDir:     "knowledge/v2.11",
		SourceVersion: "2.10.0",
		TargetVersion: "2.11.0",
		Operator:      "rhods-operator",
		Namespace:     "redhat-ods-operator",
		Channels:      []string{"stable-2.10", "stable-2.11"},
	}

	pb, err := GenerateUpgradePlaybook(opts)
	require.NoError(t, err)

	assert.Equal(t, "UpgradePlaybook", pb.Kind)
	assert.Equal(t, "chaos.opendatahub.io/v1alpha1", pb.APIVersion)
	assert.Equal(t, "rhods-operator-2.10.0-to-2.11.0", pb.Metadata.Name)
	assert.Contains(t, pb.Metadata.Description, "# REVIEW:")

	require.NotNil(t, pb.Upgrade)
	assert.Equal(t, "knowledge/v2.10", pb.Upgrade.Source.KnowledgeDir)
	assert.Equal(t, "2.10.0", pb.Upgrade.Source.Version)
	assert.Equal(t, "knowledge/v2.11", pb.Upgrade.Target.KnowledgeDir)
	assert.Equal(t, "2.11.0", pb.Upgrade.Target.Version)

	require.Len(t, pb.Upgrade.Paths, 1)
	assert.Equal(t, "rhods-operator", pb.Upgrade.Paths[0].Operator)
	assert.Equal(t, "redhat-ods-operator", pb.Upgrade.Paths[0].Namespace)
	require.Len(t, pb.Upgrade.Paths[0].Hops, 2)
	assert.Equal(t, "stable-2.10", pb.Upgrade.Paths[0].Hops[0].Channel)
	assert.Equal(t, "stable-2.11", pb.Upgrade.Paths[0].Hops[1].Channel)

	// Steps: validate-source, trigger-upgrade, validate-target
	require.Len(t, pb.Upgrade.Steps, 3)
	assert.Equal(t, "validate-source", pb.Upgrade.Steps[0].Name)
	assert.Equal(t, "validate-version", pb.Upgrade.Steps[0].Type)
	assert.Equal(t, "trigger-upgrade", pb.Upgrade.Steps[1].Name)
	assert.Equal(t, "olm", pb.Upgrade.Steps[1].Type)
	assert.Equal(t, "validate-target", pb.Upgrade.Steps[2].Name)
	assert.Equal(t, "validate-version", pb.Upgrade.Steps[2].Type)
	assert.Equal(t, []string{"trigger-upgrade"}, pb.Upgrade.Steps[2].DependsOn)
}

func TestGenerateUpgradePlaybookNoChannels(t *testing.T) {
	opts := GenerateUpgradeOpts{
		SourceDir:     "knowledge/v2.10",
		TargetDir:     "knowledge/v2.11",
		SourceVersion: "2.10.0",
		TargetVersion: "2.11.0",
		Operator:      "rhods-operator",
	}

	pb, err := GenerateUpgradePlaybook(opts)
	require.NoError(t, err)

	// Should have a placeholder hop
	require.Len(t, pb.Upgrade.Paths[0].Hops, 1)
	assert.Contains(t, pb.Upgrade.Paths[0].Hops[0].Channel, "# REVIEW:")
	// Namespace should default
	assert.Equal(t, "openshift-operators", pb.Upgrade.Paths[0].Namespace)
}

func TestGenerateUpgradePlaybookValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		opts GenerateUpgradeOpts
		want string
	}{
		{"missing source version", GenerateUpgradeOpts{TargetVersion: "2.11", Operator: "op"}, "source version"},
		{"missing target version", GenerateUpgradeOpts{SourceVersion: "2.10", Operator: "op"}, "target version"},
		{"missing operator", GenerateUpgradeOpts{SourceVersion: "2.10", TargetVersion: "2.11"}, "operator"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GenerateUpgradePlaybook(tt.opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestGenerateChaosPlaybook(t *testing.T) {
	models := []*model.OperatorKnowledge{
		{
			Operator: model.OperatorMeta{
				Name:      "dashboard",
				Namespace: "redhat-ods-applications",
				Version:   "2.10.0",
			},
			Components: []model.ComponentModel{
				{Name: "odh-dashboard"},
			},
		},
	}

	experiments := map[string][]string{
		"odh-dashboard": {
			"experiments/odh-dashboard/pod-kill.yaml",
			"experiments/odh-dashboard/network-partition.yaml",
		},
	}

	pb, err := GenerateChaosPlaybook(models, experiments, "knowledge/v2.10", "all")
	require.NoError(t, err)

	assert.Equal(t, "ChaosPlaybook", pb.Kind)
	assert.Equal(t, "chaos.opendatahub.io/v1alpha1", pb.APIVersion)
	assert.Contains(t, pb.Metadata.Name, "chaos-dashboard")

	require.NotNil(t, pb.Chaos)
	assert.Equal(t, "knowledge/v2.10", pb.Chaos.KnowledgeDir)

	// Steps: preflight, chaos-odh-dashboard, cleanup
	require.Len(t, pb.Chaos.Steps, 3)
	assert.Equal(t, "preflight", pb.Chaos.Steps[0].Name)
	assert.Equal(t, "validate-version", pb.Chaos.Steps[0].Type)

	assert.Equal(t, "chaos-odh-dashboard", pb.Chaos.Steps[1].Name)
	assert.Equal(t, "chaos", pb.Chaos.Steps[1].Type)
	assert.Equal(t, []string{"preflight"}, pb.Chaos.Steps[1].DependsOn)
	assert.Len(t, pb.Chaos.Steps[1].Experiments, 2)

	assert.Equal(t, "cleanup", pb.Chaos.Steps[2].Name)
	assert.Equal(t, "kubectl", pb.Chaos.Steps[2].Type)
	assert.Contains(t, pb.Chaos.Steps[2].Commands[0], "odh-chaos clean")
	assert.Contains(t, pb.Chaos.Steps[2].Commands[0], "redhat-ods-applications")
}

func TestGenerateChaosPlaybookDangerFilter(t *testing.T) {
	models := []*model.OperatorKnowledge{
		{
			Operator: model.OperatorMeta{
				Name:      "kserve",
				Namespace: "redhat-ods-applications",
			},
			Components: []model.ComponentModel{
				{Name: "kserve-controller"},
			},
		},
	}

	experiments := map[string][]string{
		"kserve-controller": {
			"experiments/kserve-controller/pod-kill.yaml",
			"experiments/kserve-controller/network-partition.yaml",
			"experiments/kserve-controller/webhook-disrupt.yaml",
		},
	}

	// "low" should only include pod-kill
	pb, err := GenerateChaosPlaybook(models, experiments, "knowledge", "low")
	require.NoError(t, err)
	chaosStep := pb.Chaos.Steps[1]
	assert.Len(t, chaosStep.Experiments, 1)
	assert.Contains(t, chaosStep.Experiments[0], "pod-kill")

	// "medium" should include pod-kill and network-partition
	pb, err = GenerateChaosPlaybook(models, experiments, "knowledge", "medium")
	require.NoError(t, err)
	chaosStep = pb.Chaos.Steps[1]
	assert.Len(t, chaosStep.Experiments, 2)

	// "all" should include everything
	pb, err = GenerateChaosPlaybook(models, experiments, "knowledge", "all")
	require.NoError(t, err)
	chaosStep = pb.Chaos.Steps[1]
	assert.Len(t, chaosStep.Experiments, 3)
}

func TestDangerLevel(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"pod-kill.yaml", "low"},
		{"controller-kill.yaml", "low"},
		{"network-partition.yaml", "medium"},
		{"config-drift.yaml", "medium"},
		{"finalizer-block.yaml", "medium"},
		{"crd-mutation.yaml", "medium"},
		{"webhook-disrupt.yaml", "high"},
		{"rbac-revoke.yaml", "high"},
		{"unknown-experiment.yaml", "medium"},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			assert.Equal(t, tt.want, dangerLevel(tt.filename))
		})
	}
}

func TestGenerateChaosPlaybookNoModels(t *testing.T) {
	_, err := GenerateChaosPlaybook(nil, nil, "knowledge", "all")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one knowledge model")
}
