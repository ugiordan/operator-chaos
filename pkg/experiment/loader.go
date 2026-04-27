package experiment

import (
	"fmt"
	"os"

	v1alpha1 "github.com/opendatahub-io/operator-chaos/api/v1alpha1"
	"github.com/opendatahub-io/operator-chaos/pkg/injection"
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
	if err := yaml.UnmarshalStrict(data, &exp); err != nil {
		return nil, fmt.Errorf("parsing experiment file %s: %w", path, err)
	}

	return &exp, nil
}

// Validate checks that all required fields in a ChaosExperiment are populated.
// Returns a slice of human-readable error messages; an empty slice means valid.
func Validate(exp *v1alpha1.ChaosExperiment) []string {
	var errs []string

	if exp.Name == "" {
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
	} else if err := v1alpha1.ValidateInjectionType(exp.Spec.Injection.Type); err != nil {
		errs = append(errs, err.Error())
	} else if err := injection.ValidateInjectionParams(exp.Spec.Injection, exp.Spec.BlastRadius); err != nil {
		errs = append(errs, err.Error())
	}
	if err := v1alpha1.ValidateDangerLevel(exp.Spec.Injection.DangerLevel); err != nil {
		errs = append(errs, err.Error())
	}
	if exp.Spec.Hypothesis.Description == "" {
		errs = append(errs, "spec.hypothesis.description is required")
	}
	if exp.Spec.BlastRadius.MaxPodsAffected <= 0 {
		errs = append(errs, "spec.blastRadius.maxPodsAffected must be greater than 0")
	}
	if exp.Spec.Tier != 0 && (exp.Spec.Tier < v1alpha1.MinTier || exp.Spec.Tier > v1alpha1.MaxTier) {
		errs = append(errs, fmt.Sprintf("spec.tier must be between %d and %d, got %d", v1alpha1.MinTier, v1alpha1.MaxTier, exp.Spec.Tier))
	}

	// Validate TTL and recoveryTimeout bounds
	if exp.Spec.Injection.TTL.Duration < 0 {
		errs = append(errs, "spec.injection.ttl must not be negative")
	} else if exp.Spec.Injection.TTL.Duration > v1alpha1.MaxInjectionTTL {
		errs = append(errs, fmt.Sprintf("spec.injection.ttl exceeds maximum of %s", v1alpha1.MaxInjectionTTL))
	}
	if exp.Spec.Hypothesis.RecoveryTimeout.Duration < 0 {
		errs = append(errs, "spec.hypothesis.recoveryTimeout must not be negative")
	} else if exp.Spec.Hypothesis.RecoveryTimeout.Duration > v1alpha1.MaxRecoveryTimeout {
		errs = append(errs, fmt.Sprintf("spec.hypothesis.recoveryTimeout exceeds maximum of %s", v1alpha1.MaxRecoveryTimeout))
	}

	// Validate target spec K8s names
	if exp.Spec.Target.Operator != "" {
		if err := injection.ValidateTargetSpec(exp.Spec.Target); err != nil {
			errs = append(errs, err.Error())
		}
	}

	return errs
}
