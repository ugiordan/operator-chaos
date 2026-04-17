package diff

import (
	"testing"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
)

func TestMatchComponentsExact(t *testing.T) {
	source := []model.ComponentModel{
		{
			Name:       "dashboard",
			Controller: "dashboard-controller",
			ManagedResources: []model.ManagedResource{
				{Kind: "Deployment", Name: "dashboard"},
			},
		},
		{
			Name:       "notebook-controller",
			Controller: "notebook-controller",
			ManagedResources: []model.ManagedResource{
				{Kind: "Deployment", Name: "notebook-controller"},
			},
		},
	}
	target := []model.ComponentModel{
		{
			Name:       "dashboard",
			Controller: "dashboard-controller",
			ManagedResources: []model.ManagedResource{
				{Kind: "Deployment", Name: "dashboard"},
				{Kind: "Service", Name: "dashboard-svc"},
			},
		},
		{
			Name:       "notebook-controller",
			Controller: "notebook-controller",
			ManagedResources: []model.ManagedResource{
				{Kind: "Deployment", Name: "notebook-controller"},
			},
		},
	}

	matched, added, removed := matchComponents("test-operator", source, target)

	if len(matched) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matched))
	}
	if len(added) != 0 {
		t.Fatalf("expected 0 added, got %d", len(added))
	}
	if len(removed) != 0 {
		t.Fatalf("expected 0 removed, got %d", len(removed))
	}
	for _, m := range matched {
		if m.renamed {
			t.Errorf("component %q should not be marked as renamed", m.source.Name)
		}
	}
}

func TestMatchComponentsFuzzyRename(t *testing.T) {
	source := []model.ComponentModel{
		{
			Name:       "odh-dashboard",
			Controller: "dashboard-controller",
			ManagedResources: []model.ManagedResource{
				{Kind: "Deployment", Name: "odh-dashboard", Labels: map[string]string{"app": "dashboard", "component": "ui"}},
				{Kind: "Service", Name: "odh-dashboard-svc", Labels: map[string]string{"app": "dashboard"}},
				{Kind: "ConfigMap", Name: "odh-dashboard-config"},
			},
		},
	}
	target := []model.ComponentModel{
		{
			Name:       "rhods-dashboard",
			Controller: "dashboard-controller",
			ManagedResources: []model.ManagedResource{
				{Kind: "Deployment", Name: "rhods-dashboard", Labels: map[string]string{"app": "dashboard", "component": "ui"}},
				{Kind: "Service", Name: "rhods-dashboard-svc", Labels: map[string]string{"app": "dashboard"}},
				{Kind: "ConfigMap", Name: "rhods-dashboard-config"},
			},
		},
	}

	matched, added, removed := matchComponents("test-operator", source, target)

	if len(matched) != 1 {
		t.Fatalf("expected 1 fuzzy match, got %d matched, %d added, %d removed", len(matched), len(added), len(removed))
	}
	if !matched[0].renamed {
		t.Error("expected match to be marked as renamed")
	}
	if matched[0].source.Name != "odh-dashboard" {
		t.Errorf("expected source name 'odh-dashboard', got %q", matched[0].source.Name)
	}
	if matched[0].target.Name != "rhods-dashboard" {
		t.Errorf("expected target name 'rhods-dashboard', got %q", matched[0].target.Name)
	}
}

func TestMatchComponentsAddedRemoved(t *testing.T) {
	source := []model.ComponentModel{
		{
			Name:       "old-component",
			Controller: "old-controller",
			ManagedResources: []model.ManagedResource{
				{Kind: "Deployment", Name: "old-deploy"},
			},
		},
	}
	target := []model.ComponentModel{
		{
			Name:       "new-component",
			Controller: "new-controller",
			ManagedResources: []model.ManagedResource{
				{Kind: "StatefulSet", Name: "new-sts"},
			},
		},
	}

	matched, added, removed := matchComponents("test-operator", source, target)

	if len(matched) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matched))
	}
	if len(added) != 1 {
		t.Fatalf("expected 1 added, got %d", len(added))
	}
	if len(removed) != 1 {
		t.Fatalf("expected 1 removed, got %d", len(removed))
	}
	if added[0].Name != "new-component" {
		t.Errorf("expected added component 'new-component', got %q", added[0].Name)
	}
	if removed[0].Name != "old-component" {
		t.Errorf("expected removed component 'old-component', got %q", removed[0].Name)
	}
}

func TestComponentSimilarity(t *testing.T) {
	tests := []struct {
		name      string
		a, b      model.ComponentModel
		wantAbove float64
		wantBelow float64
	}{
		{
			name: "structurally similar components score above threshold",
			a: model.ComponentModel{
				Name:       "odh-dashboard",
				Controller: "dashboard-controller",
				ManagedResources: []model.ManagedResource{
					{Kind: "Deployment", Name: "odh-dashboard", Labels: map[string]string{"app": "dashboard"}},
					{Kind: "Service", Name: "odh-svc", Labels: map[string]string{"app": "dashboard"}},
				},
			},
			b: model.ComponentModel{
				Name:       "rhods-dashboard",
				Controller: "dashboard-controller",
				ManagedResources: []model.ManagedResource{
					{Kind: "Deployment", Name: "rhods-dashboard", Labels: map[string]string{"app": "dashboard"}},
					{Kind: "Service", Name: "rhods-svc", Labels: map[string]string{"app": "dashboard"}},
				},
			},
			wantAbove: 0.6,
			wantBelow: 1.1,
		},
		{
			name: "completely different components score below threshold",
			a: model.ComponentModel{
				Name:       "dashboard",
				Controller: "dashboard-controller",
				ManagedResources: []model.ManagedResource{
					{Kind: "Deployment", Name: "dashboard"},
				},
			},
			b: model.ComponentModel{
				Name:       "model-registry",
				Controller: "registry-controller",
				ManagedResources: []model.ManagedResource{
					{Kind: "StatefulSet", Name: "model-registry"},
					{Kind: "PersistentVolumeClaim", Name: "registry-pvc"},
				},
			},
			wantAbove: -0.1,
			wantBelow: 0.6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := componentSimilarity(tt.a, tt.b)
			if score <= tt.wantAbove {
				t.Errorf("score %.3f should be above %.1f", score, tt.wantAbove)
			}
			if score >= tt.wantBelow {
				t.Errorf("score %.3f should be below %.1f", score, tt.wantBelow)
			}
		})
	}
}

func TestMatchResourcesExact(t *testing.T) {
	source := []model.ManagedResource{
		{Kind: "Deployment", Name: "dashboard"},
		{Kind: "Service", Name: "dashboard-svc"},
	}
	target := []model.ManagedResource{
		{Kind: "Deployment", Name: "dashboard"},
		{Kind: "Service", Name: "dashboard-svc"},
	}

	matched, added, removed := matchResources(source, target)

	if len(matched) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matched))
	}
	if len(added) != 0 {
		t.Fatalf("expected 0 added, got %d", len(added))
	}
	if len(removed) != 0 {
		t.Fatalf("expected 0 removed, got %d", len(removed))
	}
	for _, m := range matched {
		if m.renamed {
			t.Errorf("resource %s/%s should not be marked as renamed", m.source.Kind, m.source.Name)
		}
	}
}

func TestMatchResourcesFuzzyRename(t *testing.T) {
	source := []model.ManagedResource{
		{Kind: "Deployment", Name: "odh-dashboard", Labels: map[string]string{"app": "dashboard", "component": "ui", "managed-by": "operator"}},
	}
	target := []model.ManagedResource{
		{Kind: "Deployment", Name: "rhods-dashboard", Labels: map[string]string{"app": "dashboard", "component": "ui", "managed-by": "operator"}},
	}

	matched, added, removed := matchResources(source, target)

	if len(matched) != 1 {
		t.Fatalf("expected 1 fuzzy match, got %d matched, %d added, %d removed", len(matched), len(added), len(removed))
	}
	if !matched[0].renamed {
		t.Error("expected match to be marked as renamed")
	}
	if matched[0].source.Name != "odh-dashboard" {
		t.Errorf("expected source name 'odh-dashboard', got %q", matched[0].source.Name)
	}
	if matched[0].target.Name != "rhods-dashboard" {
		t.Errorf("expected target name 'rhods-dashboard', got %q", matched[0].target.Name)
	}
}
