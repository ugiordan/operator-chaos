package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	"github.com/spf13/cobra"
)

// cleanSummary tracks what was cleaned per artifact type.
type cleanSummary struct {
	NetworkPolicies   int
	Leases            int
	ClusterRoles      int
	RoleBindings      int
	TTLExpired        int
	WebhooksRestored  int
	RBACBindingsFixed int
}

func (s cleanSummary) total() int {
	return s.NetworkPolicies + s.Leases + s.ClusterRoles + s.RoleBindings +
		s.TTLExpired + s.WebhooksRestored + s.RBACBindingsFixed
}

func (s cleanSummary) print() {
	if s.total() == 0 {
		fmt.Println("No chaos artifacts found.")
		return
	}
	fmt.Println("\n--- Clean Summary ---")
	if s.NetworkPolicies > 0 {
		fmt.Printf("  NetworkPolicies removed:    %d\n", s.NetworkPolicies)
	}
	if s.Leases > 0 {
		fmt.Printf("  Leases removed:             %d\n", s.Leases)
	}
	if s.ClusterRoles > 0 {
		fmt.Printf("  ClusterRoles removed:       %d\n", s.ClusterRoles)
	}
	if s.RoleBindings > 0 {
		fmt.Printf("  RoleBindings removed:       %d\n", s.RoleBindings)
	}
	if s.TTLExpired > 0 {
		fmt.Printf("  TTL-expired removed:        %d\n", s.TTLExpired)
	}
	if s.WebhooksRestored > 0 {
		fmt.Printf("  Webhooks restored:          %d\n", s.WebhooksRestored)
	}
	if s.RBACBindingsFixed > 0 {
		fmt.Printf("  RBAC bindings restored:     %d\n", s.RBACBindingsFixed)
	}
	fmt.Printf("  Total cleaned:              %d\n", s.total())
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

	// 6. Restore chaos-modified ValidatingWebhookConfigurations
	summary.WebhooksRestored = cleanWebhookConfigurations(ctx, k8sClient)

	// 7. Restore chaos-modified RBAC bindings (ClusterRoleBindings + RoleBindings)
	summary.RBACBindingsFixed = cleanRBACBindings(ctx, k8sClient, namespace)

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

// cleanWebhookConfigurations finds ValidatingWebhookConfigurations that have a
// rollback annotation (set by the WebhookDisrupt injector), parses the original
// failure policies from the annotation, restores them, and removes chaos metadata.
func cleanWebhookConfigurations(ctx context.Context, k8sClient client.Client) int {
	webhooks := &admissionregv1.ValidatingWebhookConfigurationList{}
	if err := k8sClient.List(ctx, webhooks); err != nil {
		fmt.Printf("Warning: listing ValidatingWebhookConfigurations: %v\n", err)
		return 0
	}

	restored := 0
	for i := range webhooks.Items {
		wc := &webhooks.Items[i]
		annotations := wc.GetAnnotations()
		if annotations == nil {
			continue
		}
		rollbackJSON, ok := annotations[safety.RollbackAnnotationKey]
		if !ok {
			continue
		}

		// Parse the original failure policies map
		var originalPolicies map[string]string
		if err := json.Unmarshal([]byte(rollbackJSON), &originalPolicies); err != nil {
			fmt.Printf("Warning: parsing rollback data for ValidatingWebhookConfiguration %q: %v\n", wc.Name, err)
			continue
		}

		// Restore original failure policies
		for j, wh := range wc.Webhooks {
			if policyStr, found := originalPolicies[wh.Name]; found {
				if policyStr == "" {
					wc.Webhooks[j].FailurePolicy = nil
				} else {
					p := admissionregv1.FailurePolicyType(policyStr)
					wc.Webhooks[j].FailurePolicy = &p
				}
			}
		}

		// Remove rollback annotation
		delete(wc.Annotations, safety.RollbackAnnotationKey)

		// Remove chaos labels
		for k := range safety.ChaosLabels(string(v1alpha1.WebhookDisrupt)) {
			delete(wc.Labels, k)
		}

		fmt.Printf("Restoring ValidatingWebhookConfiguration %q\n", wc.Name)
		if err := k8sClient.Update(ctx, wc); err != nil {
			fmt.Printf("  Warning: %v\n", err)
		} else {
			restored++
		}
	}
	return restored
}

// cleanRBACBindings finds ClusterRoleBindings and RoleBindings that have a
// rollback annotation (set by the RBACRevoke injector), parses the original
// subjects from the annotation, restores them, and removes chaos metadata.
func cleanRBACBindings(ctx context.Context, k8sClient client.Client, namespace string) int {
	restored := 0

	// ClusterRoleBindings (cluster-scoped)
	crbs := &rbacv1.ClusterRoleBindingList{}
	if err := k8sClient.List(ctx, crbs); err != nil {
		fmt.Printf("Warning: listing ClusterRoleBindings: %v\n", err)
	} else {
		for i := range crbs.Items {
			crb := &crbs.Items[i]
			annotations := crb.GetAnnotations()
			if annotations == nil {
				continue
			}
			rollbackJSON, ok := annotations[safety.RollbackAnnotationKey]
			if !ok {
				continue
			}

			var originalSubjects []rbacv1.Subject
			if err := json.Unmarshal([]byte(rollbackJSON), &originalSubjects); err != nil {
				fmt.Printf("Warning: parsing rollback data for ClusterRoleBinding %q: %v\n", crb.Name, err)
				continue
			}

			crb.Subjects = originalSubjects

			// Remove rollback annotation
			delete(crb.Annotations, safety.RollbackAnnotationKey)

			// Remove chaos labels
			for k := range safety.ChaosLabels(string(v1alpha1.RBACRevoke)) {
				delete(crb.Labels, k)
			}

			fmt.Printf("Restoring ClusterRoleBinding %q\n", crb.Name)
			if err := k8sClient.Update(ctx, crb); err != nil {
				fmt.Printf("  Warning: %v\n", err)
			} else {
				restored++
			}
		}
	}

	// RoleBindings (namespace-scoped)
	rbs := &rbacv1.RoleBindingList{}
	listOpts := []client.ListOption{}
	if namespace != "" {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}
	if err := k8sClient.List(ctx, rbs, listOpts...); err != nil {
		fmt.Printf("Warning: listing RoleBindings: %v\n", err)
	} else {
		for i := range rbs.Items {
			rb := &rbs.Items[i]
			annotations := rb.GetAnnotations()
			if annotations == nil {
				continue
			}
			rollbackJSON, ok := annotations[safety.RollbackAnnotationKey]
			if !ok {
				continue
			}

			var originalSubjects []rbacv1.Subject
			if err := json.Unmarshal([]byte(rollbackJSON), &originalSubjects); err != nil {
				fmt.Printf("Warning: parsing rollback data for RoleBinding %s/%s: %v\n", rb.Namespace, rb.Name, err)
				continue
			}

			rb.Subjects = originalSubjects

			// Remove rollback annotation
			delete(rb.Annotations, safety.RollbackAnnotationKey)

			// Remove chaos labels
			for k := range safety.ChaosLabels(string(v1alpha1.RBACRevoke)) {
				delete(rb.Labels, k)
			}

			fmt.Printf("Restoring RoleBinding %s/%s\n", rb.Namespace, rb.Name)
			if err := k8sClient.Update(ctx, rb); err != nil {
				fmt.Printf("  Warning: %v\n", err)
			} else {
				restored++
			}
		}
	}

	return restored
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
