package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	"github.com/spf13/cobra"
)

const defaultCleanTimeout = 60 * time.Second
const defaultWatchInterval = 60 * time.Second

// cleanSummary tracks what was cleaned per artifact type.
type cleanSummary struct {
	NetworkPolicies   int
	Leases            int
	ClusterRoles      int
	RoleBindings      int
	TTLExpired        int
	WebhooksRestored  int
	RBACBindingsFixed int
	FinalizersRemoved int
	ConfigDriftsFixed int
	CRDMutationsFixed int
	ResultConfigMaps  int
}

func (s cleanSummary) total() int {
	return s.NetworkPolicies + s.Leases + s.ClusterRoles + s.RoleBindings +
		s.TTLExpired + s.WebhooksRestored + s.RBACBindingsFixed +
		s.FinalizersRemoved + s.ConfigDriftsFixed + s.CRDMutationsFixed +
		s.ResultConfigMaps
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
	if s.FinalizersRemoved > 0 {
		fmt.Printf("  Finalizers removed:         %d\n", s.FinalizersRemoved)
	}
	if s.ConfigDriftsFixed > 0 {
		fmt.Printf("  Config drifts restored:     %d\n", s.ConfigDriftsFixed)
	}
	if s.CRDMutationsFixed > 0 {
		fmt.Printf("  CRD mutations restored:     %d\n", s.CRDMutationsFixed)
	}
	if s.ResultConfigMaps > 0 {
		fmt.Printf("  Result ConfigMaps removed:  %d\n", s.ResultConfigMaps)
	}
	fmt.Printf("  Total cleaned:              %d\n", s.total())
}

func newCleanCommand() *cobra.Command {
	cmd := &cobra.Command{
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

			watch, _ := cmd.Flags().GetBool("watch")
			if !watch {
				ctx, cancel := context.WithTimeout(cmd.Context(), defaultCleanTimeout)
				defer cancel()

				summary := runClean(ctx, k8sClient, namespace)
				summary.print()
				return nil
			}

			interval, _ := cmd.Flags().GetDuration("interval")
			return runCleanWatch(cmd.Context(), k8sClient, namespace, interval)
		},
	}

	cmd.Flags().Bool("watch", false, "continuously scan and clean chaos artifacts")
	cmd.Flags().Duration("interval", defaultWatchInterval, "scan interval when --watch is set")

	return cmd
}

// cleanSummaryDiff logs the per-type difference between two scan summaries.
func cleanSummaryDiff(prev, curr cleanSummary) {
	diff := curr.total() - prev.total()
	if diff == 0 {
		fmt.Fprintf(os.Stderr, "  Delta: no change since last scan\n")
		return
	}
	if diff > 0 {
		fmt.Fprintf(os.Stderr, "  Delta: +%d artifacts since last scan\n", diff)
	} else {
		fmt.Fprintf(os.Stderr, "  Delta: %d artifacts since last scan\n", diff)
	}
	// Per-type breakdown
	type entry struct {
		name string
		diff int
	}
	entries := []entry{
		{"NetworkPolicies", curr.NetworkPolicies - prev.NetworkPolicies},
		{"Leases", curr.Leases - prev.Leases},
		{"ClusterRoles", curr.ClusterRoles - prev.ClusterRoles},
		{"RoleBindings", curr.RoleBindings - prev.RoleBindings},
		{"TTLExpired", curr.TTLExpired - prev.TTLExpired},
		{"WebhooksRestored", curr.WebhooksRestored - prev.WebhooksRestored},
		{"RBACBindingsFixed", curr.RBACBindingsFixed - prev.RBACBindingsFixed},
		{"FinalizersRemoved", curr.FinalizersRemoved - prev.FinalizersRemoved},
		{"ConfigDriftsFixed", curr.ConfigDriftsFixed - prev.ConfigDriftsFixed},
		{"CRDMutationsFixed", curr.CRDMutationsFixed - prev.CRDMutationsFixed},
		{"ResultConfigMaps", curr.ResultConfigMaps - prev.ResultConfigMaps},
	}
	for _, e := range entries {
		if e.diff != 0 {
			fmt.Fprintf(os.Stderr, "    %s: %+d\n", e.name, e.diff)
		}
	}
}

// runCleanWatch runs the clean loop on a ticker until the context is cancelled
// or a SIGINT/SIGTERM is received.
func runCleanWatch(ctx context.Context, k8sClient client.Client, namespace string, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("--interval must be positive, got %v", interval)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var prev cleanSummary
	scan := 0

	for {
		scan++
		fmt.Fprintf(os.Stderr, "\n=== Watch scan #%d ===\n", scan)

		scanCtx, scanCancel := context.WithTimeout(ctx, defaultCleanTimeout)
		summary := runClean(scanCtx, k8sClient, namespace)
		scanCancel()

		summary.print()
		if scan > 1 {
			cleanSummaryDiff(prev, summary)
		}
		prev = summary

		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "\nWatch stopped: context cancelled\n")
			return nil
		case sig := <-sigCh:
			fmt.Fprintf(os.Stderr, "\nReceived %v, shutting down watch\n", sig)
			return nil
		case <-ticker.C:
			// next iteration
		}
	}
}

// runClean performs the actual cleanup and returns a summary. It is extracted
// as a function so that nil-client scenarios can be handled gracefully and
// the logic can be tested independently of cobra.
func runClean(ctx context.Context, k8sClient client.Client, namespace string) cleanSummary {
	var summary cleanSummary
	if k8sClient == nil {
		fmt.Fprintln(os.Stderr, "Warning: no Kubernetes client available, skipping cleanup")
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

	// 8. Remove orphaned chaos finalizers from resources
	summary.FinalizersRemoved = cleanOrphanedFinalizers(ctx, k8sClient, namespace)

	// 9. Restore config drifts (ConfigMaps and Secrets with rollback annotations)
	summary.ConfigDriftsFixed = cleanConfigDrift(ctx, k8sClient, namespace)

	// 10. Restore CRD mutations (resources with rollback annotations from CRDMutation)
	summary.CRDMutationsFixed = cleanCRDMutations(ctx, k8sClient, namespace)

	// 11. Clean chaos-result ConfigMaps
	summary.ResultConfigMaps = cleanResultConfigMaps(ctx, k8sClient, namespace)

	return summary
}

func deleteMatchingResources(
	ctx context.Context,
	k8sClient client.Client,
	list client.ObjectList,
	extractItems func() []client.Object,
	kind string,
	opts ...client.ListOption,
) int {
	if err := k8sClient.List(ctx, list, opts...); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: listing chaos %s: %v\n", kind, err)
		return 0
	}

	cleaned := 0
	for _, obj := range extractItems() {
		if ns := obj.GetNamespace(); ns != "" {
			fmt.Fprintf(os.Stderr, "Deleting %s %s/%s\n", kind, ns, obj.GetName())
		} else {
			fmt.Fprintf(os.Stderr, "Deleting %s %s\n", kind, obj.GetName())
		}
		if err := k8sClient.Delete(ctx, obj); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
		} else {
			cleaned++
		}
	}
	return cleaned
}

func cleanResultConfigMaps(ctx context.Context, k8sClient client.Client, namespace string) int {
	list := &corev1.ConfigMapList{}
	chaosResultLabels := client.MatchingLabels{"app.kubernetes.io/managed-by": "odh-chaos"}
	return deleteMatchingResources(ctx, k8sClient, list, func() []client.Object {
		items := make([]client.Object, len(list.Items))
		for i := range list.Items {
			items[i] = &list.Items[i]
		}
		return items
	}, "ConfigMap", client.InNamespace(namespace), chaosResultLabels)
}

func cleanNetworkPolicies(ctx context.Context, k8sClient client.Client, namespace string, labels client.MatchingLabels) int {
	list := &networkingv1.NetworkPolicyList{}
	return deleteMatchingResources(ctx, k8sClient, list, func() []client.Object {
		items := make([]client.Object, len(list.Items))
		for i := range list.Items {
			items[i] = &list.Items[i]
		}
		return items
	}, "NetworkPolicy", client.InNamespace(namespace), labels)
}

func cleanLeases(ctx context.Context, k8sClient client.Client, namespace string, labels client.MatchingLabels) int {
	list := &coordinationv1.LeaseList{}
	return deleteMatchingResources(ctx, k8sClient, list, func() []client.Object {
		items := make([]client.Object, len(list.Items))
		for i := range list.Items {
			items[i] = &list.Items[i]
		}
		return items
	}, "Lease", client.InNamespace(namespace), labels)
}

func cleanClusterRoles(ctx context.Context, k8sClient client.Client, labels client.MatchingLabels) int {
	list := &rbacv1.ClusterRoleList{}
	return deleteMatchingResources(ctx, k8sClient, list, func() []client.Object {
		items := make([]client.Object, len(list.Items))
		for i := range list.Items {
			items[i] = &list.Items[i]
		}
		return items
	}, "ClusterRole", labels)
}

func cleanRoleBindings(ctx context.Context, k8sClient client.Client, namespace string, labels client.MatchingLabels) int {
	list := &rbacv1.RoleBindingList{}
	return deleteMatchingResources(ctx, k8sClient, list, func() []client.Object {
		items := make([]client.Object, len(list.Items))
		for i := range list.Items {
			items[i] = &list.Items[i]
		}
		return items
	}, "RoleBinding", client.InNamespace(namespace), labels)
}

// cleanWebhookConfigurations finds ValidatingWebhookConfigurations that have a
// rollback annotation (set by the WebhookDisrupt injector), parses the original
// failure policies from the annotation, restores them, and removes chaos metadata.
func cleanWebhookConfigurations(ctx context.Context, k8sClient client.Client) int {
	webhooks := &admissionregv1.ValidatingWebhookConfigurationList{}
	if err := k8sClient.List(ctx, webhooks); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: listing ValidatingWebhookConfigurations: %v\n", err)
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
		if err := safety.UnwrapRollbackData(rollbackJSON, &originalPolicies); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: parsing rollback data for ValidatingWebhookConfiguration %q: %v\n", wc.Name, err)
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

		// Remove rollback annotation and chaos labels
		safety.RemoveChaosMetadata(wc, string(v1alpha1.WebhookDisrupt))

		fmt.Fprintf(os.Stderr, "Restoring ValidatingWebhookConfiguration %q\n", wc.Name)
		if err := k8sClient.Update(ctx, wc); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "Warning: listing ClusterRoleBindings: %v\n", err)
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
			if err := safety.UnwrapRollbackData(rollbackJSON, &originalSubjects); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: parsing rollback data for ClusterRoleBinding %q: %v\n", crb.Name, err)
				continue
			}

			crb.Subjects = originalSubjects

			// Remove rollback annotation and chaos labels
			safety.RemoveChaosMetadata(crb, string(v1alpha1.RBACRevoke))

			fmt.Fprintf(os.Stderr, "Restoring ClusterRoleBinding %q\n", crb.Name)
			if err := k8sClient.Update(ctx, crb); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "Warning: listing RoleBindings: %v\n", err)
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
			if err := safety.UnwrapRollbackData(rollbackJSON, &originalSubjects); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: parsing rollback data for RoleBinding %s/%s: %v\n", rb.Namespace, rb.Name, err)
				continue
			}

			rb.Subjects = originalSubjects

			// Remove rollback annotation and chaos labels
			safety.RemoveChaosMetadata(rb, string(v1alpha1.RBACRevoke))

			fmt.Fprintf(os.Stderr, "Restoring RoleBinding %s/%s\n", rb.Namespace, rb.Name)
			if err := k8sClient.Update(ctx, rb); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
			} else {
				restored++
			}
		}
	}

	return restored
}

// scanResources lists resources of a given type in the specified namespace and
// invokes itemCallback for each item. The extractItems function converts the
// typed list into a slice of client.Object pointers. On list error, a warning
// is printed and execution continues. The returned count is the number of items
// for which the callback returned true.
func scanResources(
	ctx context.Context,
	k8sClient client.Client,
	namespace string,
	list client.ObjectList,
	extractItems func() []client.Object,
	itemCallback func(client.Object) bool,
	resourceKind string,
	scanLabel string,
) int {
	if err := k8sClient.List(ctx, list, client.InNamespace(namespace)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: listing %s for %s scan: %v\n", resourceKind, scanLabel, err)
		return 0
	}

	count := 0
	for _, obj := range extractItems() {
		if itemCallback(obj) {
			count++
		}
	}
	return count
}

// cleanOrphanedFinalizers scans Deployments, ConfigMaps, Secrets, Services,
// StatefulSets, DaemonSets, and Jobs for the rollback annotation left by the
// FinalizerBlock injector. For each found, it removes the chaos finalizer,
// rollback annotation, and chaos labels.
func cleanOrphanedFinalizers(ctx context.Context, k8sClient client.Client, namespace string) int {
	cleaned := 0
	callback := func(obj client.Object) bool {
		return cleanFinalizerFromResource(ctx, k8sClient, obj, obj.GetName(), obj.GetNamespace())
	}

	deployments := &appsv1.DeploymentList{}
	cleaned += scanResources(ctx, k8sClient, namespace, deployments, func() []client.Object {
		items := make([]client.Object, len(deployments.Items))
		for i := range deployments.Items {
			items[i] = &deployments.Items[i]
		}
		return items
	}, callback, "Deployments", "finalizer")

	configMaps := &corev1.ConfigMapList{}
	cleaned += scanResources(ctx, k8sClient, namespace, configMaps, func() []client.Object {
		items := make([]client.Object, len(configMaps.Items))
		for i := range configMaps.Items {
			items[i] = &configMaps.Items[i]
		}
		return items
	}, callback, "ConfigMaps", "finalizer")

	secrets := &corev1.SecretList{}
	cleaned += scanResources(ctx, k8sClient, namespace, secrets, func() []client.Object {
		items := make([]client.Object, len(secrets.Items))
		for i := range secrets.Items {
			items[i] = &secrets.Items[i]
		}
		return items
	}, callback, "Secrets", "finalizer")

	services := &corev1.ServiceList{}
	cleaned += scanResources(ctx, k8sClient, namespace, services, func() []client.Object {
		items := make([]client.Object, len(services.Items))
		for i := range services.Items {
			items[i] = &services.Items[i]
		}
		return items
	}, callback, "Services", "finalizer")

	statefulSets := &appsv1.StatefulSetList{}
	cleaned += scanResources(ctx, k8sClient, namespace, statefulSets, func() []client.Object {
		items := make([]client.Object, len(statefulSets.Items))
		for i := range statefulSets.Items {
			items[i] = &statefulSets.Items[i]
		}
		return items
	}, callback, "StatefulSets", "finalizer")

	daemonSets := &appsv1.DaemonSetList{}
	cleaned += scanResources(ctx, k8sClient, namespace, daemonSets, func() []client.Object {
		items := make([]client.Object, len(daemonSets.Items))
		for i := range daemonSets.Items {
			items[i] = &daemonSets.Items[i]
		}
		return items
	}, callback, "DaemonSets", "finalizer")

	jobs := &batchv1.JobList{}
	cleaned += scanResources(ctx, k8sClient, namespace, jobs, func() []client.Object {
		items := make([]client.Object, len(jobs.Items))
		for i := range jobs.Items {
			items[i] = &jobs.Items[i]
		}
		return items
	}, callback, "Jobs", "finalizer")

	return cleaned
}

// cleanFinalizerFromResource checks a resource for the finalizer rollback annotation,
// removes the chaos finalizer, annotation, and labels if found.
func cleanFinalizerFromResource(ctx context.Context, k8sClient client.Client, obj client.Object, name, namespace string) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}
	rollbackJSON, ok := annotations[safety.RollbackAnnotationKey]
	if !ok {
		return false
	}

	// Check if this is a finalizer rollback (has "finalizer" key)
	var rollbackData map[string]string
	if err := safety.UnwrapRollbackData(rollbackJSON, &rollbackData); err != nil {
		return false
	}
	finalizerName, ok := rollbackData["finalizer"]
	if !ok {
		return false
	}

	// Remove the chaos finalizer
	controllerutil.RemoveFinalizer(obj, finalizerName)

	// Remove rollback annotation and chaos labels
	safety.RemoveChaosMetadata(obj, string(v1alpha1.FinalizerBlock))

	fmt.Fprintf(os.Stderr, "Removing orphaned finalizer %q from %s/%s\n", finalizerName, namespace, name)
	if err := k8sClient.Update(ctx, obj); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
		return false
	}
	return true
}

// cleanConfigDrift scans ConfigMaps and Secrets for rollback annotations
// left by the ConfigDrift injector, restores original values, and removes
// chaos metadata.
func cleanConfigDrift(ctx context.Context, k8sClient client.Client, namespace string) int {
	restored := 0

	// Scan ConfigMaps
	configMaps := &corev1.ConfigMapList{}
	if err := k8sClient.List(ctx, configMaps, client.InNamespace(namespace)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: listing ConfigMaps for config drift scan: %v\n", err)
	} else {
		for i := range configMaps.Items {
			cm := &configMaps.Items[i]
			annotations := cm.GetAnnotations()
			if annotations == nil {
				continue
			}
			rollbackJSON, ok := annotations[safety.RollbackAnnotationKey]
			if !ok {
				continue
			}

			var rollbackData map[string]string
			if err := safety.UnwrapRollbackData(rollbackJSON, &rollbackData); err != nil {
				continue
			}
			if rollbackData["resourceType"] != "ConfigMap" {
				continue
			}

			dataKey := rollbackData["key"]

			if rollbackData["keyExists"] == "false" {
				delete(cm.Data, dataKey)
			} else {
				originalValue := rollbackData["originalValue"]
				if cm.Data == nil {
					cm.Data = make(map[string]string)
				}
				cm.Data[dataKey] = originalValue
			}

			// Remove rollback annotation and chaos labels
			safety.RemoveChaosMetadata(cm, string(v1alpha1.ConfigDrift))

			fmt.Fprintf(os.Stderr, "Restoring ConfigMap %s/%s key %q\n", cm.Namespace, cm.Name, dataKey)
			if err := k8sClient.Update(ctx, cm); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
			} else {
				restored++
			}
		}
	}

	// Scan Secrets
	secrets := &corev1.SecretList{}
	if err := k8sClient.List(ctx, secrets, client.InNamespace(namespace)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: listing Secrets for config drift scan: %v\n", err)
	} else {
		for i := range secrets.Items {
			s := &secrets.Items[i]
			annotations := s.GetAnnotations()
			if annotations == nil {
				continue
			}
			rollbackJSON, ok := annotations[safety.RollbackAnnotationKey]
			if !ok {
				continue
			}

			var rollbackData map[string]string
			if err := safety.UnwrapRollbackData(rollbackJSON, &rollbackData); err != nil {
				continue
			}
			if rollbackData["resourceType"] != "Secret" {
				continue
			}

			dataKey := rollbackData["key"]

			if s.Data == nil {
				s.Data = make(map[string][]byte)
			}

			// Check for rollbackSecretRef (new format) vs originalValue (legacy)
			if rollbackSecretRef, hasRef := rollbackData["rollbackSecretRef"]; hasRef {
				if rollbackData["keyExists"] == "false" {
					// Key did not exist before injection — remove it and clean up rollback Secret
					delete(s.Data, dataKey)
					safety.RemoveChaosMetadata(s, string(v1alpha1.ConfigDrift))

					fmt.Fprintf(os.Stderr, "Restoring Secret %s/%s (removing injected key %q)\n", s.Namespace, s.Name, dataKey)
					if err := k8sClient.Update(ctx, s); err != nil {
						fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
					} else {
						restored++
					}
				} else {
					// Read original value from dedicated rollback Secret
					rbSecret := &corev1.Secret{}
					rbKey := client.ObjectKey{Name: rollbackSecretRef, Namespace: s.Namespace}
					if err := k8sClient.Get(ctx, rbKey, rbSecret); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: reading rollback Secret %q for Secret %s/%s: %v\n",
							rollbackSecretRef, s.Namespace, s.Name, err)
						continue
					}
					s.Data[dataKey] = rbSecret.Data[dataKey]

					// Remove rollback annotation and chaos labels
					safety.RemoveChaosMetadata(s, string(v1alpha1.ConfigDrift))

					fmt.Fprintf(os.Stderr, "Restoring Secret %s/%s key %q from rollback Secret %q\n",
						s.Namespace, s.Name, dataKey, rollbackSecretRef)
					if err := k8sClient.Update(ctx, s); err != nil {
						fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
						continue
					}
					restored++
				}

				// Clean up rollback Secret if it exists
				rbSecret := &corev1.Secret{}
				rbKey := client.ObjectKey{Name: rollbackSecretRef, Namespace: s.Namespace}
				if err := k8sClient.Get(ctx, rbKey, rbSecret); err == nil {
					if err := k8sClient.Delete(ctx, rbSecret); err != nil {
						fmt.Fprintf(os.Stderr, "  Warning: deleting rollback Secret %q: %v\n", rollbackSecretRef, err)
					}
				}
			} else if rollbackData["keyExists"] == "false" {
				// Key did not exist before injection — remove it
				delete(s.Data, dataKey)

				// Remove rollback annotation and chaos labels
				safety.RemoveChaosMetadata(s, string(v1alpha1.ConfigDrift))

				fmt.Fprintf(os.Stderr, "Restoring Secret %s/%s (removing injected key %q)\n", s.Namespace, s.Name, dataKey)
				if err := k8sClient.Update(ctx, s); err != nil {
					fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
				} else {
					restored++
				}
			} else {
				// Legacy format: originalValue stored directly
				originalValue := rollbackData["originalValue"]
				s.Data[dataKey] = []byte(originalValue)

				// Remove rollback annotation and chaos labels
				safety.RemoveChaosMetadata(s, string(v1alpha1.ConfigDrift))

				fmt.Fprintf(os.Stderr, "Restoring Secret %s/%s key %q\n", s.Namespace, s.Name, dataKey)
				if err := k8sClient.Update(ctx, s); err != nil {
					fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
				} else {
					restored++
				}
			}
		}
	}

	return restored
}

// cleanCRDMutations scans core resource types (Deployments, ConfigMaps, Secrets,
// Services, StatefulSets) for rollback annotations that contain both "apiVersion"
// and "kind" keys -- the signature of CRDMutation rollback data. For each found,
// it removes the rollback annotation and chaos labels, and logs the rollback data
// for manual field restoration. It does NOT attempt to restore the field value,
// as that requires unstructured patch knowledge.
func cleanCRDMutations(ctx context.Context, k8sClient client.Client, namespace string) int {
	cleaned := 0
	callback := func(obj client.Object) bool {
		return cleanCRDMutationFromResource(ctx, k8sClient, obj, obj.GetName(), obj.GetNamespace())
	}

	deployments := &appsv1.DeploymentList{}
	cleaned += scanResources(ctx, k8sClient, namespace, deployments, func() []client.Object {
		items := make([]client.Object, len(deployments.Items))
		for i := range deployments.Items {
			items[i] = &deployments.Items[i]
		}
		return items
	}, callback, "Deployments", "CRD mutation")

	configMaps := &corev1.ConfigMapList{}
	cleaned += scanResources(ctx, k8sClient, namespace, configMaps, func() []client.Object {
		items := make([]client.Object, len(configMaps.Items))
		for i := range configMaps.Items {
			items[i] = &configMaps.Items[i]
		}
		return items
	}, callback, "ConfigMaps", "CRD mutation")

	secrets := &corev1.SecretList{}
	cleaned += scanResources(ctx, k8sClient, namespace, secrets, func() []client.Object {
		items := make([]client.Object, len(secrets.Items))
		for i := range secrets.Items {
			items[i] = &secrets.Items[i]
		}
		return items
	}, callback, "Secrets", "CRD mutation")

	services := &corev1.ServiceList{}
	cleaned += scanResources(ctx, k8sClient, namespace, services, func() []client.Object {
		items := make([]client.Object, len(services.Items))
		for i := range services.Items {
			items[i] = &services.Items[i]
		}
		return items
	}, callback, "Services", "CRD mutation")

	statefulSets := &appsv1.StatefulSetList{}
	cleaned += scanResources(ctx, k8sClient, namespace, statefulSets, func() []client.Object {
		items := make([]client.Object, len(statefulSets.Items))
		for i := range statefulSets.Items {
			items[i] = &statefulSets.Items[i]
		}
		return items
	}, callback, "StatefulSets", "CRD mutation")

	return cleaned
}

// cleanCRDMutationFromResource checks a resource for the CRDMutation rollback
// signature (rollback JSON containing both "apiVersion" and "kind" keys).
// If found, it restores the original field value via a merge patch and removes
// the rollback annotation and chaos labels.
func cleanCRDMutationFromResource(ctx context.Context, k8sClient client.Client, obj client.Object, name, namespace string) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}
	rollbackJSON, ok := annotations[safety.RollbackAnnotationKey]
	if !ok {
		return false
	}

	// Parse the rollback data as a generic map to check for CRDMutation signature
	var rollbackData map[string]any
	if err := safety.UnwrapRollbackData(rollbackJSON, &rollbackData); err != nil {
		return false
	}

	// CRDMutation signature: must have both "apiVersion" and "kind" keys
	_, hasAPIVersion := rollbackData["apiVersion"]
	_, hasKind := rollbackData["kind"]
	if !hasAPIVersion || !hasKind {
		return false
	}

	// Log the restoration to stderr (avoid leaking sensitive original values to stdout)
	fmt.Fprintf(os.Stderr, "Restoring CRD mutation metadata on %s/%s (field: %v)\n", namespace, name, rollbackData["field"])

	// Build a merge patch that restores the field value and removes chaos metadata.
	// Setting an annotation/label to null in a merge patch removes it.
	restoreAnnotations := map[string]any{
		safety.RollbackAnnotationKey: nil,
	}
	chaosLabels := safety.ChaosLabels(string(v1alpha1.CRDMutation))
	restoreLabels := make(map[string]any, len(chaosLabels))
	for k := range chaosLabels {
		restoreLabels[k] = nil
	}

	patchMap := map[string]any{
		"metadata": map[string]any{
			"annotations": restoreAnnotations,
			"labels":      restoreLabels,
		},
	}

	// Restore the field value under spec if rollback data contains field info
	if fieldName, ok := rollbackData["field"]; ok {
		if fieldStr, ok := fieldName.(string); ok && fieldStr != "" {
			originalValue := rollbackData["originalValue"]
			patchMap["spec"] = map[string]any{
				fieldStr: originalValue,
			}
		}
	}

	patch, err := json.Marshal(patchMap)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: building restore patch: %v\n", err)
		return false
	}

	if err := k8sClient.Patch(ctx, obj, client.RawPatch(types.MergePatchType, patch)); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
		return false
	}
	return true
}

// cleanTTLExpired scans all NetworkPolicies in the namespace for those with
// a TTL annotation that has expired, regardless of whether they have the
// managed-by label. This acts as a belt-and-suspenders safety net.
func cleanTTLExpired(ctx context.Context, k8sClient client.Client, namespace string) int {
	policies := &networkingv1.NetworkPolicyList{}
	if err := k8sClient.List(ctx, policies,
		client.InNamespace(namespace),
	); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: listing NetworkPolicies for TTL scan: %v\n", err)
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
		if safety.IsExpired(time.Now(), expiryStr) {
			fmt.Fprintf(os.Stderr, "Deleting TTL-expired NetworkPolicy %s/%s (expired: %s)\n",
				policies.Items[i].Namespace, policies.Items[i].Name, expiryStr)
			if err := k8sClient.Delete(ctx, &policies.Items[i]); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
			} else {
				cleaned++
			}
		}
	}
	return cleaned
}
