package cli

import (
	"fmt"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/spf13/cobra"
)

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
    expectedBehavior: "Operator reconciles all managed resources"
    recoveryTimeout: "60s"
  injection:
    type: %s
    count: 1
    ttl: "300s"
    parameters:
      labelSelector: "app.kubernetes.io/part-of=%s"
  observation:
    interval: "5s"
    duration: "120s"
    trackReconcileCycles: true
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: %s
        namespace: %s
        conditionType: Available
    timeout: "30s"
  blastRadius:
    maxPodsAffected: 1
    maxConcurrentFaults: 1
    allowedNamespaces:
      - %s
    dryRun: false
`
			fmt.Printf(tmpl,
				component, injectionType,
				component,
				operator, component, component,
				component, injectionType,
				injectionType,
				component,
				component, namespace,
				namespace,
			)
			return nil
		},
	}

	cmd.Flags().StringVar(&component, "component", "", "target component name (required)")
	cmd.Flags().StringVar(&injectionType, "type", "PodKill", "injection type")
	cmd.Flags().StringVar(&operator, "operator", "opendatahub-operator", "target operator")
	cmd.Flags().StringVar(&namespace, "namespace", "opendatahub", "target namespace")

	return cmd
}
