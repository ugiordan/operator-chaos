package cli

import (
	"fmt"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/spf13/cobra"
)

func newTypesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "types",
		Short: "List available injection types",
		Run: func(cmd *cobra.Command, args []string) {
			types := []struct {
				name   string
				desc   string
				danger v1alpha1.DangerLevel
			}{
				{"PodKill", "Delete pods matching a label selector", v1alpha1.DangerLevelLow},
				{"NetworkPartition", "Create deny-all NetworkPolicy", v1alpha1.DangerLevelMedium},
				{"ConfigDrift", "Modify ConfigMap or Secret data", v1alpha1.DangerLevelMedium},
				{"CRDMutation", "Mutate a field on any Kubernetes resource", v1alpha1.DangerLevelMedium},
				{"FinalizerBlock", "Add a blocking finalizer to a resource", v1alpha1.DangerLevelMedium},
				{"WebhookDisrupt", "Change webhook failure policy", v1alpha1.DangerLevelHigh},
				{"RBACRevoke", "Revoke RBAC binding subjects", v1alpha1.DangerLevelHigh},
				{"ClientFault", "Inject API-level faults into controller-runtime client", v1alpha1.DangerLevelMedium},
			}
			fmt.Println("Available injection types:")
			fmt.Println()
			for _, t := range types {
				fmt.Printf("  %-20s [%s] %s\n", t.name, t.danger, t.desc)
			}
		},
	}
}
