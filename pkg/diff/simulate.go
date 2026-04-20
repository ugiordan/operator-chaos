package diff

import (
	"fmt"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/opendatahub-io/operator-chaos/api/v1alpha1"
	"github.com/opendatahub-io/operator-chaos/pkg/model"
)

// GenerateUpgradeExperiments maps an UpgradeDiff to ChaosExperiment objects
// that simulate the effects of each detected change. Each breaking change
// produces one or more experiments using existing injection types.
func GenerateUpgradeExperiments(
	diff *UpgradeDiff,
	sourceKnowledge []*model.OperatorKnowledge,
	targetKnowledge []*model.OperatorKnowledge,
) []v1alpha1.ChaosExperiment {
	srcByOp := groupByOperator(sourceKnowledge)
	tgtByOp := groupByOperator(targetKnowledge)

	var experiments []v1alpha1.ChaosExperiment
	nameCount := make(map[string]int)

	addExperiment := func(exp *v1alpha1.ChaosExperiment) {
		if exp == nil {
			return
		}
		nameCount[exp.Name]++
		if nameCount[exp.Name] > 1 {
			exp.Name = fmt.Sprintf("%s-%d", exp.Name, nameCount[exp.Name])
		}
		experiments = append(experiments, *exp)
	}

	for _, cd := range diff.Components {
		srcK := srcByOp[cd.Operator]
		tgtK := tgtByOp[cd.Operator]

		if cd.ChangeType == ComponentRenamed {
			addExperiment(generateRenameExperiment(cd, diff, srcK, tgtK))
		}

		if cd.NamespaceChange != nil {
			addExperiment(generateNamespaceMoveExperiment(cd, diff, srcK))
		}

		for _, wd := range cd.WebhookDiffs {
			addExperiment(generateWebhookExperiment(cd, diff, wd))
		}

		for _, dd := range cd.DependencyDiffs {
			if dd.ChangeType == DiffAdded {
				addExperiment(generateDependencyExperiment(cd, diff, dd, tgtByOp))
			}
		}
	}

	return experiments
}

// generateRenameExperiment creates a PodKill experiment targeting the old
// deployment name, simulating what happens when the renamed component's
// pods are killed during upgrade.
func generateRenameExperiment(
	cd ComponentDiff,
	diff *UpgradeDiff,
	srcK, tgtK *model.OperatorKnowledge,
) *v1alpha1.ChaosExperiment {
	exp := baseExperiment(
		fmt.Sprintf("rename-%s", sanitizeName(cd.Component)),
		cd.Operator,
		cd.Component,
		diff,
	)

	// Use source component (the old name) for label selector
	var labelSelector string
	if srcK != nil {
		srcComp := findComponent(srcK, cd.RenamedFrom)
		if srcComp != nil {
			labelSelector = primaryDeploymentLabels(srcComp)
		}
	}

	exp.Spec.Injection = v1alpha1.InjectionSpec{
		Type: v1alpha1.PodKill,
		Parameters: map[string]string{
			"labelSelector": labelSelector,
		},
		TTL: metav1.Duration{Duration: 300 * time.Second},
	}

	exp.Spec.Hypothesis = v1alpha1.HypothesisSpec{
		Description: fmt.Sprintf(
			"Component renamed from %q to %q should recover after pod kill on old deployment",
			cd.RenamedFrom, cd.Component,
		),
	}

	// Use target steady state checks if available
	if tgtK != nil {
		tgtComp := findComponent(tgtK, cd.Component)
		if tgtComp != nil && len(tgtComp.SteadyState.Checks) > 0 {
			exp.Spec.SteadyState = tgtComp.SteadyState
		}
	}

	return &exp
}

// generateNamespaceMoveExperiment creates a NetworkPartition experiment
// that partitions traffic between the old and new namespaces.
func generateNamespaceMoveExperiment(
	cd ComponentDiff,
	diff *UpgradeDiff,
	srcK *model.OperatorKnowledge,
) *v1alpha1.ChaosExperiment {
	exp := baseExperiment(
		fmt.Sprintf("ns-move-%s", sanitizeName(cd.Component)),
		cd.Operator,
		cd.Component,
		diff,
	)

	var labelSelector string
	if srcK != nil {
		srcComp := findComponent(srcK, cd.Component)
		// If the component was also renamed, look up by the old name
		if srcComp == nil && cd.RenamedFrom != "" {
			srcComp = findComponent(srcK, cd.RenamedFrom)
		}
		if srcComp != nil {
			labelSelector = primaryDeploymentLabels(srcComp)
		}
	}

	exp.Spec.Injection = v1alpha1.InjectionSpec{
		Type: v1alpha1.NetworkPartition,
		Parameters: map[string]string{
			"labelSelector": labelSelector,
		},
		TTL: metav1.Duration{Duration: 120 * time.Second},
	}

	exp.Spec.BlastRadius.AllowedNamespaces = []string{
		cd.NamespaceChange.From,
		cd.NamespaceChange.To,
	}

	exp.Spec.Hypothesis = v1alpha1.HypothesisSpec{
		Description: fmt.Sprintf(
			"Component %q namespace move from %q to %q should handle network partition",
			cd.Component, cd.NamespaceChange.From, cd.NamespaceChange.To,
		),
	}

	return &exp
}

// generateWebhookExperiment creates a WebhookDisrupt experiment for
// added or removed webhooks.
func generateWebhookExperiment(
	cd ComponentDiff,
	diff *UpgradeDiff,
	wd WebhookDiff,
) *v1alpha1.ChaosExperiment {
	if wd.ChangeType != DiffRemoved && wd.ChangeType != DiffAdded {
		return nil
	}

	exp := baseExperiment(
		fmt.Sprintf("webhook-%s", sanitizeName(wd.Name)),
		cd.Operator,
		cd.Component,
		diff,
	)

	value := "Fail"
	desc := fmt.Sprintf("Added webhook %q should handle failure policy set to Fail", wd.Name)
	if wd.ChangeType == DiffRemoved {
		value = "Ignore"
		desc = fmt.Sprintf("Removed webhook %q should handle failure policy set to Ignore", wd.Name)
	}

	exp.Spec.Injection = v1alpha1.InjectionSpec{
		Type: v1alpha1.WebhookDisrupt,
		Parameters: map[string]string{
			"webhookName": wd.Name,
			"action":      "setFailurePolicy",
			"value":       value,
		},
		DangerLevel: v1alpha1.DangerLevelHigh,
		TTL:         metav1.Duration{Duration: 120 * time.Second},
	}

	exp.Spec.BlastRadius.AllowDangerous = true

	exp.Spec.Hypothesis = v1alpha1.HypothesisSpec{
		Description: desc,
	}

	return &exp
}

// generateDependencyExperiment creates a PodKill experiment targeting
// a newly added dependency's first component deployment.
func generateDependencyExperiment(
	cd ComponentDiff,
	diff *UpgradeDiff,
	dd DependencyDiff,
	tgtByOp map[string]*model.OperatorKnowledge,
) *v1alpha1.ChaosExperiment {
	depK, ok := tgtByOp[dd.Dependency]
	if !ok || len(depK.Components) == 0 {
		return nil
	}

	depComp := &depK.Components[0]
	labelSelector := primaryDeploymentLabels(depComp)
	if labelSelector == "" {
		return nil
	}

	exp := baseExperiment(
		fmt.Sprintf("dep-%s-%s", sanitizeName(cd.Component), sanitizeName(dd.Dependency)),
		cd.Operator,
		cd.Component,
		diff,
	)

	exp.Spec.Injection = v1alpha1.InjectionSpec{
		Type: v1alpha1.PodKill,
		Parameters: map[string]string{
			"labelSelector": labelSelector,
		},
		TTL: metav1.Duration{Duration: 300 * time.Second},
	}

	exp.Spec.Hypothesis = v1alpha1.HypothesisSpec{
		Description: fmt.Sprintf(
			"New dependency %q pod kill should not break %q",
			dd.Dependency, cd.Component,
		),
	}

	return &exp
}

// baseExperiment creates a ChaosExperiment with common metadata and labels.
func baseExperiment(name, operator, component string, diff *UpgradeDiff) v1alpha1.ChaosExperiment {
	return v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"chaos.operatorchaos.io/upgrade-simulation": "true",
				"chaos.operatorchaos.io/operator":           operator,
				"chaos.operatorchaos.io/component":          component,
				"chaos.operatorchaos.io/source-version":     diff.SourceVersion,
				"chaos.operatorchaos.io/target-version":     diff.TargetVersion,
			},
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target: v1alpha1.TargetSpec{
				Operator:  operator,
				Component: component,
			},
			BlastRadius: v1alpha1.BlastRadiusSpec{
				MaxPodsAffected: 1,
			},
		},
	}
}

// findComponent looks up a component by name in an OperatorKnowledge.
func findComponent(k *model.OperatorKnowledge, name string) *model.ComponentModel {
	return k.GetComponent(name)
}

// primaryDeploymentLabels finds the first Deployment in a component's managed
// resources that has labels, and returns them as a "k=v,k=v" selector string.
// Keys are sorted for deterministic output.
func primaryDeploymentLabels(comp *model.ComponentModel) string {
	for _, r := range comp.ManagedResources {
		if r.Kind == "Deployment" && len(r.Labels) > 0 {
			keys := make([]string, 0, len(r.Labels))
			for k := range r.Labels {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			var parts []string
			for _, k := range keys {
				parts = append(parts, fmt.Sprintf("%s=%s", k, r.Labels[k]))
			}
			return strings.Join(parts, ",")
		}
	}
	return ""
}

// sanitizeName lowercases the input, replaces dots, underscores, and spaces
// with hyphens, and truncates to 30 characters.
func sanitizeName(s string) string {
	s = strings.ToLower(s)
	replacer := strings.NewReplacer(".", "-", "_", "-", " ", "-")
	s = replacer.Replace(s)
	if len(s) > 30 {
		s = s[:30]
	}
	return s
}
