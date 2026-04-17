package upgrade

import (
	"fmt"
	"strings"
	"time"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
)

// GenerateUpgradeOpts contains options for generating an upgrade playbook.
type GenerateUpgradeOpts struct {
	SourceDir, TargetDir         string
	SourceVersion, TargetVersion string
	Operator, Namespace          string
	Channels                     []string
}

// GenerateUpgradePlaybook builds an UpgradePlaybook from the given options.
func GenerateUpgradePlaybook(opts GenerateUpgradeOpts) (*PlaybookSpec, error) {
	if opts.SourceVersion == "" {
		return nil, fmt.Errorf("source version is required")
	}
	if opts.TargetVersion == "" {
		return nil, fmt.Errorf("target version is required")
	}
	if opts.Operator == "" {
		return nil, fmt.Errorf("operator is required")
	}

	var hops []Hop
	if len(opts.Channels) > 0 {
		for _, ch := range opts.Channels {
			hops = append(hops, Hop{
				Channel: ch,
				MaxWait: Duration{20 * time.Minute},
			})
		}
	} else {
		hops = []Hop{
			{
				Channel: "# REVIEW: add target channel",
				MaxWait: Duration{20 * time.Minute},
			},
		}
	}

	namespace := opts.Namespace
	if namespace == "" {
		namespace = "openshift-operators"
	}

	steps := []PlaybookStep{
		{
			Name:         "validate-source",
			Type:         "validate-version",
			KnowledgeDir: opts.SourceDir,
		},
		{
			Name: "trigger-upgrade",
			Type: "olm",
		},
		{
			Name:         "validate-target",
			Type:         "validate-version",
			KnowledgeDir: opts.TargetDir,
			DependsOn:    []string{"trigger-upgrade"},
		},
	}

	pb := &PlaybookSpec{
		APIVersion: "chaos.opendatahub.io/v1alpha1",
		Kind:       "UpgradePlaybook",
		Metadata: PlaybookMetadata{
			Name:        fmt.Sprintf("%s-%s-to-%s", opts.Operator, opts.SourceVersion, opts.TargetVersion),
			Description: fmt.Sprintf("# REVIEW: upgrade %s from %s to %s", opts.Operator, opts.SourceVersion, opts.TargetVersion),
		},
		Upgrade: &UpgradeSpec{
			Source: VersionRef{
				KnowledgeDir: opts.SourceDir,
				Version:      opts.SourceVersion,
			},
			Target: VersionRef{
				KnowledgeDir: opts.TargetDir,
				Version:      opts.TargetVersion,
			},
			Paths: []UpgradePath{
				{
					Operator:  opts.Operator,
					Namespace: namespace,
					Hops:      hops,
				},
			},
			Steps: steps,
		},
	}

	return pb, nil
}

// dangerLevel returns the danger level for an experiment filename.
// pod-kill/controller-kill = low, network-partition/config-drift/finalizer-block/crd-mutation = medium,
// webhook-disrupt/rbac-revoke = high.
func dangerLevel(filename string) string {
	lower := strings.ToLower(filename)
	if strings.Contains(lower, "webhook-disrupt") || strings.Contains(lower, "rbac-revoke") {
		return "high"
	}
	if strings.Contains(lower, "network-partition") || strings.Contains(lower, "config-drift") ||
		strings.Contains(lower, "finalizer-block") || strings.Contains(lower, "crd-mutation") {
		return "medium"
	}
	if strings.Contains(lower, "pod-kill") || strings.Contains(lower, "controller-kill") {
		return "low"
	}
	// default to medium for unknown experiments
	return "medium"
}

// filterByDanger returns true if the experiment should be included given the danger filter.
// "all" includes everything, "low" = pod-kill only, "medium" = pod-kill + network/config, "high" = all.
func filterByDanger(filename, dangerFilter string) bool {
	if dangerFilter == "all" {
		return true
	}
	level := dangerLevel(filename)
	switch dangerFilter {
	case "low":
		return level == "low"
	case "medium":
		return level == "low" || level == "medium"
	case "high":
		return true
	}
	return true
}

// GenerateChaosPlaybook builds a ChaosPlaybook from knowledge models and experiments.
func GenerateChaosPlaybook(models []*model.OperatorKnowledge, experiments map[string][]string, knowledgeDir, dangerFilter string) (*PlaybookSpec, error) {
	if len(models) == 0 {
		return nil, fmt.Errorf("at least one knowledge model is required")
	}

	if dangerFilter == "" {
		dangerFilter = "all"
	}

	// Determine namespace from first model
	namespace := "default"
	if len(models) > 0 && models[0].Operator.Namespace != "" {
		namespace = models[0].Operator.Namespace
	}

	var steps []PlaybookStep

	// Preflight step
	steps = append(steps, PlaybookStep{
		Name:         "preflight",
		Type:         "validate-version",
		KnowledgeDir: knowledgeDir,
	})

	// Chaos steps per component, with dependsOn chaining
	prevStep := "preflight"
	for component, expFiles := range experiments {
		var filtered []string
		for _, f := range expFiles {
			if filterByDanger(f, dangerFilter) {
				filtered = append(filtered, f)
			}
		}
		if len(filtered) == 0 {
			continue
		}
		stepName := fmt.Sprintf("chaos-%s", component)
		steps = append(steps, PlaybookStep{
			Name:        stepName,
			Type:        "chaos",
			Experiments: filtered,
			Knowledge:   knowledgeDir,
			DependsOn:   []string{prevStep},
		})
		prevStep = stepName
	}

	// Cleanup step
	steps = append(steps, PlaybookStep{
		Name:     "cleanup",
		Type:     "kubectl",
		Commands: []string{fmt.Sprintf("odh-chaos clean --namespace %s", namespace)},
		DependsOn: []string{prevStep},
	})

	// Build name from operator names
	var operatorNames []string
	for _, m := range models {
		operatorNames = append(operatorNames, m.Operator.Name)
	}
	name := fmt.Sprintf("chaos-%s", strings.Join(operatorNames, "-"))

	pb := &PlaybookSpec{
		APIVersion: "chaos.opendatahub.io/v1alpha1",
		Kind:       "ChaosPlaybook",
		Metadata: PlaybookMetadata{
			Name:        name,
			Description: fmt.Sprintf("# REVIEW: chaos playbook for %s", strings.Join(operatorNames, ", ")),
		},
		Chaos: &ChaosSpec{
			KnowledgeDir: knowledgeDir,
			Steps:        steps,
		},
	}

	return pb, nil
}
