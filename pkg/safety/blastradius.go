package safety

import (
	"fmt"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
)

func ValidateBlastRadius(spec v1alpha1.BlastRadiusSpec, targetNamespace string, targetResource string, affectedCount int) error {
	if spec.MaxPodsAffected <= 0 {
		return fmt.Errorf("maxPodsAffected must be > 0, got %d", spec.MaxPodsAffected)
	}

	if affectedCount > spec.MaxPodsAffected {
		return fmt.Errorf("blast radius exceeded: %d affected > %d max", affectedCount, spec.MaxPodsAffected)
	}

	allowed := false
	for _, ns := range spec.AllowedNamespaces {
		if ns == targetNamespace {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("namespace %q not in allowed list %v", targetNamespace, spec.AllowedNamespaces)
	}

	for _, forbidden := range spec.ForbiddenResources {
		if forbidden == targetResource {
			return fmt.Errorf("resource %q is forbidden", targetResource)
		}
	}

	return nil
}

func CheckDangerLevel(level string, allowDangerous bool) error {
	if level == "high" && !allowDangerous {
		return fmt.Errorf("injection with dangerLevel=high requires blastRadius.allowDangerous=true")
	}
	return nil
}
