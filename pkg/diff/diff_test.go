package diff

import (
	"testing"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
)

// makeSourceKnowledge returns ODH v2.10 style knowledge:
// dashboard with "odh-dashboard" component in "opendatahub" namespace,
// kserve in "kserve" namespace. Dashboard has a webhook, a finalizer, no dependencies.
func makeSourceKnowledge() []*model.OperatorKnowledge {
	return []*model.OperatorKnowledge{
		{
			Operator: model.OperatorMeta{
				Name:      "dashboard",
				Namespace: "opendatahub",
				Version:   "2.10",
				Platform:  "ODH",
			},
			Components: []model.ComponentModel{
				{
					Name:       "odh-dashboard",
					Controller: "dashboard-controller",
					ManagedResources: []model.ManagedResource{
						{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       "odh-dashboard",
							Namespace:  "opendatahub",
							Labels:     map[string]string{"app": "odh-dashboard"},
						},
						{
							APIVersion: "v1",
							Kind:       "Service",
							Name:       "odh-dashboard",
							Namespace:  "opendatahub",
							Labels:     map[string]string{"app": "odh-dashboard"},
						},
					},
					Webhooks: []model.WebhookSpec{
						{Name: "dashboard-webhook", Type: "validating", Path: "/validate"},
					},
					Finalizers: []string{"dashboard.opendatahub.io/finalizer"},
				},
			},
		},
		{
			Operator: model.OperatorMeta{
				Name:      "kserve",
				Namespace: "kserve",
				Version:   "2.10",
				Platform:  "ODH",
			},
			Components: []model.ComponentModel{
				{
					Name:       "kserve-controller",
					Controller: "kserve-controller-manager",
					ManagedResources: []model.ManagedResource{
						{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       "kserve-controller-manager",
							Namespace:  "kserve",
							Labels:     map[string]string{"control-plane": "kserve-controller-manager"},
						},
					},
				},
			},
		},
	}
}

// makeTargetKnowledge returns RHOAI v3.3 style knowledge:
// dashboard with "rhods-dashboard" component in "redhat-ods-applications" namespace,
// kserve in "redhat-ods-applications". Dashboard webhook path changed, new dependency on kserve.
func makeTargetKnowledge() []*model.OperatorKnowledge {
	return []*model.OperatorKnowledge{
		{
			Operator: model.OperatorMeta{
				Name:      "dashboard",
				Namespace: "redhat-ods-applications",
				Version:   "3.3",
				Platform:  "RHOAI",
			},
			Components: []model.ComponentModel{
				{
					Name:       "rhods-dashboard",
					Controller: "dashboard-controller",
					ManagedResources: []model.ManagedResource{
						{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       "rhods-dashboard",
							Namespace:  "redhat-ods-applications",
							Labels:     map[string]string{"app": "odh-dashboard"},
						},
						{
							APIVersion: "v1",
							Kind:       "Service",
							Name:       "rhods-dashboard",
							Namespace:  "redhat-ods-applications",
							Labels:     map[string]string{"app": "odh-dashboard"},
						},
					},
					Webhooks: []model.WebhookSpec{
						{Name: "dashboard-webhook", Type: "validating", Path: "/validate-v2"},
					},
					Finalizers:   []string{"dashboard.opendatahub.io/finalizer"},
					Dependencies: []string{"kserve"},
				},
			},
		},
		{
			Operator: model.OperatorMeta{
				Name:      "kserve",
				Namespace: "redhat-ods-applications",
				Version:   "3.3",
				Platform:  "RHOAI",
			},
			Components: []model.ComponentModel{
				{
					Name:       "kserve-controller",
					Controller: "kserve-controller-manager",
					ManagedResources: []model.ManagedResource{
						{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       "kserve-controller-manager",
							Namespace:  "redhat-ods-applications",
							Labels:     map[string]string{"control-plane": "kserve-controller-manager"},
						},
					},
				},
			},
		},
	}
}

func TestComputeDiff(t *testing.T) {
	source := makeSourceKnowledge()
	target := makeTargetKnowledge()

	result := ComputeDiff(source, target)

	// Versions extracted correctly
	if result.SourceVersion != "2.10" {
		t.Errorf("expected source version 2.10, got %s", result.SourceVersion)
	}
	if result.TargetVersion != "3.3" {
		t.Errorf("expected target version 3.3, got %s", result.TargetVersion)
	}

	// Find dashboard component diff
	var dashboardDiff *ComponentDiff
	var kserveDiff *ComponentDiff
	for i := range result.Components {
		cd := &result.Components[i]
		if cd.Operator == "dashboard" {
			dashboardDiff = cd
		}
		if cd.Operator == "kserve" {
			kserveDiff = cd
		}
	}

	if dashboardDiff == nil {
		t.Fatal("expected dashboard component diff")
	}

	// Dashboard detected as Renamed (odh-dashboard -> rhods-dashboard)
	if dashboardDiff.ChangeType != ComponentRenamed {
		t.Errorf("expected dashboard change type Renamed, got %s", dashboardDiff.ChangeType)
	}
	if dashboardDiff.RenamedFrom != "odh-dashboard" {
		t.Errorf("expected dashboard renamed from odh-dashboard, got %s", dashboardDiff.RenamedFrom)
	}
	if dashboardDiff.Component != "rhods-dashboard" {
		t.Errorf("expected dashboard component name rhods-dashboard, got %s", dashboardDiff.Component)
	}

	// Dashboard namespace change detected
	if dashboardDiff.NamespaceChange == nil {
		t.Fatal("expected dashboard namespace change")
	}
	if dashboardDiff.NamespaceChange.From != "opendatahub" {
		t.Errorf("expected namespace change from opendatahub, got %s", dashboardDiff.NamespaceChange.From)
	}
	if dashboardDiff.NamespaceChange.To != "redhat-ods-applications" {
		t.Errorf("expected namespace change to redhat-ods-applications, got %s", dashboardDiff.NamespaceChange.To)
	}

	// Webhook path change detected
	if len(dashboardDiff.WebhookDiffs) == 0 {
		t.Fatal("expected webhook diffs for dashboard")
	}
	foundWebhookPathChange := false
	for _, wd := range dashboardDiff.WebhookDiffs {
		if wd.Name == "dashboard-webhook" && wd.OldPath == "/validate" && wd.NewPath == "/validate-v2" {
			foundWebhookPathChange = true
		}
	}
	if !foundWebhookPathChange {
		t.Errorf("expected webhook path change /validate -> /validate-v2")
	}

	// Dependency on kserve detected as Added
	if len(dashboardDiff.DependencyDiffs) == 0 {
		t.Fatal("expected dependency diffs for dashboard")
	}
	foundDepAdded := false
	for _, dd := range dashboardDiff.DependencyDiffs {
		if dd.Dependency == "kserve" && dd.ChangeType == DiffAdded {
			foundDepAdded = true
		}
	}
	if !foundDepAdded {
		t.Errorf("expected dependency 'kserve' to be Added")
	}

	// Kserve detected as Modified with namespace change
	if kserveDiff == nil {
		t.Fatal("expected kserve component diff")
	}
	if kserveDiff.ChangeType != ComponentModified {
		t.Errorf("expected kserve change type Modified, got %s", kserveDiff.ChangeType)
	}
	if kserveDiff.NamespaceChange == nil {
		t.Fatal("expected kserve namespace change")
	}
	if kserveDiff.NamespaceChange.From != "kserve" || kserveDiff.NamespaceChange.To != "redhat-ods-applications" {
		t.Errorf("expected kserve namespace change kserve -> redhat-ods-applications, got %s -> %s",
			kserveDiff.NamespaceChange.From, kserveDiff.NamespaceChange.To)
	}

	// Summary checks
	if result.Summary.ComponentsRenamed != 1 {
		t.Errorf("expected 1 renamed component, got %d", result.Summary.ComponentsRenamed)
	}
	if result.Summary.NamespaceMoves < 1 {
		t.Errorf("expected at least 1 namespace move, got %d", result.Summary.NamespaceMoves)
	}
	if result.Summary.BreakingChanges < 1 {
		t.Errorf("expected at least 1 breaking change, got %d", result.Summary.BreakingChanges)
	}
}

func TestComputeDiffAddedRemoved(t *testing.T) {
	source := []*model.OperatorKnowledge{
		{
			Operator: model.OperatorMeta{
				Name:      "old-op",
				Namespace: "ns-old",
				Version:   "1.0",
			},
			Components: []model.ComponentModel{
				{
					Name:       "X",
					Controller: "controller-x",
					ManagedResources: []model.ManagedResource{
						{APIVersion: "v1", Kind: "ConfigMap", Name: "x-config", Namespace: "ns-old"},
					},
				},
			},
		},
	}

	target := []*model.OperatorKnowledge{
		{
			Operator: model.OperatorMeta{
				Name:      "new-op",
				Namespace: "ns-new",
				Version:   "2.0",
			},
			Components: []model.ComponentModel{
				{
					Name:       "Y",
					Controller: "controller-y",
					ManagedResources: []model.ManagedResource{
						{APIVersion: "apps/v1", Kind: "Deployment", Name: "y-deploy", Namespace: "ns-new"},
					},
				},
			},
		},
	}

	result := ComputeDiff(source, target)

	removedCount := 0
	addedCount := 0
	for _, cd := range result.Components {
		switch cd.ChangeType {
		case ComponentRemoved:
			removedCount++
		case ComponentAdded:
			addedCount++
		}
	}

	if removedCount != 1 {
		t.Errorf("expected 1 removed component, got %d", removedCount)
	}
	if addedCount != 1 {
		t.Errorf("expected 1 added component, got %d", addedCount)
	}

	if result.Summary.ComponentsRemoved != 1 {
		t.Errorf("expected summary: 1 removed, got %d", result.Summary.ComponentsRemoved)
	}
	if result.Summary.ComponentsAdded != 1 {
		t.Errorf("expected summary: 1 added, got %d", result.Summary.ComponentsAdded)
	}
}
