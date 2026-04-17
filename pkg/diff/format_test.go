package diff

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatTable(t *testing.T) {
	d := &UpgradeDiff{
		SourceVersion: "2.10",
		TargetVersion: "3.3",
		Platform:      "rhoai",
		Components: []ComponentDiff{
			{
				Operator:   "dashboard",
				Component:  "rhods-dashboard",
				ChangeType: ComponentRenamed,
				RenamedFrom: "odh-dashboard",
				NamespaceChange: &NamespaceChange{From: "opendatahub", To: "redhat-ods-applications"},
			},
		},
		Summary: DiffSummary{ComponentsRenamed: 1, NamespaceMoves: 1, BreakingChanges: 2},
	}

	var buf bytes.Buffer
	err := FormatUpgradeDiff(&buf, d, "table")
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "rhoai v2.10")
	assert.Contains(t, output, "v3.3")
	assert.Contains(t, output, "rhods-dashboard")
	assert.Contains(t, output, "odh-dashboard")
	assert.Contains(t, output, "Renamed")
	assert.Contains(t, output, "opendatahub")
	assert.Contains(t, output, "redhat-ods-applications")
	assert.Contains(t, output, "BREAKING CHANGES: 2")
}

func TestFormatJSON(t *testing.T) {
	d := &UpgradeDiff{
		SourceVersion: "2.10",
		TargetVersion: "3.3",
		Platform:      "rhoai",
		Components:    []ComponentDiff{},
		Summary:       DiffSummary{},
	}

	var buf bytes.Buffer
	err := FormatUpgradeDiff(&buf, d, "json")
	require.NoError(t, err)

	var parsed UpgradeDiff
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "2.10", parsed.SourceVersion)
}
