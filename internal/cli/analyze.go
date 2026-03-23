package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/analyzer"
	"github.com/spf13/cobra"
)

func newAnalyzeCommand() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "analyze <directory>",
		Short: "Analyze Go source code for fault injection candidates",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := args[0]

			findings, err := analyzer.AnalyzeDirectory(dir)
			if err != nil {
				return fmt.Errorf("analyzing %s: %w", dir, err)
			}

			if len(findings) == 0 {
				fmt.Println("No fault injection candidates found.")
				return nil
			}

			if jsonOutput {
				encoder := json.NewEncoder(os.Stdout)
				encoder.SetIndent("", "  ")
				return encoder.Encode(findings)
			}

			// Human-readable output
			fmt.Printf("Found %d fault injection candidates in %s:\n\n", len(findings), dir)
			for _, f := range findings {
				fmt.Printf("  [%s] %s:%d - %s (%s)\n", f.Severity, f.File, f.Line, f.Detail, f.Type)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output in JSON format")

	return cmd
}
