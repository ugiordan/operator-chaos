package upgrade

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

var validStepTypes = map[string]bool{
	"validate-version": true,
	"kubectl":          true,
	"manual":           true,
	"olm":              true,
	"chaos":            true,
}

// LoadPlaybook reads and parses a playbook YAML file (UpgradePlaybook or ChaosPlaybook).
func LoadPlaybook(path string) (*PlaybookSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading playbook %s: %w", path, err)
	}

	var pb PlaybookSpec
	if err := yaml.Unmarshal(data, &pb); err != nil {
		return nil, fmt.Errorf("parsing playbook %s: %w", path, err)
	}

	// Backward compat: convert single Path to Paths slice
	if pb.Upgrade != nil && pb.Upgrade.Path != nil && len(pb.Upgrade.Paths) == 0 {
		pb.Upgrade.Paths = []UpgradePath{*pb.Upgrade.Path}
		pb.Upgrade.Path = nil
	}

	// Set default maxWait for hops across all paths
	if pb.Upgrade != nil {
		for i := range pb.Upgrade.Paths {
			for j := range pb.Upgrade.Paths[i].Hops {
				if pb.Upgrade.Paths[i].Hops[j].MaxWait.Duration == 0 {
					pb.Upgrade.Paths[i].Hops[j].MaxWait = Duration{DefaultMaxWait}
				}
			}
		}
	}

	return &pb, nil
}

// ValidatePlaybook checks a parsed playbook for structural errors.
// Returns a list of human-readable error strings. Empty list means valid.
func ValidatePlaybook(pb *PlaybookSpec) []string {
	var errs []string

	if pb.APIVersion != "" && pb.APIVersion != "chaos.opendatahub.io/v1alpha1" {
		errs = append(errs, fmt.Sprintf("unsupported apiVersion %q (expected chaos.opendatahub.io/v1alpha1)", pb.APIVersion))
	}

	// Common metadata validation
	if pb.Metadata.Name == "" {
		errs = append(errs, "metadata.name is required")
	}
	if pb.Metadata.Description == "" {
		errs = append(errs, "metadata.description is required")
	}

	switch pb.Kind {
	case "UpgradePlaybook", "":
		errs = append(errs, validateUpgrade(pb)...)
	case "ChaosPlaybook":
		errs = append(errs, validateChaos(pb)...)
	default:
		errs = append(errs, fmt.Sprintf("unknown kind %q (expected UpgradePlaybook or ChaosPlaybook)", pb.Kind))
		return errs
	}

	// Shared step validation
	errs = append(errs, validateSteps(pb.Steps())...)

	return errs
}

func validateUpgrade(pb *PlaybookSpec) []string {
	var errs []string

	if pb.Upgrade == nil {
		errs = append(errs, "upgrade spec is required")
		return errs
	}

	if pb.Upgrade.Source.KnowledgeDir == "" {
		errs = append(errs, "upgrade.source.knowledgeDir is required")
	}
	if pb.Upgrade.Source.Version == "" {
		errs = append(errs, "upgrade.source.version is required")
	}
	if pb.Upgrade.Target.KnowledgeDir == "" {
		errs = append(errs, "upgrade.target.knowledgeDir is required")
	}
	if pb.Upgrade.Target.Version == "" {
		errs = append(errs, "upgrade.target.version is required")
	}

	// Validate paths when OLM steps exist
	hasOLMStep := false
	for _, s := range pb.Upgrade.Steps {
		if s.Type == "olm" {
			hasOLMStep = true
			break
		}
	}

	if hasOLMStep {
		if len(pb.Upgrade.Paths) == 0 {
			errs = append(errs, "upgrade.paths is required when using olm steps")
		} else {
			for i, p := range pb.Upgrade.Paths {
				prefix := fmt.Sprintf("upgrade.paths[%d]", i)
				if p.Operator == "" {
					errs = append(errs, fmt.Sprintf("%s.operator is required", prefix))
				}
				if p.Namespace == "" {
					errs = append(errs, fmt.Sprintf("%s.namespace is required", prefix))
				}
				if len(p.Hops) == 0 {
					errs = append(errs, fmt.Sprintf("%s.hops must have at least one entry", prefix))
				}
			}
		}
	}

	// Multi-path: require pathRef on OLM steps
	if len(pb.Upgrade.Paths) > 1 {
		for _, s := range pb.Upgrade.Steps {
			if s.Type == "olm" && s.PathRef == "" {
				errs = append(errs, fmt.Sprintf("step %q: pathRef is required when multiple paths are defined", s.Name))
			}
		}
	}

	if len(pb.Upgrade.Steps) == 0 {
		errs = append(errs, "upgrade.steps must have at least one step")
	}

	return errs
}

func validateChaos(pb *PlaybookSpec) []string {
	var errs []string

	if pb.Chaos == nil {
		errs = append(errs, "chaos spec is required")
		return errs
	}

	if pb.Chaos.KnowledgeDir == "" {
		errs = append(errs, "chaos.knowledgeDir is required")
	}

	if len(pb.Chaos.Steps) == 0 {
		errs = append(errs, "chaos.steps must have at least one step")
	}

	return errs
}

func validateSteps(steps []PlaybookStep) []string {
	var errs []string
	seen := map[string]bool{}

	for i, s := range steps {
		if s.Name == "" {
			errs = append(errs, fmt.Sprintf("step %d: name is required", i))
			continue
		}
		if seen[s.Name] {
			errs = append(errs, fmt.Sprintf("duplicate step name: %q", s.Name))
		}
		seen[s.Name] = true

		if !validStepTypes[s.Type] {
			errs = append(errs, fmt.Sprintf("step %q: unknown step type %q", s.Name, s.Type))
		}

		// Validate dependsOn references
		for _, dep := range s.DependsOn {
			if !seen[dep] {
				// Check if it exists later in the list (forward ref, still invalid ordering)
				found := false
				for _, other := range steps {
					if other.Name == dep {
						found = true
						break
					}
				}
				if !found {
					errs = append(errs, fmt.Sprintf("step %q: dependsOn references non-existent step %q", s.Name, dep))
				}
			}
		}
	}

	return errs
}

// HasShellCommands returns true if the playbook contains kubectl steps with raw commands.
func HasShellCommands(pb *PlaybookSpec) bool {
	for _, s := range pb.Steps() {
		if s.Type == "kubectl" && len(s.Commands) > 0 {
			return true
		}
	}
	return false
}

// ResolveKnowledgeDir returns the effective knowledge directory for a step,
// defaulting to source (before OLM step) or target (after OLM step).
func ResolveKnowledgeDir(step PlaybookStep, pb *PlaybookSpec) string {
	if step.KnowledgeDir != "" {
		return step.KnowledgeDir
	}

	// For chaos playbooks, use the top-level knowledgeDir
	if pb.Chaos != nil {
		return pb.Chaos.KnowledgeDir
	}

	if pb.Upgrade == nil {
		return ""
	}

	allSteps := pb.Steps()

	// Find first OLM step and last OLM step positions, plus the target step
	firstOLM := -1
	lastOLM := -1
	stepIndex := -1
	for i, s := range allSteps {
		if s.Type == "olm" {
			if firstOLM < 0 {
				firstOLM = i
			}
			lastOLM = i
		}
		if s.Name == step.Name {
			stepIndex = i
		}
	}

	// No OLM step: default to target (post-upgrade knowledge)
	if firstOLM < 0 {
		return pb.Upgrade.Target.KnowledgeDir
	}

	// After the last OLM step: use target
	if stepIndex > lastOLM {
		return pb.Upgrade.Target.KnowledgeDir
	}

	// Before the first OLM step: use source
	if stepIndex < firstOLM {
		return pb.Upgrade.Source.KnowledgeDir
	}

	// Between first and last OLM (or is an OLM step): use target
	return pb.Upgrade.Target.KnowledgeDir
}
