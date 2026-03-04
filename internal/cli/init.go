package cli

import (
	"fmt"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
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
		params = fmt.Sprintf(`      apiVersion: "apps/v1"
      kind: "Deployment"
      name: "%s"
      field: "replicas"
      value: "0"`, component)
		dangerLevel = fmt.Sprintf("    dangerLevel: %s", v1alpha1.DangerLevelHigh)
		allowDangerous = "    allowDangerous: true"
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
	default: // PodKill
		params = fmt.Sprintf("      labelSelector: \"app.kubernetes.io/part-of=%s\"", component)
	}
	return params, dangerLevel, allowDangerous
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

			tmpl := `apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: %s-%s
  labels:
    component: %s
spec:
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
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - %s
    dryRun: false%s
`
			fmt.Fprintf(cmd.OutOrStdout(), tmpl,
				component, injectionType,
				component,
				operator, component, component,
				component, injectionType,
				injectionType,
				dangerLevelLine,
				params,
				namespace,
				allowDangerousLine,
			)
			return nil
		},
	}

	cmd.Flags().StringVar(&component, "component", "", "target component name (required)")
	cmd.Flags().StringVar(&injectionType, "type", "PodKill", "injection type")
	cmd.Flags().StringVar(&operator, "operator", "opendatahub-operator", "target operator")
	cmd.Flags().StringVar(&namespace, "namespace", v1alpha1.DefaultNamespace, "target namespace")

	return cmd
}
