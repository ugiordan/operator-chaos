package cli

import (
	"fmt"

	v1alpha1 "github.com/opendatahub-io/operator-chaos/api/v1alpha1"
	"github.com/spf13/cobra"
)

// injectionParameters returns the YAML fragment for the parameters section,
// an optional dangerLevel line, and an optional allowDangerous line,
// based on the injection type.
func injectionParameters(injType v1alpha1.InjectionType, component string) (params string, dangerLevel string, allowDangerous string) {
	switch injType {
	case v1alpha1.NetworkPartition:
		params = fmt.Sprintf("      labelSelector: \"app.kubernetes.io/part-of=%s\"", component)
	case v1alpha1.CRDMutation:
		params = fmt.Sprintf(`      apiVersion: "replace-with-crd-api-version"
      kind: "replace-with-crd-kind"
      name: "%s"
      field: "chaosTest"
      value: "injected"`, component)
	case v1alpha1.ConfigDrift:
		params = fmt.Sprintf(`      name: "%s-config"
      key: "replace-key"
      value: "replace-value"`, component)
	case v1alpha1.WebhookDisrupt:
		params = fmt.Sprintf(`      webhookName: "%s-webhook"
      action: "setFailurePolicy"`, component)
		dangerLevel = fmt.Sprintf("    dangerLevel: %s", v1alpha1.DangerLevelHigh)
		allowDangerous = "    allowDangerous: true"
	case v1alpha1.RBACRevoke:
		params = fmt.Sprintf(`      bindingName: "%s-binding"
      bindingType: "ClusterRoleBinding"`, component)
		dangerLevel = fmt.Sprintf("    dangerLevel: %s", v1alpha1.DangerLevelHigh)
		allowDangerous = "    allowDangerous: true"
	case v1alpha1.FinalizerBlock:
		params = fmt.Sprintf(`      kind: "Deployment"
      name: "%s"`, component)
	case v1alpha1.ClientFault:
		params = `      faults: '{"reconcile":{"errorRate":0.3,"error":"connection refused"}}'`
		dangerLevel = fmt.Sprintf("    dangerLevel: %s", v1alpha1.DangerLevelMedium)
	case v1alpha1.OwnerRefOrphan:
		params = fmt.Sprintf(`      apiVersion: "apps/v1"
      kind: "Deployment"
      name: "%s"`, component)
	case v1alpha1.LabelStomping:
		params = fmt.Sprintf(`      apiVersion: "apps/v1"
      kind: "Deployment"
      name: "%s"
      labelKey: "chaos-test-label"
      action: "delete"`, component)
		dangerLevel = fmt.Sprintf("    dangerLevel: %s", v1alpha1.DangerLevelMedium)
	case v1alpha1.WebhookLatency:
		params = fmt.Sprintf(`      webhookName: "%s-webhook"
      resources: "pods,deployments"
      apiGroups: "apps"
      delay: "5s"`, component)
		dangerLevel = fmt.Sprintf("    dangerLevel: %s", v1alpha1.DangerLevelHigh)
		allowDangerous = "    allowDangerous: true"
	case v1alpha1.NamespaceDeletion:
		params = `      namespace: "replace-with-target-namespace"`
		dangerLevel = fmt.Sprintf("    dangerLevel: %s", v1alpha1.DangerLevelHigh)
		allowDangerous = "    allowDangerous: true"
	case v1alpha1.QuotaExhaustion:
		params = fmt.Sprintf(`      quotaName: "%s-quota"
      cpu: "100m"`, component)
		dangerLevel = fmt.Sprintf("    dangerLevel: %s", v1alpha1.DangerLevelHigh)
		allowDangerous = "    allowDangerous: true"
	default: // PodKill
		params = fmt.Sprintf("      labelSelector: \"app.kubernetes.io/part-of=%s\"", component)
	}
	return params, dangerLevel, allowDangerous
}

func defaultTierForInjectionType(t v1alpha1.InjectionType) int32 {
	switch t {
	case v1alpha1.PodKill:
		return 1
	case v1alpha1.ConfigDrift, v1alpha1.NetworkPartition:
		return 2
	case v1alpha1.CRDMutation, v1alpha1.FinalizerBlock, v1alpha1.OwnerRefOrphan, v1alpha1.LabelStomping, v1alpha1.ClientFault:
		return 3
	case v1alpha1.WebhookDisrupt, v1alpha1.RBACRevoke, v1alpha1.WebhookLatency:
		return 4
	case v1alpha1.NamespaceDeletion, v1alpha1.QuotaExhaustion:
		return 5
	default:
		return 1
	}
}

func newInitCommand() *cobra.Command {
	var (
		component     string
		injectionType string
		operator      string
		namespace     string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Generate a skeleton experiment YAML",
		RunE: func(cmd *cobra.Command, args []string) error {
			if component == "" {
				return fmt.Errorf("--component is required")
			}
			if operator == "" {
				return fmt.Errorf("--operator is required")
			}
			if injectionType == "" {
				injectionType = string(v1alpha1.PodKill)
			}

			injType := v1alpha1.InjectionType(injectionType)
			if err := v1alpha1.ValidateInjectionType(injType); err != nil {
				return err
			}
			params, dangerLevel, allowDangerous := injectionParameters(injType, component)

			dangerLevelLine := ""
			if dangerLevel != "" {
				dangerLevelLine = "\n" + dangerLevel
			}

			allowDangerousLine := ""
			if allowDangerous != "" {
				allowDangerousLine = "\n" + allowDangerous
			}

			// Cluster-scoped injection types must NOT include allowedNamespaces
			clusterScoped := injType == v1alpha1.WebhookDisrupt ||
				injType == v1alpha1.RBACRevoke ||
				injType == v1alpha1.WebhookLatency

			if clusterScoped && namespace != v1alpha1.DefaultNamespace {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: --namespace is ignored for cluster-scoped injection type %s\n", injType)
			}

			var blastRadiusBlock string
			if clusterScoped {
				blastRadiusBlock = fmt.Sprintf(`  blastRadius:
    maxPodsAffected: 1
    dryRun: false%s`, allowDangerousLine)
			} else {
				blastRadiusBlock = fmt.Sprintf(`  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - %s
    dryRun: false%s`, namespace, allowDangerousLine)
			}

			tmpl := `apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: %s-%s
  labels:
    component: %s
spec:
  tier: %d
  target:
    operator: %s
    component: %s
    resource: "Deployment/%s"
  hypothesis:
    description: "%s recovers from %s"
    recoveryTimeout: "60s"
  injection:
    type: %s
    count: 1
    ttl: "300s"%s
    parameters:
%s
%s
`
			tier := defaultTierForInjectionType(injType)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), tmpl,
				component, injectionType,
				component,
				tier,
				operator, component, component,
				component, injectionType,
				injectionType,
				dangerLevelLine,
				params,
				blastRadiusBlock,
			)
			return nil
		},
	}

	cmd.Flags().StringVar(&component, "component", "", "target component name (required)")
	cmd.Flags().StringVar(&injectionType, "type", "PodKill", "injection type (PodKill|NetworkPartition|CRDMutation|ConfigDrift|WebhookDisrupt|RBACRevoke|FinalizerBlock|ClientFault|OwnerRefOrphan|QuotaExhaustion|WebhookLatency|NamespaceDeletion|LabelStomping)")
	cmd.Flags().StringVar(&operator, "operator", "", "target operator (required)")
	cmd.Flags().StringVar(&namespace, "namespace", v1alpha1.DefaultNamespace, "target namespace")

	return cmd
}
