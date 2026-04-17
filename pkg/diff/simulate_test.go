package diff

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
)

func TestGenerateExperimentsRename(t *testing.T) {
	source := []*model.OperatorKnowledge{
		{
			Operator: model.OperatorMeta{
				Name:      "dashboard",
				Namespace: "opendatahub",
				Version:   "2.10",
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
					},
				},
			},
		},
	}

	target := []*model.OperatorKnowledge{
		{
			Operator: model.OperatorMeta{
				Name:      "dashboard",
				Namespace: "opendatahub",
				Version:   "2.11",
			},
			Components: []model.ComponentModel{
				{
					Name:       "dashboard",
					Controller: "dashboard-controller",
					ManagedResources: []model.ManagedResource{
						{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       "dashboard",
							Namespace:  "opendatahub",
							Labels:     map[string]string{"app": "dashboard"},
						},
					},
				},
			},
		},
	}

	diff := ComputeDiff(source, target)
	experiments := GenerateUpgradeExperiments(diff, source, target)

	if len(experiments) == 0 {
		t.Fatal("expected at least one experiment for rename, got none")
	}

	found := false
	for _, exp := range experiments {
		if exp.Spec.Injection.Type == v1alpha1.PodKill {
			found = true

			// Check upgrade-simulation label
			if exp.Labels == nil {
				t.Fatal("expected labels on experiment")
			}
			if exp.Labels["chaos.opendatahub.io/upgrade-simulation"] != "true" {
				t.Error("expected upgrade-simulation label to be 'true'")
			}

			// Check operator target
			if exp.Spec.Target.Operator != "dashboard" {
				t.Errorf("expected operator 'dashboard', got %q", exp.Spec.Target.Operator)
			}

			// Check TTL is 300s
			if exp.Spec.Injection.TTL.Duration != 300*time.Second {
				t.Errorf("expected TTL 300s, got %v", exp.Spec.Injection.TTL.Duration)
			}

			// Check labelSelector parameter
			ls, ok := exp.Spec.Injection.Parameters["labelSelector"]
			if !ok || ls == "" {
				t.Error("expected labelSelector parameter to be set")
			}
		}
	}

	if !found {
		t.Error("expected a PodKill experiment for rename, but none found")
	}
}

func TestGenerateExperimentsNamespaceMove(t *testing.T) {
	source := []*model.OperatorKnowledge{
		{
			Operator: model.OperatorMeta{
				Name:      "kserve",
				Namespace: "kserve",
				Version:   "2.10",
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

	target := []*model.OperatorKnowledge{
		{
			Operator: model.OperatorMeta{
				Name:      "kserve",
				Namespace: "opendatahub",
				Version:   "2.11",
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
							Namespace:  "opendatahub",
							Labels:     map[string]string{"control-plane": "kserve-controller-manager"},
						},
					},
				},
			},
		},
	}

	diff := ComputeDiff(source, target)
	experiments := GenerateUpgradeExperiments(diff, source, target)

	if len(experiments) == 0 {
		t.Fatal("expected at least one experiment for namespace move, got none")
	}

	found := false
	for _, exp := range experiments {
		if exp.Spec.Injection.Type == v1alpha1.NetworkPartition {
			found = true

			// Check TTL is 120s
			if exp.Spec.Injection.TTL.Duration != 120*time.Second {
				t.Errorf("expected TTL 120s, got %v", exp.Spec.Injection.TTL.Duration)
			}

			// Check AllowedNamespaces includes both old and new
			ns := exp.Spec.BlastRadius.AllowedNamespaces
			hasOld, hasNew := false, false
			for _, n := range ns {
				if n == "kserve" {
					hasOld = true
				}
				if n == "opendatahub" {
					hasNew = true
				}
			}
			if !hasOld || !hasNew {
				t.Errorf("expected AllowedNamespaces to contain both 'kserve' and 'opendatahub', got %v", ns)
			}
		}
	}

	if !found {
		t.Error("expected a NetworkPartition experiment for namespace move, but none found")
	}
}

func TestGenerateExperimentsWebhookRemoved(t *testing.T) {
	source := []*model.OperatorKnowledge{
		{
			Operator: model.OperatorMeta{
				Name:      "dashboard",
				Namespace: "opendatahub",
				Version:   "2.10",
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
					},
					Webhooks: []model.WebhookSpec{
						{Name: "dashboard-webhook", Type: "validating", Path: "/validate"},
					},
				},
			},
		},
	}

	target := []*model.OperatorKnowledge{
		{
			Operator: model.OperatorMeta{
				Name:      "dashboard",
				Namespace: "opendatahub",
				Version:   "2.11",
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
					},
					// No webhooks in target
				},
			},
		},
	}

	diff := ComputeDiff(source, target)
	experiments := GenerateUpgradeExperiments(diff, source, target)

	if len(experiments) == 0 {
		t.Fatal("expected at least one experiment for webhook removal, got none")
	}

	found := false
	for _, exp := range experiments {
		if exp.Spec.Injection.Type == v1alpha1.WebhookDisrupt {
			found = true

			// Check webhookName parameter
			wn, ok := exp.Spec.Injection.Parameters["webhookName"]
			if !ok || wn != "dashboard-webhook" {
				t.Errorf("expected webhookName parameter 'dashboard-webhook', got %q", wn)
			}

			// Check value=Ignore for removed webhook
			val, ok := exp.Spec.Injection.Parameters["value"]
			if !ok || val != "Ignore" {
				t.Errorf("expected value parameter 'Ignore', got %q", val)
			}

			// Check action=setFailurePolicy
			action, ok := exp.Spec.Injection.Parameters["action"]
			if !ok || action != "setFailurePolicy" {
				t.Errorf("expected action parameter 'setFailurePolicy', got %q", action)
			}

			// Check DangerLevel High and AllowDangerous
			if exp.Spec.Injection.DangerLevel != v1alpha1.DangerLevelHigh {
				t.Errorf("expected DangerLevel high, got %q", exp.Spec.Injection.DangerLevel)
			}
			if !exp.Spec.BlastRadius.AllowDangerous {
				t.Error("expected AllowDangerous to be true")
			}
		}
	}

	if !found {
		t.Error("expected a WebhookDisrupt experiment for webhook removal, but none found")
	}
}

// suppress unused import warning
var _ = metav1.Duration{}
