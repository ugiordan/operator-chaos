package diff

import (
	"fmt"
	"sort"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
)

// ComputeDiff compares two sets of OperatorKnowledge (source and target versions)
// and produces an UpgradeDiff describing all changes between them.
func ComputeDiff(source, target []*model.OperatorKnowledge) *UpgradeDiff {
	srcVersion, srcPlatform := extractVersionInfo(source)
	tgtVersion, _ := extractVersionInfo(target)

	srcByOp := groupByOperator(source)
	tgtByOp := groupByOperator(target)

	var components []ComponentDiff

	// For each operator in source, check if it exists in target
	for opName, srcKnowledge := range srcByOp {
		tgtKnowledge, exists := tgtByOp[opName]
		if !exists {
			// All components removed
			for _, comp := range srcKnowledge.Components {
				components = append(components, ComponentDiff{
					Operator:   opName,
					Component:  comp.Name,
					ChangeType: ComponentRemoved,
				})
			}
			continue
		}
		// Compute detailed diffs
		opDiffs := diffOperator(opName, srcKnowledge, tgtKnowledge)
		components = append(components, opDiffs...)
	}

	// For each operator only in target, all components are added
	for opName, tgtKnowledge := range tgtByOp {
		if _, exists := srcByOp[opName]; exists {
			continue
		}
		for _, comp := range tgtKnowledge.Components {
			components = append(components, ComponentDiff{
				Operator:   opName,
				Component:  comp.Name,
				ChangeType: ComponentAdded,
			})
		}
	}

	// Sort for deterministic output
	sort.Slice(components, func(i, j int) bool {
		if components[i].Operator != components[j].Operator {
			return components[i].Operator < components[j].Operator
		}
		return components[i].Component < components[j].Component
	})

	return &UpgradeDiff{
		SourceVersion: srcVersion,
		TargetVersion: tgtVersion,
		Platform:      srcPlatform,
		Components:    components,
		Summary:       computeSummary(components),
	}
}

// extractVersionInfo returns the version and platform from the first knowledge
// entry that has them set.
func extractVersionInfo(knowledge []*model.OperatorKnowledge) (version, platform string) {
	for _, k := range knowledge {
		if k.Operator.Version != "" && version == "" {
			version = k.Operator.Version
		}
		if k.Operator.Platform != "" && platform == "" {
			platform = k.Operator.Platform
		}
		if version != "" && platform != "" {
			return
		}
	}
	return
}

// groupByOperator indexes knowledge entries by operator name.
// If multiple entries share the same operator name, their components are merged.
func groupByOperator(knowledge []*model.OperatorKnowledge) map[string]*model.OperatorKnowledge {
	result := make(map[string]*model.OperatorKnowledge)
	for _, k := range knowledge {
		name := k.Operator.Name
		existing, exists := result[name]
		if !exists {
			// Copy to avoid mutating the input
			copy := *k
			result[name] = &copy
		} else {
			existing.Components = append(existing.Components, k.Components...)
		}
	}
	return result
}

// diffOperator computes component-level diffs between source and target knowledge
// for a single operator.
func diffOperator(operator string, source, target *model.OperatorKnowledge) []ComponentDiff {
	matches, added, removed := matchComponents(operator, source.Components, target.Components)

	var diffs []ComponentDiff

	for _, m := range matches {
		cd := diffComponent(source.Operator, target.Operator, m)
		cd.Operator = operator
		diffs = append(diffs, cd)
	}

	for _, comp := range removed {
		diffs = append(diffs, ComponentDiff{
			Operator:   operator,
			Component:  comp.Name,
			ChangeType: ComponentRemoved,
		})
	}

	for _, comp := range added {
		diffs = append(diffs, ComponentDiff{
			Operator:   operator,
			Component:  comp.Name,
			ChangeType: ComponentAdded,
		})
	}

	return diffs
}

// diffComponent produces a ComponentDiff from a matched pair of components,
// detecting renames, namespace changes, and sub-resource diffs.
func diffComponent(sourceOp, targetOp model.OperatorMeta, m componentMatch) ComponentDiff {
	cd := ComponentDiff{
		Component: m.target.Name,
	}

	if m.renamed {
		cd.ChangeType = ComponentRenamed
		cd.RenamedFrom = m.source.Name
	} else {
		cd.ChangeType = ComponentModified
	}

	// Namespace change at operator level
	if sourceOp.Namespace != targetOp.Namespace {
		cd.NamespaceChange = &NamespaceChange{
			From: sourceOp.Namespace,
			To:   targetOp.Namespace,
		}
	}

	cd.ResourceDiffs = diffResources(m.source.ManagedResources, m.target.ManagedResources)
	cd.WebhookDiffs = diffWebhooks(m.source.Webhooks, m.target.Webhooks)
	cd.FinalizerDiffs = diffFinalizers(m.source.Finalizers, m.target.Finalizers)
	cd.DependencyDiffs = diffDependencies(m.source.Dependencies, m.target.Dependencies)

	return cd
}

// diffResources computes resource-level diffs between source and target managed resources.
func diffResources(source, target []model.ManagedResource) []ResourceDiff {
	matches, added, removed := matchResources(source, target)

	var diffs []ResourceDiff

	for _, m := range matches {
		rd := ResourceDiff{
			Kind: m.target.Kind,
			Name: m.target.Name,
		}

		if m.renamed {
			rd.ChangeType = ResourceRenamed
			rd.RenamedFrom = m.source.Name
		} else if m.source.Namespace != m.target.Namespace && m.source.Namespace != "" && m.target.Namespace != "" {
			rd.ChangeType = ResourceMoved
			rd.MovedFrom = m.source.Namespace
		} else {
			rd.ChangeType = ResourceModified
		}

		rd.FieldChanges = diffResourceFields(m.source, m.target)

		// Only include if there are actual changes
		if rd.ChangeType != ResourceModified || len(rd.FieldChanges) > 0 {
			diffs = append(diffs, rd)
		}
	}

	for _, r := range removed {
		diffs = append(diffs, ResourceDiff{
			Kind:       r.Kind,
			Name:       r.Name,
			ChangeType: ResourceRemoved,
		})
	}

	for _, r := range added {
		diffs = append(diffs, ResourceDiff{
			Kind:       r.Kind,
			Name:       r.Name,
			ChangeType: ResourceAdded,
		})
	}

	return diffs
}

// diffResourceFields compares Labels and ExpectedSpec between two matched resources.
func diffResourceFields(source, target model.ManagedResource) []FieldChange {
	var changes []FieldChange

	// Compare labels
	allLabelKeys := make(map[string]bool)
	for k := range source.Labels {
		allLabelKeys[k] = true
	}
	for k := range target.Labels {
		allLabelKeys[k] = true
	}

	sortedKeys := sortedMapKeys(allLabelKeys)
	for _, k := range sortedKeys {
		sv := source.Labels[k]
		tv := target.Labels[k]
		if sv != tv {
			changes = append(changes, FieldChange{
				Path:     fmt.Sprintf("labels.%s", k),
				OldValue: sv,
				NewValue: tv,
			})
		}
	}

	// Compare ExpectedSpec keys
	allSpecKeys := make(map[string]bool)
	for k := range source.ExpectedSpec {
		allSpecKeys[k] = true
	}
	for k := range target.ExpectedSpec {
		allSpecKeys[k] = true
	}

	sortedSpecKeys := sortedMapKeys(allSpecKeys)
	for _, k := range sortedSpecKeys {
		sv, sOk := source.ExpectedSpec[k]
		tv, tOk := target.ExpectedSpec[k]
		if !sOk {
			changes = append(changes, FieldChange{
				Path:     fmt.Sprintf("spec.%s", k),
				OldValue: "",
				NewValue: fmt.Sprintf("%v", tv),
			})
		} else if !tOk {
			changes = append(changes, FieldChange{
				Path:     fmt.Sprintf("spec.%s", k),
				OldValue: fmt.Sprintf("%v", sv),
				NewValue: "",
			})
		} else if fmt.Sprintf("%v", sv) != fmt.Sprintf("%v", tv) {
			changes = append(changes, FieldChange{
				Path:     fmt.Sprintf("spec.%s", k),
				OldValue: fmt.Sprintf("%v", sv),
				NewValue: fmt.Sprintf("%v", tv),
			})
		}
	}

	return changes
}

// diffWebhooks compares webhooks by name, detecting type and path changes.
func diffWebhooks(source, target []model.WebhookSpec) []WebhookDiff {
	srcByName := make(map[string]model.WebhookSpec)
	tgtByName := make(map[string]model.WebhookSpec)

	for _, w := range source {
		srcByName[w.Name] = w
	}
	for _, w := range target {
		tgtByName[w.Name] = w
	}

	var diffs []WebhookDiff

	// Check source webhooks
	for name, sw := range srcByName {
		tw, exists := tgtByName[name]
		if !exists {
			diffs = append(diffs, WebhookDiff{
				Name:       name,
				ChangeType: DiffRemoved,
			})
			continue
		}
		// Check for changes
		if sw.Type != tw.Type || sw.Path != tw.Path {
			wd := WebhookDiff{
				Name:       name,
				ChangeType: DiffModified,
			}
			if sw.Type != tw.Type {
				wd.OldType = sw.Type
				wd.NewType = tw.Type
			}
			if sw.Path != tw.Path {
				wd.OldPath = sw.Path
				wd.NewPath = tw.Path
			}
			diffs = append(diffs, wd)
		}
	}

	// Check for added webhooks
	for name := range tgtByName {
		if _, exists := srcByName[name]; !exists {
			diffs = append(diffs, WebhookDiff{
				Name:       name,
				ChangeType: DiffAdded,
			})
		}
	}

	sort.Slice(diffs, func(i, j int) bool {
		return diffs[i].Name < diffs[j].Name
	})

	return diffs
}

// diffFinalizers computes set differences between source and target finalizer lists.
func diffFinalizers(source, target []string) []FinalizerDiff {
	srcSet := toSet(source)
	tgtSet := toSet(target)

	var diffs []FinalizerDiff

	for _, f := range source {
		if !tgtSet[f] {
			diffs = append(diffs, FinalizerDiff{
				Finalizer:  f,
				ChangeType: DiffRemoved,
			})
		}
	}

	for _, f := range target {
		if !srcSet[f] {
			diffs = append(diffs, FinalizerDiff{
				Finalizer:  f,
				ChangeType: DiffAdded,
			})
		}
	}

	return diffs
}

// diffDependencies computes set differences between source and target dependency lists.
func diffDependencies(source, target []string) []DependencyDiff {
	srcSet := toSet(source)
	tgtSet := toSet(target)

	var diffs []DependencyDiff

	for _, d := range source {
		if !tgtSet[d] {
			diffs = append(diffs, DependencyDiff{
				Dependency: d,
				ChangeType: DiffRemoved,
			})
		}
	}

	for _, d := range target {
		if !srcSet[d] {
			diffs = append(diffs, DependencyDiff{
				Dependency: d,
				ChangeType: DiffAdded,
			})
		}
	}

	return diffs
}

// computeSummary tallies all changes across component diffs.
func computeSummary(components []ComponentDiff) DiffSummary {
	var s DiffSummary

	for i := range components {
		cd := &components[i]
		switch cd.ChangeType {
		case ComponentAdded:
			s.ComponentsAdded++
		case ComponentRemoved:
			s.ComponentsRemoved++
		case ComponentRenamed:
			s.ComponentsRenamed++
		}

		if cd.NamespaceChange != nil {
			s.NamespaceMoves++
		}

		s.ResourceChanges += len(cd.ResourceDiffs)
		s.WebhookChanges += len(cd.WebhookDiffs)
		s.FinalizerChanges += len(cd.FinalizerDiffs)
		s.DependencyChanges += len(cd.DependencyDiffs)

		if cd.IsBreaking() {
			s.BreakingChanges++
		}
	}

	return s
}

// toSet converts a string slice to a set map.
func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}

// sortedMapKeys returns sorted keys from a map[string]bool.
func sortedMapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
