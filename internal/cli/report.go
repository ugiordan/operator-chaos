package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/reporter"
	"github.com/spf13/cobra"
)

func newReportCommand() *cobra.Command {
	var (
		format    string
		outputDir string
	)

	cmd := &cobra.Command{
		Use:   "report [results-directory]",
		Short: "Generate summary reports from experiment results",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := args[0]

			entries, err := os.ReadDir(dir)
			if err != nil {
				return fmt.Errorf("reading results directory %s: %w", dir, err)
			}

			var reports []reporter.ExperimentReport
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
					path := filepath.Join(dir, entry.Name())
					data, err := os.ReadFile(path)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Warning: skipping %s: %v\n", entry.Name(), err)
						continue
					}

					var report reporter.ExperimentReport
					if err := json.Unmarshal(data, &report); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: skipping %s: %v\n", entry.Name(), err)
						continue
					}

					reports = append(reports, report)
				}
			}

			if len(reports) == 0 {
				fmt.Println("No experiment reports found.")
				return nil
			}

			if format == "junit" {
				output := os.Stdout
				if outputDir != "" {
					outPath := filepath.Join(outputDir, "chaos-results.xml")
					f, err := os.Create(outPath)
					if err != nil {
						return fmt.Errorf("creating output file: %w", err)
					}
					defer f.Close()
					output = f
					fmt.Fprintf(os.Stderr, "Writing JUnit report to %s\n", outPath)
				}

				r := reporter.NewJUnitReporter(output)
				return r.WriteSuite("odh-chaos-results", reports)
			}

			// Default: summary
			fmt.Printf("Chaos Engineering Report (%d experiments)\n", len(reports))
			fmt.Println(strings.Repeat("=", 50))
			for _, r := range reports {
				fmt.Printf("  %-30s  %s\n", r.Experiment, r.Evaluation.Verdict)
			}
			fmt.Println(strings.Repeat("=", 50))

			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "summary", "output format (summary, junit)")
	cmd.Flags().StringVar(&outputDir, "output", "", "output directory for reports")

	return cmd
}
