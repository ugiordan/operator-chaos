package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	v1alpha1 "github.com/opendatahub-io/operator-chaos/api/v1alpha1"
	"github.com/opendatahub-io/operator-chaos/pkg/diff"
	"github.com/opendatahub-io/operator-chaos/pkg/experiment"
	"github.com/opendatahub-io/operator-chaos/pkg/model"
)

func newSimulateUpgradeCommand() *cobra.Command {
	var (
		sourceDir       string
		targetDir       string
		component       string
		dryRun          bool
		reportDir       string
		knowledgePaths  []string
		knowledgeDir    string
		timeout         time.Duration
		distributedLock bool
		lockNamespace   string
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

			// 6. Live execution: build orchestrator using target knowledge
			// so the orchestrator has all operator models for multi-operator upgrades.
			verbose, _ := cmd.Flags().GetBool("verbose")

			// Default knowledge-dir to the target directory if not explicitly set,
			// so the orchestrator loads all operator models from the target version.
			effectiveKnowledgeDir := knowledgeDir
			if effectiveKnowledgeDir == "" && len(knowledgePaths) == 0 {
				effectiveKnowledgeDir = targetDir
			}
			deps, err := buildOrchestrator(knowledgePaths, effectiveKnowledgeDir, dryRun, reportDir, distributedLock, lockNamespace, verbose)
			if err != nil {
				return fmt.Errorf("building orchestrator: %w", err)
			}

			fmt.Printf("\nRunning %d upgrade simulation experiments...\n\n", len(experiments))

			passed := 0
			failed := 0
			skipped := 0
			var results []suiteResult

			for i, exp := range experiments {
				expCopy := exp.DeepCopy()
				fmt.Printf("[%d/%d] %s (%s)... ", i+1, len(experiments), expCopy.Name, expCopy.Spec.Injection.Type)

				// Validate before execution
				if errs := experiment.Validate(expCopy); len(errs) > 0 {
					fmt.Printf("SKIP (validation: %v)\n", errs[0])
					r := suiteResult{
						name:    expCopy.Name,
						target:  expCopy.Spec.Target,
						verdict: fmt.Sprintf("SKIP  %s: %d validation errors", expCopy.Name, len(errs)),
						status:  "skip",
						err:     fmt.Errorf("%v", errs[0]),
					}
					results = append(results, r)
					skipped++
					continue
				}

				ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
				orchResult, runErr := deps.Orchestrator.Run(ctx, expCopy)
				cancel()

				r := suiteResult{
					name:   expCopy.Name,
					target: expCopy.Spec.Target,
				}

				if runErr != nil {
					fmt.Printf("ERROR: %v\n", runErr)
					r.verdict = fmt.Sprintf("FAIL  %s: %v", expCopy.Name, runErr)
					r.status = "fail"
					r.err = runErr
					failed++
				} else {
					r.orchResult = orchResult
					switch orchResult.Verdict {
					case v1alpha1.Resilient:
						fmt.Printf("%s\n", colorVerdict("Resilient"))
						r.verdict = fmt.Sprintf("PASS  %s (Resilient)", expCopy.Name)
						r.status = "pass"
						passed++
					case v1alpha1.Degraded, v1alpha1.Failed:
						fmt.Printf("%s\n", colorVerdict(string(orchResult.Verdict)))
						r.verdict = fmt.Sprintf("FAIL  %s (%s)", expCopy.Name, orchResult.Verdict)
						r.status = "fail"
						failed++
					case v1alpha1.Inconclusive:
						fmt.Printf("%s\n", colorVerdict("Inconclusive"))
						r.verdict = fmt.Sprintf("SKIP  %s (Inconclusive)", expCopy.Name)
						r.status = "skip"
						skipped++
					}
				}

				results = append(results, r)
			}

			fmt.Printf("\nUpgrade simulation summary: %d passed, %d failed, %d skipped (total: %d)\n",
				passed, failed, skipped, len(experiments))

			// Write JUnit report if requested
			if reportDir != "" {
				suiteName := fmt.Sprintf("upgrade-%s-to-%s", result.SourceVersion, result.TargetVersion)
				if err := writeSuiteJUnitReport(reportDir, suiteName, results); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to write JUnit report: %v\n", err)
				}
			}

			// Save generated experiment files alongside reports for reproducibility
			if reportDir != "" {
				expDir := filepath.Join(reportDir, "experiments")
				if _, writeErr := writeUpgradeExperimentFiles(experiments, expDir); writeErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to write experiment files: %v\n", writeErr)
				}
			}

			if failed > 0 {
				return fmt.Errorf("%d upgrade simulation experiment(s) failed", failed)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&sourceDir, "source", "", "path to source version knowledge directory (required)")
	cmd.Flags().StringVar(&targetDir, "target", "", "path to target version knowledge directory (required)")
	cmd.Flags().StringVar(&component, "component", "", "limit to a specific component")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "output generated experiments without executing")
	cmd.Flags().StringVar(&reportDir, "report-dir", "", "directory for reports")
	cmd.Flags().StringArrayVar(&knowledgePaths, "knowledge", nil, "path to operator knowledge YAML (for live execution)")
	cmd.Flags().StringVar(&knowledgeDir, "knowledge-dir", "", "directory of operator knowledge YAMLs (for live execution)")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "timeout per experiment")
	cmd.Flags().BoolVar(&distributedLock, "distributed-lock", false, "use Kubernetes Lease-based distributed locking")
	cmd.Flags().StringVar(&lockNamespace, "lock-namespace", v1alpha1.DefaultNamespace, "namespace for distributed lock leases")
	_ = cmd.MarkFlagRequired("source")
	_ = cmd.MarkFlagRequired("target")

	return cmd
}

// writeUpgradeExperimentFiles writes generated experiments to a directory
// for reproducibility. Each experiment is saved as {name}.yaml.
func writeUpgradeExperimentFiles(experiments []v1alpha1.ChaosExperiment, dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("creating experiment directory: %w", err)
	}

	var paths []string
	for i, exp := range experiments {
		data, err := yaml.Marshal(exp)
		if err != nil {
			return nil, fmt.Errorf("marshaling experiment %d: %w", i, err)
		}
		path := filepath.Join(dir, fmt.Sprintf("%s.yaml", exp.Name))
		if err := os.WriteFile(path, data, 0644); err != nil {
			return nil, fmt.Errorf("writing experiment %d: %w", i, err)
		}
		paths = append(paths, path)
	}

	return paths, nil
}
