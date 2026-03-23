package cli

import (
	"fmt"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/internal/controller"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/clock"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/evaluator"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/injection"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/observer"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/orchestrator"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func newControllerCommand() *cobra.Command {
	var (
		namespace    string
		metricsAddr  string
		healthAddr   string
		leaderElect  bool
		knowledgeDir string
	)

	cmd := &cobra.Command{
		Use:   "controller",
		Short: "Controller mode commands",
	}

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the ChaosExperiment controller",
		Long:  "Starts a Kubernetes controller that watches ChaosExperiment CRs and drives them through the experiment lifecycle.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if namespace == "" {
				return fmt.Errorf("--namespace is required")
			}
			return startController(namespace, metricsAddr, healthAddr, leaderElect, knowledgeDir)
		},
	}

	startCmd.Flags().StringVar(&namespace, "namespace", "", "namespace to watch (required)")
	startCmd.Flags().StringVar(&metricsAddr, "metrics-addr", ":8080", "metrics bind address")
	startCmd.Flags().StringVar(&healthAddr, "health-addr", ":8081", "health probe bind address")
	startCmd.Flags().BoolVar(&leaderElect, "leader-elect", true, "enable leader election")
	startCmd.Flags().StringVar(&knowledgeDir, "knowledge-dir", "", "directory of operator knowledge YAMLs")
	_ = startCmd.MarkFlagRequired("namespace")

	cmd.AddCommand(startCmd)
	return cmd
}

func startController(namespace, metricsAddr, healthAddr string, leaderElect bool, knowledgeDir string) error {
	ctrl.SetLogger(zap.New(zap.UseDevMode(false)))

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("adding scheme: %w", err)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				namespace: {},
			},
		},
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress:  healthAddr,
		LeaderElection:          leaderElect,
		LeaderElectionID:        "odh-chaos-controller",
		LeaderElectionNamespace: namespace,
	})
	if err != nil {
		return fmt.Errorf("creating manager: %w", err)
	}

	k8sClient := mgr.GetClient()

	// Load knowledge (optional)
	var knowledge *model.OperatorKnowledge
	var allModels []*model.OperatorKnowledge
	var depGraph *model.DependencyGraph

	if knowledgeDir != "" {
		allModels, err = model.LoadKnowledgeDir(knowledgeDir)
		if err != nil {
			return fmt.Errorf("loading knowledge dir %s: %w", knowledgeDir, err)
		}
		if len(allModels) > 0 {
			knowledge = allModels[0]
		}
		if len(allModels) > 1 {
			depGraph, err = model.BuildDependencyGraph(allModels)
			if err != nil {
				return fmt.Errorf("building dependency graph: %w", err)
			}
		}
	}

	// Build registry
	registry := injection.NewRegistry()
	registry.Register(v1alpha1.PodKill, injection.NewPodKillInjector(k8sClient))
	registry.Register(v1alpha1.CRDMutation, injection.NewCRDMutationInjector(k8sClient))
	registry.Register(v1alpha1.ConfigDrift, injection.NewConfigDriftInjector(k8sClient))
	registry.Register(v1alpha1.NetworkPartition, injection.NewNetworkPartitionInjector(k8sClient))
	registry.Register(v1alpha1.WebhookDisrupt, injection.NewWebhookDisruptInjector(k8sClient))
	registry.Register(v1alpha1.RBACRevoke, injection.NewRBACRevokeInjector(k8sClient))
	registry.Register(v1alpha1.FinalizerBlock, injection.NewFinalizerBlockInjector(k8sClient))
	registry.Register(v1alpha1.ClientFault, injection.NewClientFaultInjector(k8sClient))

	maxCycles := 10
	if knowledge != nil {
		maxCycles = knowledge.Recovery.MaxReconcileCycles
	}

	lock := safety.NewLeaseExperimentLock(k8sClient, namespace)

	orch := orchestrator.New(orchestrator.OrchestratorConfig{
		Registry:   registry,
		Observer:   observer.NewKubernetesObserver(k8sClient),
		Reconciler: observer.NewReconciliationChecker(k8sClient),
		Evaluator:  evaluator.New(maxCycles),
		Lock:       lock,
		Knowledge:  knowledge,
		K8sClient:  k8sClient,
		DepGraph:   depGraph,
	})

	reconciler := &controller.ChaosExperimentReconciler{
		Client:       k8sClient,
		Scheme:       mgr.GetScheme(),
		Orchestrator: orch,
		Lock:         lock,
		Clock:        clock.RealClock{},
		Recorder:     mgr.GetEventRecorderFor("chaosexperiment-controller"), //nolint:staticcheck // TODO: migrate to events.EventRecorder interface
	}

	if err := reconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setting up controller: %w", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("adding healthz check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("adding readyz check: %w", err)
	}

	ctrl.Log.Info("starting controller", "namespace", namespace)
	return mgr.Start(ctrl.SetupSignalHandler())
}
