package cli

import (
	"fmt"
	"os"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/diff"
	"github.com/spf13/cobra"
)

func newDiffCRDsCommand() *cobra.Command {
	var (
		sourceCRDs string
		targetCRDs string
		format     string
	)

	cmd := &cobra.Command{
		Use:   "diff-crds",
		Short: "Compare CRD schemas between versions",
		Long:  "Compare OpenAPI v3 schemas embedded in CRD YAML files. Detects field removals, type changes, enum value changes, defaulting shifts, and API version removals.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if sourceCRDs == "" {
				return fmt.Errorf("--source-crds is required (--live mode not yet implemented)")
			}

			report, err := diff.ComputeCRDDiff(sourceCRDs, targetCRDs)
			if err != nil {
				return fmt.Errorf("computing CRD diff: %w", err)
			}

			return diff.FormatCRDDiffReport(os.Stdout, report, format)
		},
	}

	cmd.Flags().StringVar(&sourceCRDs, "source-crds", "", "path to source version CRD YAML directory")
	cmd.Flags().StringVar(&targetCRDs, "target-crds", "", "path to target version CRD YAML directory (required)")
	cmd.Flags().StringVar(&format, "format", "table", "output format: table, json, yaml")
	_ = cmd.MarkFlagRequired("target-crds")

	return cmd
}
