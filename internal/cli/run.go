package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/experiment"
	"github.com/spf13/cobra"
)

func newRunCommand() *cobra.Command {
	var (
		knowledgePaths  []string
		knowledgeDir    string
		reportDir       string
		dryRun          bool
		timeout         time.Duration
		distributedLock bool
		lockNamespace   string
	)

	cmd := &cobra.Command{
		Use:   "run <experiment.yaml>",
		Short: "Run a chaos experiment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			// Load experiment
			exp, err := experiment.Load(args[0])
			if err != nil {
				return fmt.Errorf("loading experiment: %w", err)
			}

			// Validate
			if errs := experiment.Validate(exp); len(errs) > 0 {
				fmt.Fprintln(os.Stderr, "Validation errors:")
				for _, e := range errs {
					fmt.Fprintf(os.Stderr, "  - %s\n", e)
				}
				return fmt.Errorf("%d validation errors", len(errs))
			}

			// Override namespace from CLI flag
			namespace, _ := cmd.Flags().GetString("namespace")
			if namespace != "" {
				overrideExperimentNamespace(exp, namespace)
			}

			// Override dry-run from CLI flag
			if dryRun {
				exp.Spec.BlastRadius.DryRun = true
			}

			// Read verbose from persistent flags
			verbose, _ := cmd.Flags().GetBool("verbose")

			// Build orchestrator and all dependencies
			deps, err := buildOrchestrator(knowledgePaths, knowledgeDir, dryRun, reportDir, distributedLock, lockNamespace, verbose)
			if err != nil {
				return err
			}

			// Run
			result, err := deps.Orchestrator.Run(ctx, exp)
			if err != nil {
				return fmt.Errorf("experiment failed: %w", err)
			}

			// Print summary
			printExperimentResult(result)

			// Exit non-zero for non-Resilient verdicts so CI pipelines can gate on results
			switch result.Verdict {
			case v1alpha1.Resilient:
				return nil
			case v1alpha1.Degraded, v1alpha1.Failed:
				return fmt.Errorf("experiment verdict: %s", result.Verdict)
			case v1alpha1.Inconclusive:
				return fmt.Errorf("experiment verdict: Inconclusive")
			default:
				return fmt.Errorf("experiment verdict: %s", result.Verdict)
			}
		},
	}

	cmd.Flags().StringArrayVar(&knowledgePaths, "knowledge", nil, "path to operator knowledge YAML (repeatable)")

	cmd.Flags().StringVar(&knowledgeDir, "knowledge-dir", "", "directory of operator knowledge YAMLs")
	cmd.Flags().StringVar(&reportDir, "report-dir", "", "directory for report output")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate without injecting")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "total experiment timeout")
	cmd.Flags().BoolVar(&distributedLock, "distributed-lock", false, "use Kubernetes Lease-based distributed locking")
	cmd.Flags().StringVar(&lockNamespace, "lock-namespace", v1alpha1.DefaultNamespace, "namespace for distributed lock leases")

	return cmd
}

// overrideExperimentNamespace updates the experiment's metadata namespace,
// steady-state check namespaces, and blast radius allowedNamespaces to use
// the given namespace. This allows the --namespace CLI flag to fully override
// hardcoded namespace references in experiment YAML files.
func overrideExperimentNamespace(exp *v1alpha1.ChaosExperiment, namespace string) {
	exp.Namespace = namespace

	// Override steady-state check namespaces
	for i := range exp.Spec.SteadyState.Checks {
		if exp.Spec.SteadyState.Checks[i].Namespace != "" {
			exp.Spec.SteadyState.Checks[i].Namespace = namespace
		}
	}

	// Override allowedNamespaces in blast radius
	if len(exp.Spec.BlastRadius.AllowedNamespaces) > 0 {
		seen := make(map[string]bool)
		updated := make([]string, 0, len(exp.Spec.BlastRadius.AllowedNamespaces))
		for _, ns := range exp.Spec.BlastRadius.AllowedNamespaces {
			replacement := namespace
			if ns == "" {
				replacement = ns
			}
			if !seen[replacement] {
				seen[replacement] = true
				updated = append(updated, replacement)
			}
		}
		exp.Spec.BlastRadius.AllowedNamespaces = updated
	}
}
