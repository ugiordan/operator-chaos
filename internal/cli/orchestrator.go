package cli

import (
	"fmt"
	"os"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/evaluator"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/injection"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/observer"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/orchestrator"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// orchestratorDeps holds the components built by buildOrchestrator.
type orchestratorDeps struct {
	Orchestrator *orchestrator.Orchestrator
	Knowledge    *model.OperatorKnowledge
	K8sClient    client.Client
}

// buildOrchestrator creates an Orchestrator and all its dependencies.
// It handles knowledge loading, K8s client creation, injection registry setup,
// observer, evaluator, and lock initialization.
func buildOrchestrator(knowledgePath string, dryRun bool, reportDir string, distributedLock bool, lockNamespace string, verbose bool) (*orchestratorDeps, error) {
	// Load knowledge (optional)
	var knowledge *model.OperatorKnowledge
	if knowledgePath != "" {
		var err error
		knowledge, err = model.LoadKnowledge(knowledgePath)
		if err != nil {
			return nil, fmt.Errorf("loading knowledge: %w", err)
		}
	}

	// Create report dir if specified
	if reportDir != "" {
		if err := os.MkdirAll(reportDir, 0750); err != nil {
			return nil, fmt.Errorf("creating report directory: %w", err)
		}
	}

	// Build K8s client
	var k8sClient client.Client
	if !dryRun {
		cfg, err := config.GetConfig()
		if err != nil {
			return nil, fmt.Errorf("getting kubeconfig: %w", err)
		}
		k8sClient, err = client.New(cfg, client.Options{})
		if err != nil {
			return nil, fmt.Errorf("creating k8s client: %w", err)
		}
	}

	// Build injection registry
	registry := injection.NewRegistry()

	// All injectors require a valid K8s client
	if k8sClient != nil {
		registry.Register(v1alpha1.PodKill, injection.NewPodKillInjector(k8sClient))
		registry.Register(v1alpha1.CRDMutation, injection.NewCRDMutationInjector(k8sClient))
		registry.Register(v1alpha1.ConfigDrift, injection.NewConfigDriftInjector(k8sClient))
		registry.Register(v1alpha1.NetworkPartition, injection.NewNetworkPartitionInjector(k8sClient))
		registry.Register(v1alpha1.WebhookDisrupt, injection.NewWebhookDisruptInjector(k8sClient))
		registry.Register(v1alpha1.RBACRevoke, injection.NewRBACRevokeInjector(k8sClient))
		registry.Register(v1alpha1.FinalizerBlock, injection.NewFinalizerBlockInjector(k8sClient))
		registry.Register(v1alpha1.ClientFault, injection.NewClientFaultInjector(k8sClient))
	}

	// Build orchestrator config
	maxCycles := 10
	if knowledge != nil {
		maxCycles = knowledge.Recovery.MaxReconcileCycles
	}

	// Build experiment lock
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
		K8sClient:  k8sClient,
		ReportDir:  reportDir,
		Verbose:    verbose,
	})

	return &orchestratorDeps{
		Orchestrator: orch,
		Knowledge:    knowledge,
		K8sClient:    k8sClient,
	}, nil
}

// printExperimentResult prints a human-readable summary of an experiment result.
func printExperimentResult(result *orchestrator.ExperimentResult) {
	fmt.Printf("\nExperiment: %s\n", result.Experiment)
	fmt.Printf("Verdict:    %s\n", colorVerdict(string(result.Verdict)))
	if result.Evaluation != nil {
		fmt.Printf("Confidence: %s\n", result.Evaluation.Confidence)
		if len(result.Evaluation.Deviations) > 0 {
			fmt.Println("Deviations:")
			for _, d := range result.Evaluation.Deviations {
				fmt.Printf("  - [%s] %s\n", d.Type, d.Detail)
			}
		}
	}
	if result.CleanupError != "" {
		fmt.Fprintf(os.Stderr, "  Cleanup Error: %s\n", result.CleanupError)
	}
}
