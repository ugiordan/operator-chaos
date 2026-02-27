package experiment

import (
	"fmt"
	"os"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"sigs.k8s.io/yaml"
)

const maxExperimentFileSize = 1 * 1024 * 1024 // 1 MB

// Load reads and parses a ChaosExperiment YAML file from the given path.
func Load(path string) (*v1alpha1.ChaosExperiment, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Size() > maxExperimentFileSize {
		return nil, fmt.Errorf("file %s (%d bytes) exceeds maximum size of %d bytes", path, info.Size(), maxExperimentFileSize)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading experiment file %s: %w", path, err)
	}

	var exp v1alpha1.ChaosExperiment
	if err := yaml.Unmarshal(data, &exp); err != nil {
		return nil, fmt.Errorf("parsing experiment file %s: %w", path, err)
	}

	return &exp, nil
}

// Validate checks that all required fields in a ChaosExperiment are populated.
// Returns a slice of human-readable error messages; an empty slice means valid.
func Validate(exp *v1alpha1.ChaosExperiment) []string {
	var errs []string

	if exp.Metadata.Name == "" {
		errs = append(errs, "metadata.name is required")
	}
	if exp.Spec.Target.Operator == "" {
		errs = append(errs, "spec.target.operator is required")
	}
	if exp.Spec.Target.Component == "" {
		errs = append(errs, "spec.target.component is required")
	}
	if exp.Spec.Injection.Type == "" {
		errs = append(errs, "spec.injection.type is required")
	}
	if exp.Spec.Hypothesis.Description == "" {
		errs = append(errs, "spec.hypothesis.description is required")
	}
	if len(exp.Spec.BlastRadius.AllowedNamespaces) == 0 {
		errs = append(errs, "spec.blastRadius.allowedNamespaces must not be empty")
	}

	return errs
}
