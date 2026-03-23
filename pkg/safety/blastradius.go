package safety

import (
	"fmt"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
)

// ValidateBlastRadius checks that the affected count, namespace, and resource are within blast radius limits.
// When affectedCount <= 0, it is defaulted to 1 to match the injection-type default behavior
// (e.g., PodKill defaults count to 1 when unset). This ensures the blast radius check uses
// the same effective count that the injector will actually apply.
func ValidateBlastRadius(spec v1alpha1.BlastRadiusSpec, targetNamespace string, targetResource string, affectedCount int32) error {
	// Default to 1 when count is zero or negative, matching the injection defaulting
	// in validatePodKillParams. count=0 means "use injection-type default (typically 1)".
	if affectedCount <= 0 {
		affectedCount = 1
	}

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

// CheckDangerLevel returns an error if a high-danger injection is attempted without explicit opt-in.
func CheckDangerLevel(level v1alpha1.DangerLevel, allowDangerous bool) error {
	if level == v1alpha1.DangerLevelHigh && !allowDangerous {
		return fmt.Errorf("injection with dangerLevel=high requires blastRadius.allowDangerous=true")
	}
	return nil
}
