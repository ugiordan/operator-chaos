package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	v1alpha1 "github.com/opendatahub-io/operator-chaos/api/v1alpha1"
	"github.com/opendatahub-io/operator-chaos/pkg/model"
	"github.com/spf13/cobra"
)

// resourceStatus represents the cluster check result for a single resource.
type resourceStatus struct {
	Component string
	Name      string
	Kind      string
	Status    string // "Found", "Missing", "Degraded", "Error"
	Error     string // non-empty when Status is "Error"
}

func newPreflightCommand() *cobra.Command {
	var knowledgePath string
	var local bool

	cmd := &cobra.Command{
		Use:   "preflight",
		Short: "Check cluster readiness before running chaos experiments",
		Long: `Preflight verifies that all resources declared in an operator knowledge
file exist and are healthy on the cluster. Use --local to validate the
knowledge file structure without connecting to a cluster.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString("namespace")
			verbose, _ := cmd.Flags().GetBool("verbose")

			// 1. Load and validate knowledge
			knowledge, err := model.LoadKnowledge(knowledgePath)
			if err != nil {
				return fmt.Errorf("loading knowledge: %w", err)
			}

			errs := model.ValidateKnowledge(knowledge)
			if len(errs) > 0 {
				fmt.Fprintln(os.Stderr, "Knowledge validation FAILED:")
				for _, e := range errs {
					fmt.Fprintf(os.Stderr, "  - %s\n", e)
				}
				return fmt.Errorf("%d validation errors", len(errs))
			}

			// 2. Cross-reference consistency checks
			crossRefErrs := crossReferenceChecks(knowledge)
			if len(crossRefErrs) > 0 {
				fmt.Fprintln(os.Stderr, "Cross-reference checks FAILED:")
				for _, e := range crossRefErrs {
					fmt.Fprintf(os.Stderr, "  - %s\n", e)
				}
				return fmt.Errorf("%d cross-reference errors", len(crossRefErrs))
			}

			// 3. Print summary
			printKnowledgeSummary(knowledge, verbose)

			if local {
				fmt.Println("\nLocal preflight passed.")
				return nil
			}

			// 4. Cluster mode: connect and check resources
			cfg, err := config.GetConfig()
			if err != nil {
				return fmt.Errorf("getting kubeconfig: %w", err)
			}

			k8sClient, err := client.New(cfg, client.Options{})
			if err != nil {
				return fmt.Errorf("creating k8s client: %w", err)
			}

			results := checkClusterResources(cmd.Context(), k8sClient, knowledge, namespace)

			printResourceTable(results)

			// Check for critical failures
			missing := 0
			degraded := 0
			errored := 0
			for _, r := range results {
				switch r.Status {
				case "Missing":
					missing++
				case "Degraded":
					degraded++
				case "Error":
					errored++
				}
			}

			if errored > 0 {
				fmt.Fprintf(os.Stderr, "\nWarning: %d resources could not be checked (RBAC/connectivity issues)\n", errored)
			}
			if degraded > 0 {
				fmt.Fprintf(os.Stderr, "\nWarning: %d resources are degraded\n", degraded)
			}
			if missing > 0 || errored > 0 {
				return fmt.Errorf("%d resources missing, %d resources could not be checked", missing, errored)
			}

			fmt.Println("\nCluster preflight passed.")
			return nil
		},
	}

	cmd.Flags().StringVar(&knowledgePath, "knowledge", "", "path to operator knowledge YAML (required)")
	_ = cmd.MarkFlagRequired("knowledge")
	cmd.Flags().BoolVar(&local, "local", false, "skip cluster checks, only validate knowledge file")

	return cmd
}

// crossReferenceChecks validates internal consistency of the knowledge file
// beyond basic field validation (which is already handled by model.ValidateKnowledge).
func crossReferenceChecks(k *model.OperatorKnowledge) []string {
	var errs []string

	for i, comp := range k.Components {
		// Build a set of managed resource names for this component
		resourceNames := make(map[string]bool)
		for _, mr := range comp.ManagedResources {
			if mr.Name != "" {
				resourceNames[mr.Name] = true
			}
		}

		// Check steadyState references: each check that references a resource
		// by name should match a declared managedResource
		for j, check := range comp.SteadyState.Checks {
			if check.Name != "" && !resourceNames[check.Name] {
				errs = append(errs, fmt.Sprintf(
					"components[%d].steadyState.checks[%d]: references resource %q not declared in managedResources",
					i, j, check.Name))
			}
		}
	}

	return errs
}

// printKnowledgeSummary prints a human-readable summary of the knowledge file contents.
func printKnowledgeSummary(k *model.OperatorKnowledge, verbose bool) {
	fmt.Printf("Operator:    %s\n", k.Operator.Name)
	fmt.Printf("Namespace:   %s\n", k.Operator.Namespace)
	fmt.Printf("Components:  %d\n", len(k.Components))

	totalResources := 0
	totalWebhooks := 0
	totalFinalizers := 0
	for _, comp := range k.Components {
		totalResources += len(comp.ManagedResources)
		totalWebhooks += len(comp.Webhooks)
		totalFinalizers += len(comp.Finalizers)
	}

	fmt.Printf("Resources:   %d\n", totalResources)
	fmt.Printf("Webhooks:    %d\n", totalWebhooks)
	fmt.Printf("Finalizers:  %d\n", totalFinalizers)

	if verbose {
		for _, comp := range k.Components {
			fmt.Printf("\n  Component: %s (controller: %s)\n", comp.Name, comp.Controller)
			for _, mr := range comp.ManagedResources {
				fmt.Printf("    - %s/%s %s\n", mr.APIVersion, mr.Kind, mr.Name)
			}
			for _, wh := range comp.Webhooks {
				fmt.Printf("    - webhook: %s (%s)\n", wh.Name, wh.Type)
			}
			for _, f := range comp.Finalizers {
				fmt.Printf("    - finalizer: %s\n", f)
			}
		}
	}
}

// clusterScopedKinds lists Kubernetes resource kinds that are cluster-scoped
// (no namespace). This prevents injecting a namespace for resources that
// should be looked up without one.
var clusterScopedKinds = map[string]bool{
	"ClusterRole":                      true,
	"ClusterRoleBinding":               true,
	"CustomResourceDefinition":         true,
	"ValidatingWebhookConfiguration":   true,
	"MutatingWebhookConfiguration":     true,
	"Namespace":                        true,
	"PersistentVolume":                 true,
	"StorageClass":                     true,
	"IngressClass":                     true,
	"PriorityClass":                    true,
	"APIService":                       true,
	"Node":                             true,
}

// checkClusterResources checks each managed resource on the cluster and returns
// a slice of resourceStatus results.
func checkClusterResources(ctx context.Context, k8sClient client.Client, k *model.OperatorKnowledge, namespace string) []resourceStatus {
	var results []resourceStatus

	for _, comp := range k.Components {
		for _, mr := range comp.ManagedResources {
			var ns string
			if clusterScopedKinds[mr.Kind] {
				ns = ""
			} else if namespace != "" {
				ns = namespace
			} else if mr.Namespace != "" {
				ns = mr.Namespace
			} else {
				ns = v1alpha1.DefaultNamespace
			}

			status, errMsg := checkSingleResource(ctx, k8sClient, mr, ns)
			results = append(results, resourceStatus{
				Component: comp.Name,
				Name:      mr.Name,
				Kind:      mr.Kind,
				Status:    status,
				Error:     errMsg,
			})
		}
	}

	return results
}

// checkSingleResource checks whether a single managed resource exists and is healthy.
// Returns a status string and an optional error message for the "Error" status.
func checkSingleResource(ctx context.Context, k8sClient client.Client, mr model.ManagedResource, namespace string) (string, string) {
	// Special handling for Deployments: check health condition
	if mr.Kind == "Deployment" && mr.APIVersion == "apps/v1" {
		deploy := &appsv1.Deployment{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: mr.Name, Namespace: namespace}, deploy)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return "Missing", ""
			}
			return "Error", err.Error()
		}
		// Check for Available condition
		for _, cond := range deploy.Status.Conditions {
			if cond.Type == appsv1.DeploymentAvailable {
				if cond.Status == "True" {
					return "Found", ""
				}
				return "Degraded", ""
			}
		}
		// No Available condition found - treat as degraded
		return "Degraded", ""
	}

	// Generic resource check using unstructured
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.FromAPIVersionAndKind(mr.APIVersion, mr.Kind))
	err := k8sClient.Get(ctx, client.ObjectKey{Name: mr.Name, Namespace: namespace}, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "Missing", ""
		}
		return "Error", err.Error()
	}

	return "Found", ""
}

// printResourceTable prints a formatted table of resource check results.
func printResourceTable(results []resourceStatus) {
	fmt.Println("\n--- Cluster Resource Check ---")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "  COMPONENT\tNAME\tKIND\tSTATUS\n")
	_, _ = fmt.Fprintf(w, "  ---------\t----\t----\t------\n")
	for _, r := range results {
		status := r.Status
		if r.Error != "" {
			status = fmt.Sprintf("%s (%s)", r.Status, r.Error)
		}
		_, _ = fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", r.Component, r.Name, r.Kind, status)
	}
	_ = w.Flush()
}
