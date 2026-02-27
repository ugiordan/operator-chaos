package cli

import (
	"context"
	"encoding/json"
	"fmt"
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
}

func (s cleanSummary) total() int {
	return s.NetworkPolicies + s.Leases + s.ClusterRoles + s.RoleBindings +
		s.TTLExpired + s.WebhooksRestored + s.RBACBindingsFixed +
		s.FinalizersRemoved + s.ConfigDriftsFixed + s.CRDMutationsFixed
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

	// 8. Remove orphaned chaos finalizers from resources
	summary.FinalizersRemoved = cleanOrphanedFinalizers(ctx, k8sClient, namespace)

	// 9. Restore config drifts (ConfigMaps and Secrets with rollback annotations)
	summary.ConfigDriftsFixed = cleanConfigDrift(ctx, k8sClient, namespace)

	// 10. Restore CRD mutations (resources with rollback annotations from CRDMutation)
	summary.CRDMutationsFixed = cleanCRDMutations(ctx, k8sClient, namespace)

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
		if err := safety.UnwrapRollbackData(rollbackJSON, &originalPolicies); err != nil {
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
			if err := safety.UnwrapRollbackData(rollbackJSON, &originalSubjects); err != nil {
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
			if err := safety.UnwrapRollbackData(rollbackJSON, &originalSubjects); err != nil {
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

// cleanOrphanedFinalizers scans Deployments, ConfigMaps, Secrets, Services,
// StatefulSets, DaemonSets, and Jobs for the rollback annotation left by the
// FinalizerBlock injector. For each found, it removes the chaos finalizer,
// rollback annotation, and chaos labels.
func cleanOrphanedFinalizers(ctx context.Context, k8sClient client.Client, namespace string) int {
	cleaned := 0

	// Scan Deployments
	deployments := &appsv1.DeploymentList{}
	if err := k8sClient.List(ctx, deployments, client.InNamespace(namespace)); err != nil {
		fmt.Printf("Warning: listing Deployments for finalizer scan: %v\n", err)
	} else {
		for i := range deployments.Items {
			dep := &deployments.Items[i]
			if cleanFinalizerFromResource(ctx, k8sClient, dep, dep.Name, dep.Namespace) {
				cleaned++
			}
		}
	}

	// Scan ConfigMaps
	configMaps := &corev1.ConfigMapList{}
	if err := k8sClient.List(ctx, configMaps, client.InNamespace(namespace)); err != nil {
		fmt.Printf("Warning: listing ConfigMaps for finalizer scan: %v\n", err)
	} else {
		for i := range configMaps.Items {
			cm := &configMaps.Items[i]
			if cleanFinalizerFromResource(ctx, k8sClient, cm, cm.Name, cm.Namespace) {
				cleaned++
			}
		}
	}

	// Scan Secrets
	secrets := &corev1.SecretList{}
	if err := k8sClient.List(ctx, secrets, client.InNamespace(namespace)); err != nil {
		fmt.Printf("Warning: listing Secrets for finalizer scan: %v\n", err)
	} else {
		for i := range secrets.Items {
			s := &secrets.Items[i]
			if cleanFinalizerFromResource(ctx, k8sClient, s, s.Name, s.Namespace) {
				cleaned++
			}
		}
	}

	// Scan Services
	services := &corev1.ServiceList{}
	if err := k8sClient.List(ctx, services, client.InNamespace(namespace)); err != nil {
		fmt.Printf("Warning: listing Services for finalizer scan: %v\n", err)
	} else {
		for i := range services.Items {
			svc := &services.Items[i]
			if cleanFinalizerFromResource(ctx, k8sClient, svc, svc.Name, svc.Namespace) {
				cleaned++
			}
		}
	}

	// Scan StatefulSets
	statefulSets := &appsv1.StatefulSetList{}
	if err := k8sClient.List(ctx, statefulSets, client.InNamespace(namespace)); err != nil {
		fmt.Printf("Warning: listing StatefulSets for finalizer scan: %v\n", err)
	} else {
		for i := range statefulSets.Items {
			ss := &statefulSets.Items[i]
			if cleanFinalizerFromResource(ctx, k8sClient, ss, ss.Name, ss.Namespace) {
				cleaned++
			}
		}
	}

	// Scan DaemonSets
	daemonSets := &appsv1.DaemonSetList{}
	if err := k8sClient.List(ctx, daemonSets, client.InNamespace(namespace)); err != nil {
		fmt.Printf("Warning: listing DaemonSets for finalizer scan: %v\n", err)
	} else {
		for i := range daemonSets.Items {
			ds := &daemonSets.Items[i]
			if cleanFinalizerFromResource(ctx, k8sClient, ds, ds.Name, ds.Namespace) {
				cleaned++
			}
		}
	}

	// Scan Jobs
	jobs := &batchv1.JobList{}
	if err := k8sClient.List(ctx, jobs, client.InNamespace(namespace)); err != nil {
		fmt.Printf("Warning: listing Jobs for finalizer scan: %v\n", err)
	} else {
		for i := range jobs.Items {
			job := &jobs.Items[i]
			if cleanFinalizerFromResource(ctx, k8sClient, job, job.Name, job.Namespace) {
				cleaned++
			}
		}
	}

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

	// Remove rollback annotation
	delete(annotations, safety.RollbackAnnotationKey)
	obj.SetAnnotations(annotations)

	// Remove chaos labels
	labels := obj.GetLabels()
	if labels != nil {
		for k := range safety.ChaosLabels(string(v1alpha1.FinalizerBlock)) {
			delete(labels, k)
		}
		obj.SetLabels(labels)
	}

	fmt.Printf("Removing orphaned finalizer %q from %s/%s\n", finalizerName, namespace, name)
	if err := k8sClient.Update(ctx, obj); err != nil {
		fmt.Printf("  Warning: %v\n", err)
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
		fmt.Printf("Warning: listing ConfigMaps for config drift scan: %v\n", err)
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
			originalValue := rollbackData["originalValue"]

			if cm.Data == nil {
				cm.Data = make(map[string]string)
			}
			cm.Data[dataKey] = originalValue

			delete(cm.Annotations, safety.RollbackAnnotationKey)
			for k := range safety.ChaosLabels(string(v1alpha1.ConfigDrift)) {
				delete(cm.Labels, k)
			}

			fmt.Printf("Restoring ConfigMap %s/%s key %q\n", cm.Namespace, cm.Name, dataKey)
			if err := k8sClient.Update(ctx, cm); err != nil {
				fmt.Printf("  Warning: %v\n", err)
			} else {
				restored++
			}
		}
	}

	// Scan Secrets
	secrets := &corev1.SecretList{}
	if err := k8sClient.List(ctx, secrets, client.InNamespace(namespace)); err != nil {
		fmt.Printf("Warning: listing Secrets for config drift scan: %v\n", err)
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
				// Read original value from dedicated rollback Secret
				rbSecret := &corev1.Secret{}
				rbKey := client.ObjectKey{Name: rollbackSecretRef, Namespace: s.Namespace}
				if err := k8sClient.Get(ctx, rbKey, rbSecret); err != nil {
					fmt.Printf("Warning: reading rollback Secret %q for Secret %s/%s: %v\n",
						rollbackSecretRef, s.Namespace, s.Name, err)
					continue
				}
				s.Data[dataKey] = rbSecret.Data[dataKey]

				delete(s.Annotations, safety.RollbackAnnotationKey)
				for k := range safety.ChaosLabels(string(v1alpha1.ConfigDrift)) {
					delete(s.Labels, k)
				}

				fmt.Printf("Restoring Secret %s/%s key %q from rollback Secret %q\n",
					s.Namespace, s.Name, dataKey, rollbackSecretRef)
				if err := k8sClient.Update(ctx, s); err != nil {
					fmt.Printf("  Warning: %v\n", err)
					continue
				}

				// Delete the rollback Secret
				if err := k8sClient.Delete(ctx, rbSecret); err != nil {
					fmt.Printf("  Warning: deleting rollback Secret %q: %v\n", rollbackSecretRef, err)
				}
				restored++
			} else {
				// Legacy format: originalValue stored directly
				originalValue := rollbackData["originalValue"]
				s.Data[dataKey] = []byte(originalValue)

				delete(s.Annotations, safety.RollbackAnnotationKey)
				for k := range safety.ChaosLabels(string(v1alpha1.ConfigDrift)) {
					delete(s.Labels, k)
				}

				fmt.Printf("Restoring Secret %s/%s key %q\n", s.Namespace, s.Name, dataKey)
				if err := k8sClient.Update(ctx, s); err != nil {
					fmt.Printf("  Warning: %v\n", err)
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

	// Scan Deployments
	deployments := &appsv1.DeploymentList{}
	if err := k8sClient.List(ctx, deployments, client.InNamespace(namespace)); err != nil {
		fmt.Printf("Warning: listing Deployments for CRD mutation scan: %v\n", err)
	} else {
		for i := range deployments.Items {
			dep := &deployments.Items[i]
			if cleanCRDMutationFromResource(ctx, k8sClient, dep, dep.Name, dep.Namespace) {
				cleaned++
			}
		}
	}

	// Scan ConfigMaps
	configMaps := &corev1.ConfigMapList{}
	if err := k8sClient.List(ctx, configMaps, client.InNamespace(namespace)); err != nil {
		fmt.Printf("Warning: listing ConfigMaps for CRD mutation scan: %v\n", err)
	} else {
		for i := range configMaps.Items {
			cm := &configMaps.Items[i]
			if cleanCRDMutationFromResource(ctx, k8sClient, cm, cm.Name, cm.Namespace) {
				cleaned++
			}
		}
	}

	// Scan Secrets
	secrets := &corev1.SecretList{}
	if err := k8sClient.List(ctx, secrets, client.InNamespace(namespace)); err != nil {
		fmt.Printf("Warning: listing Secrets for CRD mutation scan: %v\n", err)
	} else {
		for i := range secrets.Items {
			s := &secrets.Items[i]
			if cleanCRDMutationFromResource(ctx, k8sClient, s, s.Name, s.Namespace) {
				cleaned++
			}
		}
	}

	// Scan Services
	services := &corev1.ServiceList{}
	if err := k8sClient.List(ctx, services, client.InNamespace(namespace)); err != nil {
		fmt.Printf("Warning: listing Services for CRD mutation scan: %v\n", err)
	} else {
		for i := range services.Items {
			svc := &services.Items[i]
			if cleanCRDMutationFromResource(ctx, k8sClient, svc, svc.Name, svc.Namespace) {
				cleaned++
			}
		}
	}

	// Scan StatefulSets
	statefulSets := &appsv1.StatefulSetList{}
	if err := k8sClient.List(ctx, statefulSets, client.InNamespace(namespace)); err != nil {
		fmt.Printf("Warning: listing StatefulSets for CRD mutation scan: %v\n", err)
	} else {
		for i := range statefulSets.Items {
			ss := &statefulSets.Items[i]
			if cleanCRDMutationFromResource(ctx, k8sClient, ss, ss.Name, ss.Namespace) {
				cleaned++
			}
		}
	}

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
	var rollbackData map[string]interface{}
	if err := safety.UnwrapRollbackData(rollbackJSON, &rollbackData); err != nil {
		return false
	}

	// CRDMutation signature: must have both "apiVersion" and "kind" keys
	_, hasAPIVersion := rollbackData["apiVersion"]
	_, hasKind := rollbackData["kind"]
	if !hasAPIVersion || !hasKind {
		return false
	}

	// Log the restoration (avoid leaking sensitive original values to stdout)
	fmt.Printf("Restoring CRD mutation metadata on %s/%s (field: %v)\n", namespace, name, rollbackData["field"])

	// Build a merge patch that restores the field value and removes chaos metadata.
	// Setting an annotation/label to null in a merge patch removes it.
	restoreAnnotations := map[string]interface{}{
		safety.RollbackAnnotationKey: nil,
	}
	chaosLabels := safety.ChaosLabels(string(v1alpha1.CRDMutation))
	restoreLabels := make(map[string]interface{}, len(chaosLabels))
	for k := range chaosLabels {
		restoreLabels[k] = nil
	}

	patchMap := map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": restoreAnnotations,
			"labels":      restoreLabels,
		},
	}

	// Restore the field value under spec if rollback data contains field info
	if fieldName, ok := rollbackData["field"]; ok {
		if fieldStr, ok := fieldName.(string); ok && fieldStr != "" {
			originalValue := rollbackData["originalValue"]
			patchMap["spec"] = map[string]interface{}{
				fieldStr: originalValue,
			}
		}
	}

	patch, err := json.Marshal(patchMap)
	if err != nil {
		fmt.Printf("  Warning: building restore patch: %v\n", err)
		return false
	}

	if err := k8sClient.Patch(ctx, obj, client.RawPatch(types.MergePatchType, patch)); err != nil {
		fmt.Printf("  Warning: %v\n", err)
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
