package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
)

// versionValidationResult holds the check results for a single operator.
type versionValidationResult struct {
	Operator   string                       `json:"operator"`
	Components []componentValidationResult  `json:"components"`
	Status     string                       `json:"status"`
}

// componentValidationResult holds the check results for a single component.
type componentValidationResult struct {
	Component      string `json:"component"`
	ResourcesFound int    `json:"resourcesFound"`
	ResourcesTotal int    `json:"resourcesTotal"`
	Status         string `json:"status"`
}

func newValidateVersionCommand() *cobra.Command {
	var (
		knowledgeDir string
		format       string
	)

	cmd := &cobra.Command{
		Use:   "validate-version",
		Short: "Validate versioned knowledge models against a live cluster",
		Long: `Loads all knowledge models from a versioned directory and checks that
the expected managed resources exist on the cluster. Useful for verifying
that a knowledge directory accurately describes the current cluster state.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString("namespace")

			// 1. Load all models from knowledge dir
			models, err := model.LoadKnowledgeDir(knowledgeDir)
			if err != nil {
				return fmt.Errorf("loading knowledge dir: %w", err)
			}
			if len(models) == 0 {
				return fmt.Errorf("no knowledge models found in %s", knowledgeDir)
			}

			// 2. Extract version/platform from first model that has them
			version, platform := extractVersionPlatform(models)

			fmt.Printf("Knowledge dir: %s\n", knowledgeDir)
			if version != "" {
				fmt.Printf("Version:       %s\n", version)
			}
			if platform != "" {
				fmt.Printf("Platform:      %s\n", platform)
			}
			fmt.Printf("Operators:     %d\n", len(models))

			// 3. Connect to cluster
			cfg, err := config.GetConfig()
			if err != nil {
				return fmt.Errorf("getting kubeconfig: %w", err)
			}

			k8sClient, err := client.New(cfg, client.Options{})
			if err != nil {
				return fmt.Errorf("creating k8s client: %w", err)
			}

			// 4. Check resources for each model
			var results []versionValidationResult
			for _, m := range models {
				result := checkOperatorResources(cmd.Context(), k8sClient, m, namespace)
				results = append(results, result)
			}

			// 5. Output
			switch format {
			case "json":
				return printValidationJSON(results)
			default:
				printValidationTable(results)
			}

			// 6. Summary and exit status
			passed := 0
			for _, r := range results {
				if r.Status == "PASS" {
					passed++
				}
			}
			fmt.Printf("\nSummary: %d/%d operators match expected state\n", passed, len(results))

			if passed < len(results) {
				return fmt.Errorf("%d operators failed validation", len(results)-passed)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&knowledgeDir, "knowledge-dir", "", "path to versioned knowledge directory (required)")
	_ = cmd.MarkFlagRequired("knowledge-dir")
	cmd.Flags().StringVar(&format, "format", "table", "output format: table or json")

	return cmd
}

// extractVersionPlatform returns the version and platform from the first model
// that has them set.
func extractVersionPlatform(models []*model.OperatorKnowledge) (string, string) {
	var version, platform string
	for _, m := range models {
		if version == "" && m.Operator.Version != "" {
			version = m.Operator.Version
		}
		if platform == "" && m.Operator.Platform != "" {
			platform = m.Operator.Platform
		}
		if version != "" && platform != "" {
			break
		}
	}
	return version, platform
}

// checkOperatorResources checks all managed resources for an operator and
// returns a validation result.
func checkOperatorResources(ctx context.Context, k8sClient client.Client, k *model.OperatorKnowledge, namespace string) versionValidationResult {
	result := versionValidationResult{
		Operator: k.Operator.Name,
		Status:   "PASS",
	}

	for _, comp := range k.Components {
		found := 0
		total := len(comp.ManagedResources)

		for _, mr := range comp.ManagedResources {
			var ns string
			if clusterScopedKinds[mr.Kind] {
				ns = ""
			} else if mr.Namespace != "" {
				ns = mr.Namespace
			} else {
				ns = namespace
			}

			status, _ := checkSingleResource(ctx, k8sClient, mr, ns)
			if status == "Found" {
				found++
			}
		}

		compStatus := "PASS"
		if found < total {
			compStatus = "FAIL"
			result.Status = "FAIL"
		}

		result.Components = append(result.Components, componentValidationResult{
			Component:      comp.Name,
			ResourcesFound: found,
			ResourcesTotal: total,
			Status:         compStatus,
		})
	}

	return result
}

func printValidationTable(results []versionValidationResult) {
	fmt.Println("\n--- Version Validation ---")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "  OPERATOR\tCOMPONENT\tRESOURCES\tSTATUS\n")
	_, _ = fmt.Fprintf(w, "  --------\t---------\t---------\t------\n")
	for _, r := range results {
		for _, c := range r.Components {
			_, _ = fmt.Fprintf(w, "  %s\t%s\t%d/%d\t%s\n",
				r.Operator, c.Component, c.ResourcesFound, c.ResourcesTotal, c.Status)
		}
	}
	_ = w.Flush()
}

func printValidationJSON(results []versionValidationResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}
