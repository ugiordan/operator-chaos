package diff

import (
	"testing"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrationRealKnowledgeModels(t *testing.T) {
	sourceDir := "../../knowledge/odh/v2.10"
	targetDir := "../../knowledge/rhoai/v3.3"

	sourceModels, err := model.LoadKnowledgeDir(sourceDir)
	if err != nil {
		t.Skipf("skipping integration test: source dir not found: %v", err)
	}
	targetModels, err := model.LoadKnowledgeDir(targetDir)
	if err != nil {
		t.Skipf("skipping integration test: target dir not found: %v", err)
	}

	result := ComputeDiff(sourceModels, targetModels)

	require.NotNil(t, result)
	assert.Equal(t, "odh", result.Platform)

	// Should detect known renames
	var foundDashboardRename bool
	for _, c := range result.Components {
		if c.Operator == "dashboard" && c.ChangeType == ComponentRenamed {
			foundDashboardRename = true
			assert.Equal(t, "odh-dashboard", c.RenamedFrom)
			assert.Equal(t, "rhods-dashboard", c.Component)
		}
	}
	assert.True(t, foundDashboardRename, "should detect dashboard rename")

	// Should detect namespace moves
	assert.Greater(t, result.Summary.NamespaceMoves, 0, "should detect namespace moves")

	// Should have breaking changes
	assert.Greater(t, result.Summary.BreakingChanges, 0, "should report breaking changes")

	// Experiment generation from real diff
	experiments := GenerateUpgradeExperiments(result, sourceModels, targetModels)
	assert.Greater(t, len(experiments), 0, "should generate at least one simulation experiment")

	// All experiments should have upgrade-simulation label
	for _, exp := range experiments {
		assert.Equal(t, "true", exp.Labels["chaos.opendatahub.io/upgrade-simulation"])
	}
}
