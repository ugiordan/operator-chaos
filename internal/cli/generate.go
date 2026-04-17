package cli

import (
	"bytes"
	"context"
	"fmt"
	"go/format"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"

	upgrade "github.com/opendatahub-io/odh-platform-chaos/internal/cli/upgrade"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/olm"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	syaml "sigs.k8s.io/yaml"
)

func newGenerateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate code from knowledge models",
	}

	cmd.AddCommand(newGenerateFuzzTargetsCommand())
	cmd.AddCommand(newGenerateUpgradeCommand())
	cmd.AddCommand(newGenerateChaosCommand())

	return cmd
}

func newGenerateFuzzTargetsCommand() *cobra.Command {
	var knowledgePath string
	var outputPath string

	cmd := &cobra.Command{
		Use:   "fuzz-targets",
		Short: "Generate fuzz test targets from a knowledge model",
		RunE: func(cmd *cobra.Command, args []string) error {
			k, err := model.LoadKnowledge(knowledgePath)
			if err != nil {
				return fmt.Errorf("loading knowledge: %w", err)
			}

			errs := model.ValidateKnowledge(k)
			if len(errs) > 0 {
				return fmt.Errorf("knowledge validation failed: %v", errs[0])
			}

			output, err := renderFuzzTargets(k)
			if err != nil {
				return fmt.Errorf("rendering template: %w", err)
			}

			if outputPath == "" {
				fmt.Print(output)
				return nil
			}

			return os.WriteFile(outputPath, []byte(output), 0644)
		},
	}

	cmd.Flags().StringVar(&knowledgePath, "knowledge", "", "path to knowledge YAML file (required)")
	cmd.Flags().StringVar(&outputPath, "output", "", "output file path (defaults to stdout)")
	_ = cmd.MarkFlagRequired("knowledge")

	return cmd
}

type templateData struct {
	OperatorName string
	Components   []templateComponent
}

type templateComponent struct {
	FuncName    string
	Name        string
	SeedObjects string // Go code for seed object slice elements
	Request     string // Go code for reconcile.Request
	Invariants  string // Go code for invariant additions
	SeedCorpus  string // Go code for f.Add() calls
}

func renderFuzzTargets(k *model.OperatorKnowledge) (string, error) {
	data := templateData{
		OperatorName: k.Operator.Name,
	}

	corpusEntries := model.SeedCorpusEntries(k)

	for _, comp := range k.Components {
		tc := templateComponent{
			FuncName: "Fuzz" + pascalCase(comp.Name),
			Name:     comp.Name,
		}

		// Find request target (prefer Deployment, then DaemonSet, then first resource)
		for _, mr := range comp.ManagedResources {
			if mr.Kind == "Deployment" || mr.Kind == "DaemonSet" {
				tc.Request = fmt.Sprintf(
					`reconcile.Request{NamespacedName: types.NamespacedName{Name: %q, Namespace: %q}}`,
					mr.Name, mr.Namespace,
				)
				break
			}
		}
		if tc.Request == "" && len(comp.ManagedResources) > 0 {
			mr := comp.ManagedResources[0]
			tc.Request = fmt.Sprintf(
				`reconcile.Request{NamespacedName: types.NamespacedName{Name: %q, Namespace: %q}}`,
				mr.Name, mr.Namespace,
			)
		}

		// Seed objects: one per managed resource
		var seedLines []string
		for _, mr := range comp.ManagedResources {
			seedLines = append(seedLines, model.SeedObjectCode(mr)+",")
		}
		tc.SeedObjects = strings.Join(seedLines, "\n\t\t")

		// Invariants: from steady-state checks + Deployment replicas
		seen := make(map[string]bool)
		var invLines []string
		for _, check := range comp.SteadyState.Checks {
			key := check.Kind + "/" + check.Namespace + "/" + check.Name
			if !seen[key] {
				seen[key] = true
				invLines = append(invLines, model.InvariantCode(check.Kind, check.Name, check.Namespace))
			}
		}
		for _, mr := range comp.ManagedResources {
			if mr.Kind == "Deployment" {
				if _, ok := mr.ExpectedSpec["replicas"]; ok {
					key := "Deployment/" + mr.Namespace + "/" + mr.Name
					if !seen[key] {
						seen[key] = true
						invLines = append(invLines, model.InvariantCode("Deployment", mr.Name, mr.Namespace))
					}
				}
			}
		}
		tc.Invariants = strings.Join(invLines, "\n\t\t\t")

		// Seed corpus entries (operator-level, same for all components)
		var corpusLines []string
		for _, e := range corpusEntries {
			corpusLines = append(corpusLines, fmt.Sprintf(
				"f.Add(uint16(%#04x), uint8(%d), uint16(%d)) // %s",
				e.OpMask, e.FaultType, e.Intensity, e.Label,
			))
		}
		tc.SeedCorpus = strings.Join(corpusLines, "\n\t")

		data.Components = append(data.Components, tc)
	}

	tmpl, err := template.New("fuzz").Parse(fuzzTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return buf.String(), nil
	}

	return string(formatted), nil
}

// pascalCase converts "kserve-controller-manager" to "KserveControllerManager".
func pascalCase(s string) string {
	var result strings.Builder
	upper := true
	for _, r := range s {
		if r == '-' || r == '_' || r == '.' {
			upper = true
			continue
		}
		if upper {
			result.WriteRune(unicode.ToUpper(r))
			upper = false
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

const fuzzTemplate = `// Code generated from knowledge model: {{.OperatorName}}. DO NOT EDIT (except reconcilerFactory).

package fuzz_test

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/sdk/fuzz"
)

// reconcilerFactory creates a reconciler wired to the given client.
// Replace this with your actual reconciler constructor.
func reconcilerFactory(c client.Client) reconcile.Reconciler {
	panic("replace with your reconciler: e.g. return &MyReconciler{client: c}")
}

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = appsv1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = coordinationv1.AddToScheme(s)
	_ = rbacv1.AddToScheme(s)
	return s
}
{{range .Components}}
func {{.FuncName}}(f *testing.F) {
	scheme := testScheme()
	req := {{.Request}}

	seeds := []client.Object{
		{{.SeedObjects}}
	}

	{{.SeedCorpus}}

	f.Fuzz(func(t *testing.T, opMask uint16, faultType uint8, intensity uint16) {
		h := fuzz.NewHarness(reconcilerFactory, scheme, req, seeds...)
		{{.Invariants}}
		fc := fuzz.DecodeFaultConfig(opMask, faultType, intensity)
		if err := h.Run(t, fc); err != nil {
			t.Fatal(err)
		}
	})
}
{{end}}`

func newGenerateUpgradeCommand() *cobra.Command {
	var (
		source    string
		target    string
		operator  string
		namespace string
		output    string
		discover  bool
	)

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Generate an upgrade playbook from knowledge directories",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load source knowledge to get version
			sourceModels, err := model.LoadKnowledgeDir(source)
			if err != nil {
				return fmt.Errorf("loading source knowledge: %w", err)
			}
			if len(sourceModels) == 0 {
				return fmt.Errorf("no knowledge models found in %s", source)
			}

			// Load target knowledge to get version
			targetModels, err := model.LoadKnowledgeDir(target)
			if err != nil {
				return fmt.Errorf("loading target knowledge: %w", err)
			}
			if len(targetModels) == 0 {
				return fmt.Errorf("no knowledge models found in %s", target)
			}

			sourceVersion := sourceModels[0].Operator.Version
			targetVersion := targetModels[0].Operator.Version

			if operator == "" {
				operator = sourceModels[0].Operator.Name
			}
			if namespace == "" {
				namespace = sourceModels[0].Operator.Namespace
			}

			var channels []string
			if discover {
				channels, _ = discoverChannels(operator, namespace)
			}

			opts := upgrade.GenerateUpgradeOpts{
				SourceDir:     source,
				TargetDir:     target,
				SourceVersion: sourceVersion,
				TargetVersion: targetVersion,
				Operator:      operator,
				Namespace:     namespace,
				Channels:      channels,
			}

			pb, err := upgrade.GenerateUpgradePlaybook(opts)
			if err != nil {
				return fmt.Errorf("generating playbook: %w", err)
			}

			data, err := syaml.Marshal(pb)
			if err != nil {
				return fmt.Errorf("marshaling playbook: %w", err)
			}

			if output == "" {
				fmt.Print(string(data))
				return nil
			}

			return os.WriteFile(output, data, 0644)
		},
	}

	cmd.Flags().StringVar(&source, "source", "", "source knowledge directory (required)")
	cmd.Flags().StringVar(&target, "target", "", "target knowledge directory (required)")
	cmd.Flags().StringVar(&operator, "operator", "", "operator name (defaults to first model's operator)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "operator namespace (defaults to first model's namespace)")
	cmd.Flags().StringVar(&output, "output", "", "output file path (defaults to stdout)")
	cmd.Flags().BoolVar(&discover, "discover", true, "try to discover OLM channels from cluster")
	_ = cmd.MarkFlagRequired("source")
	_ = cmd.MarkFlagRequired("target")

	return cmd
}

// discoverChannels attempts to connect to a cluster and discover OLM channels.
// Returns nil on any error (best-effort).
func discoverChannels(operator, namespace string) ([]string, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	c, err := client.New(cfg, client.Options{})
	if err != nil {
		return nil, err
	}

	olmClient := olm.NewClient(c, log.Default())
	infos, err := olmClient.Discover(context.Background(), operator, namespace)
	if err != nil {
		return nil, err
	}

	var channels []string
	for _, info := range infos {
		channels = append(channels, info.Name)
	}
	return channels, nil
}

func newGenerateChaosCommand() *cobra.Command {
	var (
		knowledge      string
		experimentsDir string
		danger         string
		output         string
	)

	cmd := &cobra.Command{
		Use:   "chaos",
		Short: "Generate a chaos playbook from knowledge models and experiments",
		RunE: func(cmd *cobra.Command, args []string) error {
			models, err := model.LoadKnowledgeDir(knowledge)
			if err != nil {
				return fmt.Errorf("loading knowledge: %w", err)
			}
			if len(models) == 0 {
				return fmt.Errorf("no knowledge models found in %s", knowledge)
			}

			experiments := scanExperiments(experimentsDir, models)

			pb, err := upgrade.GenerateChaosPlaybook(models, experiments, knowledge, danger)
			if err != nil {
				return fmt.Errorf("generating playbook: %w", err)
			}

			data, err := syaml.Marshal(pb)
			if err != nil {
				return fmt.Errorf("marshaling playbook: %w", err)
			}

			if output == "" {
				fmt.Print(string(data))
				return nil
			}

			return os.WriteFile(output, data, 0644)
		},
	}

	cmd.Flags().StringVar(&knowledge, "knowledge", "", "knowledge directory (required)")
	cmd.Flags().StringVar(&experimentsDir, "experiments", "", "experiments directory (required)")
	cmd.Flags().StringVar(&danger, "danger", "all", "danger filter: all, low, medium, high")
	cmd.Flags().StringVar(&output, "output", "", "output file path (defaults to stdout)")
	_ = cmd.MarkFlagRequired("knowledge")
	_ = cmd.MarkFlagRequired("experiments")

	return cmd
}

// scanExperiments scans the experiments directory for YAML files organized by component.
// Convention: experiments/{component-name}/*.yaml
func scanExperiments(dir string, models []*model.OperatorKnowledge) map[string][]string {
	result := make(map[string][]string)

	// Build set of known component names
	componentNames := make(map[string]bool)
	for _, m := range models {
		for _, c := range m.Components {
			componentNames[c.Name] = true
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return result
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		compName := entry.Name()
		if !componentNames[compName] {
			continue
		}

		compDir := filepath.Join(dir, compName)
		files, err := os.ReadDir(compDir)
		if err != nil {
			continue
		}

		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
				result[compName] = append(result[compName], filepath.Join(compDir, name))
			}
		}
	}

	return result
}
