package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/evaluator"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/experiment"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/injection"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/observer"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/orchestrator"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func newRunCommand() *cobra.Command {
	var (
		knowledgePath   string
		reportDir       string
		dryRun          bool
		timeout         time.Duration
		distributedLock bool
		lockNamespace   string
	)

	cmd := &cobra.Command{
		Use:   "run [experiment.yaml]",
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
				fmt.Println("Validation errors:")
				for _, e := range errs {
					fmt.Printf("  - %s\n", e)
				}
				return fmt.Errorf("%d validation errors", len(errs))
			}

			// Override dry-run from CLI flag
			if dryRun {
				exp.Spec.BlastRadius.DryRun = true
			}

			// Load knowledge (optional)
			var knowledge *model.OperatorKnowledge
			if knowledgePath != "" {
				knowledge, err = model.LoadKnowledge(knowledgePath)
				if err != nil {
					return fmt.Errorf("loading knowledge: %w", err)
				}
			}

			// Create report dir if specified
			if reportDir != "" {
				if err := os.MkdirAll(reportDir, 0755); err != nil {
					return fmt.Errorf("creating report directory: %w", err)
				}
			}

			// Build K8s client
			var k8sClient client.Client
			if !exp.Spec.BlastRadius.DryRun {
				cfg, err := config.GetConfig()
				if err != nil {
					return fmt.Errorf("getting kubeconfig: %w", err)
				}
				k8sClient, err = client.New(cfg, client.Options{})
				if err != nil {
					return fmt.Errorf("creating k8s client: %w", err)
				}
			}

			// Build injection registry
			registry := injection.NewRegistry()
			registry.Register(v1alpha1.PodKill, injection.NewPodKillInjector(k8sClient))
			registry.Register(v1alpha1.CRDMutation, injection.NewCRDMutationInjector(k8sClient))
			registry.Register(v1alpha1.ConfigDrift, injection.NewConfigDriftInjector(k8sClient))
			registry.Register(v1alpha1.NetworkPartition, injection.NewNetworkPartitionInjector(k8sClient))

			// Register Phase 2 injectors (require K8s client)
			if k8sClient != nil {
				registry.Register(v1alpha1.WebhookDisrupt, injection.NewWebhookDisruptInjector(k8sClient))
				registry.Register(v1alpha1.RBACRevoke, injection.NewRBACRevokeInjector(k8sClient))
				registry.Register(v1alpha1.FinalizerBlock, injection.NewFinalizerBlockInjector(k8sClient))
			}

			// Build orchestrator
			verbose, _ := cmd.Flags().GetBool("verbose")
			maxCycles := 10
			if knowledge != nil {
				maxCycles = knowledge.Recovery.MaxReconcileCycles
			}

			// Build experiment lock: distributed (Lease-based) or local (in-process)
			var lock safety.ExperimentLock
			if distributedLock && k8sClient != nil {
				lock = safety.NewLeaseExperimentLock(k8sClient, lockNamespace)
			} else {
				lock = safety.NewLocalExperimentLock()
			}

			orch := orchestrator.New(orchestrator.OrchestratorConfig{
				Registry:   registry,
				Observer:   observer.NewKubernetesObserver(k8sClient),
				Reconciler: observer.NewReconciliationChecker(k8sClient),
				Evaluator:  evaluator.New(maxCycles),
				Lock:       lock,
				Knowledge:  knowledge,
				ReportDir:  reportDir,
				Verbose:    verbose,
			})

			// Run
			result, err := orch.Run(ctx, exp)
			if err != nil {
				return fmt.Errorf("experiment failed: %w", err)
			}

			// Print summary
			fmt.Printf("\nExperiment: %s\n", result.Experiment)
			fmt.Printf("Verdict:    %s\n", result.Verdict)
			if result.Evaluation != nil {
				fmt.Printf("Confidence: %s\n", result.Evaluation.Confidence)
				if len(result.Evaluation.Deviations) > 0 {
					fmt.Println("Deviations:")
					for _, d := range result.Evaluation.Deviations {
						fmt.Printf("  - [%s] %s\n", d.Type, d.Detail)
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&knowledgePath, "knowledge", "", "path to operator knowledge YAML")
	cmd.Flags().StringVar(&reportDir, "report-dir", "", "directory for report output")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate without injecting")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "total experiment timeout")
	cmd.Flags().BoolVar(&distributedLock, "distributed-lock", false, "use Kubernetes Lease-based distributed locking")
	cmd.Flags().StringVar(&lockNamespace, "lock-namespace", "opendatahub", "namespace for distributed lock leases")

	return cmd
}
