package model

import "fmt"

// ValidateKnowledge checks that all required fields in an OperatorKnowledge
// are populated and semantically correct. Returns a slice of human-readable
// error messages; an empty slice means valid.
func ValidateKnowledge(k *OperatorKnowledge) []string {
	if k == nil {
		return []string{"knowledge must not be nil"}
	}

	var errs []string

	if k.Operator.Name == "" {
		errs = append(errs, "operator.name is required")
	}
	if k.Operator.Namespace == "" {
		errs = append(errs, "operator.namespace is required")
	}

	if len(k.Components) == 0 {
		errs = append(errs, "at least one component is required")
	}

	componentNames := make(map[string]bool)
	for i, comp := range k.Components {
		prefix := fmt.Sprintf("components[%d]", i)

		if comp.Name == "" {
			errs = append(errs, prefix+".name is required")
		} else {
			if componentNames[comp.Name] {
				errs = append(errs, fmt.Sprintf("components[%d]: duplicate component name %q", i, comp.Name))
			}
			componentNames[comp.Name] = true
		}

		if comp.Controller == "" {
			errs = append(errs, prefix+".controller is required")
		}

		if len(comp.ManagedResources) == 0 {
			errs = append(errs, prefix+" must have at least one managedResource")
		}

		resourceNames := make(map[string]bool)
		for j, mr := range comp.ManagedResources {
			mrPrefix := fmt.Sprintf("%s.managedResources[%d]", prefix, j)

			if mr.APIVersion == "" {
				errs = append(errs, mrPrefix+".apiVersion is required")
			}
			if mr.Kind == "" {
				errs = append(errs, mrPrefix+".kind is required")
			}
			if mr.Name == "" {
				errs = append(errs, mrPrefix+".name is required")
			} else {
				if resourceNames[mr.Name] {
					errs = append(errs, fmt.Sprintf("%s: duplicate managedResource name %q", prefix, mr.Name))
				}
				resourceNames[mr.Name] = true
			}
		}

		for j, wh := range comp.Webhooks {
			whPrefix := fmt.Sprintf("%s.webhooks[%d]", prefix, j)

			if wh.Name == "" {
				errs = append(errs, whPrefix+".name is required")
			}
			if wh.Type == "" {
				errs = append(errs, whPrefix+".type is required")
			} else if wh.Type != "validating" && wh.Type != "mutating" {
				errs = append(errs, fmt.Sprintf("%s.type must be \"validating\" or \"mutating\", got %q", whPrefix, wh.Type))
			}
			if wh.Path == "" {
				errs = append(errs, whPrefix+".path is required")
			}
		}
	}

	// Validate dependency references point to known component names.
	for i, comp := range k.Components {
		for _, dep := range comp.Dependencies {
			if !componentNames[dep] {
				errs = append(errs, fmt.Sprintf("components[%d].dependencies references unknown component %q", i, dep))
			}
		}
	}

	if k.Recovery.ReconcileTimeout.Duration <= 0 {
		errs = append(errs, "recovery.reconcileTimeout must be greater than 0")
	}
	if k.Recovery.MaxReconcileCycles <= 0 {
		errs = append(errs, "recovery.maxReconcileCycles must be greater than 0")
	}

	return errs
}
