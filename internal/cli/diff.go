package cli

import (
	"fmt"
	"os"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/diff"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
	"github.com/spf13/cobra"
)

func newDiffCommand() *cobra.Command {
	var (
		sourceDir string
		targetDir string
		format    string
		breaking  bool
	)

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare two versioned knowledge model directories",
		Long:  "Structural comparison of operator knowledge models between two versions. Detects renames, namespace moves, webhook changes, and dependency shifts. No cluster access required.",
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceModels, err := model.LoadKnowledgeDir(sourceDir)
			if err != nil {
				return fmt.Errorf("loading source knowledge: %w", err)
			}
			targetModels, err := model.LoadKnowledgeDir(targetDir)
			if err != nil {
				return fmt.Errorf("loading target knowledge: %w", err)
			}

			result := diff.ComputeDiff(sourceModels, targetModels)

			if breaking {
				var filtered []diff.ComponentDiff
				for _, c := range result.Components {
					if c.IsBreaking() {
						filtered = append(filtered, c)
					}
				}
				result.Components = filtered
			}

			return diff.FormatUpgradeDiff(os.Stdout, result, format)
		},
	}

	cmd.Flags().StringVar(&sourceDir, "source", "", "path to source version knowledge directory (required)")
	cmd.Flags().StringVar(&targetDir, "target", "", "path to target version knowledge directory (required)")
	cmd.Flags().StringVar(&format, "format", "table", "output format: table, json, yaml")
	cmd.Flags().BoolVar(&breaking, "breaking", false, "only show breaking changes")
	_ = cmd.MarkFlagRequired("source")
	_ = cmd.MarkFlagRequired("target")

	return cmd
}
