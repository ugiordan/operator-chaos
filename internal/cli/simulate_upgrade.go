package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/diff"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
)

func newSimulateUpgradeCommand() *cobra.Command {
	var (
		sourceDir string
		targetDir string
		component string
		dryRun    bool
		reportDir string
	)

	cmd := &cobra.Command{
		Use:   "simulate-upgrade",
		Short: "Simulate an upgrade by computing diff and generating experiments",
		Long: `Compares source and target versioned knowledge directories, computes
the structural diff, and generates chaos experiments that simulate the
effects of each detected change. Use --dry-run to preview the generated
experiments without executing them.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 1. Load source and target knowledge dirs
			sourceModels, err := model.LoadKnowledgeDir(sourceDir)
			if err != nil {
				return fmt.Errorf("loading source knowledge: %w", err)
			}
			targetModels, err := model.LoadKnowledgeDir(targetDir)
			if err != nil {
				return fmt.Errorf("loading target knowledge: %w", err)
			}

			// 2. Compute diff
			result := diff.ComputeDiff(sourceModels, targetModels)

			// 3. Filter by component if specified
			if component != "" {
				var filtered []diff.ComponentDiff
				for _, c := range result.Components {
					if c.Component == component {
						filtered = append(filtered, c)
					}
				}
				result.Components = filtered
			}

			fmt.Printf("Source: %s (version: %s)\n", sourceDir, result.SourceVersion)
			fmt.Printf("Target: %s (version: %s)\n", targetDir, result.TargetVersion)
			fmt.Printf("Component diffs: %d\n", len(result.Components))

			if len(result.Components) == 0 {
				fmt.Println("No differences found. Nothing to simulate.")
				return nil
			}

			// 4. Generate experiments
			experiments := diff.GenerateUpgradeExperiments(result, sourceModels, targetModels)

			fmt.Printf("Generated experiments: %d\n", len(experiments))

			if len(experiments) == 0 {
				fmt.Println("No experiments generated from the detected changes.")
				return nil
			}

			// 5. Dry-run: print YAML of each experiment
			if dryRun {
				for i, exp := range experiments {
					data, err := yaml.Marshal(exp)
					if err != nil {
						return fmt.Errorf("marshaling experiment %d: %w", i, err)
					}
					_, _ = fmt.Fprintf(os.Stdout, "---\n%s", string(data))
				}
				return nil
			}

			// 6. Live execution placeholder
			_ = reportDir // will be used when live execution is implemented
			fmt.Println("Live execution not yet implemented. Use --dry-run to preview experiments.")
			return nil
		},
	}

	cmd.Flags().StringVar(&sourceDir, "source", "", "path to source version knowledge directory (required)")
	cmd.Flags().StringVar(&targetDir, "target", "", "path to target version knowledge directory (required)")
	cmd.Flags().StringVar(&component, "component", "", "limit to a specific component")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "output generated experiments without executing")
	cmd.Flags().StringVar(&reportDir, "report-dir", "", "directory for reports")
	_ = cmd.MarkFlagRequired("source")
	_ = cmd.MarkFlagRequired("target")

	return cmd
}
