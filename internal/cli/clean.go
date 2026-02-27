package cli

import (
	"context"
	"fmt"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	"github.com/spf13/cobra"
)

// cleanSummary tracks what was cleaned per artifact type.
type cleanSummary struct {
	NetworkPolicies int
	Leases          int
	ClusterRoles    int
	RoleBindings    int
	TTLExpired      int
}

func (s cleanSummary) total() int {
	return s.NetworkPolicies + s.Leases + s.ClusterRoles + s.RoleBindings + s.TTLExpired
}

func (s cleanSummary) print() {
	if s.total() == 0 {
		fmt.Println("No chaos artifacts found.")
		return
	}
	fmt.Println("\n--- Clean Summary ---")
	if s.NetworkPolicies > 0 {
		fmt.Printf("  NetworkPolicies removed: %d\n", s.NetworkPolicies)
	}
	if s.Leases > 0 {
		fmt.Printf("  Leases removed:          %d\n", s.Leases)
	}
	if s.ClusterRoles > 0 {
		fmt.Printf("  ClusterRoles removed:    %d\n", s.ClusterRoles)
	}
	if s.RoleBindings > 0 {
		fmt.Printf("  RoleBindings removed:    %d\n", s.RoleBindings)
	}
	if s.TTLExpired > 0 {
		fmt.Printf("  TTL-expired removed:     %d\n", s.TTLExpired)
	}
	fmt.Printf("  Total cleaned:           %d\n", s.total())
}

func newCleanCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "clean",
		Short: "Remove all chaos artifacts from the cluster (emergency stop)",
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString("namespace")

			cfg, err := config.GetConfig()
			if err != nil {
				return fmt.Errorf("getting kubeconfig: %w", err)
			}

			k8sClient, err := client.New(cfg, client.Options{})
			if err != nil {
				return fmt.Errorf("creating k8s client: %w", err)
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
			defer cancel()

			summary := runClean(ctx, k8sClient, namespace)
			summary.print()
			return nil
		},
	}
}

// runClean performs the actual cleanup and returns a summary. It is extracted
// as a function so that nil-client scenarios can be handled gracefully and
// the logic can be tested independently of cobra.
func runClean(ctx context.Context, k8sClient client.Client, namespace string) cleanSummary {
	var summary cleanSummary
	if k8sClient == nil {
		fmt.Println("Warning: no Kubernetes client available, skipping cleanup")
		return summary
	}

	chaosLabels := client.MatchingLabels{safety.ManagedByLabel: safety.ManagedByValue}

	// 1. Clean NetworkPolicies with chaos label
	summary.NetworkPolicies = cleanNetworkPolicies(ctx, k8sClient, namespace, chaosLabels)

	// 2. Clean Leases (distributed experiment locks)
	summary.Leases = cleanLeases(ctx, k8sClient, namespace, chaosLabels)

	// 3. Clean ClusterRoles with chaos label (for RBACRevoke injector)
	summary.ClusterRoles = cleanClusterRoles(ctx, k8sClient, chaosLabels)

	// 4. Clean RoleBindings with chaos label
	summary.RoleBindings = cleanRoleBindings(ctx, k8sClient, namespace, chaosLabels)

	// 5. Scan for TTL-expired NetworkPolicies (belt-and-suspenders)
	summary.TTLExpired = cleanTTLExpired(ctx, k8sClient, namespace)

	return summary
}

func cleanNetworkPolicies(ctx context.Context, k8sClient client.Client, namespace string, labels client.MatchingLabels) int {
	policies := &networkingv1.NetworkPolicyList{}
	if err := k8sClient.List(ctx, policies,
		client.InNamespace(namespace),
		labels,
	); err != nil {
		fmt.Printf("Warning: listing chaos NetworkPolicies: %v\n", err)
		return 0
	}

	cleaned := 0
	for i := range policies.Items {
		fmt.Printf("Deleting NetworkPolicy %s/%s\n", policies.Items[i].Namespace, policies.Items[i].Name)
		if err := k8sClient.Delete(ctx, &policies.Items[i]); err != nil {
			fmt.Printf("  Warning: %v\n", err)
		} else {
			cleaned++
		}
	}
	return cleaned
}

func cleanLeases(ctx context.Context, k8sClient client.Client, namespace string, labels client.MatchingLabels) int {
	leases := &coordinationv1.LeaseList{}
	if err := k8sClient.List(ctx, leases,
		client.InNamespace(namespace),
		labels,
	); err != nil {
		fmt.Printf("Warning: listing chaos Leases: %v\n", err)
		return 0
	}

	cleaned := 0
	for i := range leases.Items {
		fmt.Printf("Deleting Lease %s/%s\n", leases.Items[i].Namespace, leases.Items[i].Name)
		if err := k8sClient.Delete(ctx, &leases.Items[i]); err != nil {
			fmt.Printf("  Warning: %v\n", err)
		} else {
			cleaned++
		}
	}
	return cleaned
}

func cleanClusterRoles(ctx context.Context, k8sClient client.Client, labels client.MatchingLabels) int {
	roles := &rbacv1.ClusterRoleList{}
	if err := k8sClient.List(ctx, roles, labels); err != nil {
		fmt.Printf("Warning: listing chaos ClusterRoles: %v\n", err)
		return 0
	}

	cleaned := 0
	for i := range roles.Items {
		fmt.Printf("Deleting ClusterRole %s\n", roles.Items[i].Name)
		if err := k8sClient.Delete(ctx, &roles.Items[i]); err != nil {
			fmt.Printf("  Warning: %v\n", err)
		} else {
			cleaned++
		}
	}
	return cleaned
}

func cleanRoleBindings(ctx context.Context, k8sClient client.Client, namespace string, labels client.MatchingLabels) int {
	bindings := &rbacv1.RoleBindingList{}
	if err := k8sClient.List(ctx, bindings,
		client.InNamespace(namespace),
		labels,
	); err != nil {
		fmt.Printf("Warning: listing chaos RoleBindings: %v\n", err)
		return 0
	}

	cleaned := 0
	for i := range bindings.Items {
		fmt.Printf("Deleting RoleBinding %s/%s\n", bindings.Items[i].Namespace, bindings.Items[i].Name)
		if err := k8sClient.Delete(ctx, &bindings.Items[i]); err != nil {
			fmt.Printf("  Warning: %v\n", err)
		} else {
			cleaned++
		}
	}
	return cleaned
}

// cleanTTLExpired scans all NetworkPolicies in the namespace for those with
// a TTL annotation that has expired, regardless of whether they have the
// managed-by label. This acts as a belt-and-suspenders safety net.
func cleanTTLExpired(ctx context.Context, k8sClient client.Client, namespace string) int {
	policies := &networkingv1.NetworkPolicyList{}
	if err := k8sClient.List(ctx, policies,
		client.InNamespace(namespace),
	); err != nil {
		fmt.Printf("Warning: listing NetworkPolicies for TTL scan: %v\n", err)
		return 0
	}

	cleaned := 0
	for i := range policies.Items {
		annotations := policies.Items[i].GetAnnotations()
		if annotations == nil {
			continue
		}
		expiryStr, ok := annotations[safety.TTLAnnotationKey]
		if !ok {
			continue
		}
		if safety.IsExpired(expiryStr) {
			fmt.Printf("Deleting TTL-expired NetworkPolicy %s/%s (expired: %s)\n",
				policies.Items[i].Namespace, policies.Items[i].Name, expiryStr)
			if err := k8sClient.Delete(ctx, &policies.Items[i]); err != nil {
				fmt.Printf("  Warning: %v\n", err)
			} else {
				cleaned++
			}
		}
	}
	return cleaned
}
