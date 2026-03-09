package model

import (
	"testing"
	"time"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
)

func validKnowledge() *OperatorKnowledge {
	return &OperatorKnowledge{
		Operator: OperatorMeta{
			Name:      "test-operator",
			Namespace: "test-ns",
		},
		Components: []ComponentModel{
			{
				Name:       "dashboard",
				Controller: "DataScienceCluster",
				ManagedResources: []ManagedResource{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "test-dashboard",
					},
				},
			},
		},
		Recovery: RecoveryExpectations{
			ReconcileTimeout:   v1alpha1.Duration{Duration: 300 * time.Second},
			MaxReconcileCycles: 10,
		},
	}
}

func TestValidateKnowledge_Valid(t *testing.T) {
	errs := ValidateKnowledge(validKnowledge())
	if len(errs) > 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateKnowledge_MissingOperatorName(t *testing.T) {
	k := validKnowledge()
	k.Operator.Name = ""
	errs := ValidateKnowledge(k)
	assertContains(t, errs, "operator.name is required")
}

func TestValidateKnowledge_MissingOperatorNamespace(t *testing.T) {
	k := validKnowledge()
	k.Operator.Namespace = ""
	errs := ValidateKnowledge(k)
	assertContains(t, errs, "operator.namespace is required")
}

func TestValidateKnowledge_NoComponents(t *testing.T) {
	k := validKnowledge()
	k.Components = nil
	errs := ValidateKnowledge(k)
	assertContains(t, errs, "at least one component is required")
}

func TestValidateKnowledge_ComponentMissingName(t *testing.T) {
	k := validKnowledge()
	k.Components[0].Name = ""
	errs := ValidateKnowledge(k)
	assertContains(t, errs, "components[0].name is required")
}

func TestValidateKnowledge_ComponentMissingController(t *testing.T) {
	k := validKnowledge()
	k.Components[0].Controller = ""
	errs := ValidateKnowledge(k)
	assertContains(t, errs, "components[0].controller is required")
}

func TestValidateKnowledge_ComponentNoManagedResources(t *testing.T) {
	k := validKnowledge()
	k.Components[0].ManagedResources = nil
	errs := ValidateKnowledge(k)
	assertContains(t, errs, "components[0] must have at least one managedResource")
}

func TestValidateKnowledge_ManagedResourceMissingFields(t *testing.T) {
	k := validKnowledge()
	k.Components[0].ManagedResources[0] = ManagedResource{}
	errs := ValidateKnowledge(k)
	assertContains(t, errs, "components[0].managedResources[0].apiVersion is required")
	assertContains(t, errs, "components[0].managedResources[0].kind is required")
	assertContains(t, errs, "components[0].managedResources[0].name is required")
}

func TestValidateKnowledge_WebhookValidType(t *testing.T) {
	k := validKnowledge()
	k.Components[0].Webhooks = []WebhookSpec{
		{Name: "wh1", Type: "validating", Path: "/validate"},
		{Name: "wh2", Type: "mutating", Path: "/mutate"},
	}
	errs := ValidateKnowledge(k)
	if len(errs) > 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateKnowledge_WebhookInvalidType(t *testing.T) {
	k := validKnowledge()
	k.Components[0].Webhooks = []WebhookSpec{
		{Name: "wh1", Type: "invalid", Path: "/validate"},
	}
	errs := ValidateKnowledge(k)
	assertContains(t, errs, `components[0].webhooks[0].type must be "validating" or "mutating", got "invalid"`)
}

func TestValidateKnowledge_WebhookMissingFields(t *testing.T) {
	k := validKnowledge()
	k.Components[0].Webhooks = []WebhookSpec{{}}
	errs := ValidateKnowledge(k)
	assertContains(t, errs, "components[0].webhooks[0].name is required")
	assertContains(t, errs, "components[0].webhooks[0].type is required")
	assertContains(t, errs, "components[0].webhooks[0].path is required")
}

func TestValidateKnowledge_RecoveryZeroTimeout(t *testing.T) {
	k := validKnowledge()
	k.Recovery.ReconcileTimeout = v1alpha1.Duration{}
	errs := ValidateKnowledge(k)
	assertContains(t, errs, "recovery.reconcileTimeout must be greater than 0")
}

func TestValidateKnowledge_RecoveryZeroCycles(t *testing.T) {
	k := validKnowledge()
	k.Recovery.MaxReconcileCycles = 0
	errs := ValidateKnowledge(k)
	assertContains(t, errs, "recovery.maxReconcileCycles must be greater than 0")
}

func TestValidateKnowledge_DuplicateComponentNames(t *testing.T) {
	k := validKnowledge()
	k.Components = append(k.Components, ComponentModel{
		Name:       "dashboard",
		Controller: "DSC",
		ManagedResources: []ManagedResource{
			{APIVersion: "v1", Kind: "Service", Name: "svc"},
		},
	})
	errs := ValidateKnowledge(k)
	assertContains(t, errs, `components[1]: duplicate component name "dashboard"`)
}

func TestValidateKnowledge_DuplicateManagedResourceNames(t *testing.T) {
	k := validKnowledge()
	k.Components[0].ManagedResources = append(k.Components[0].ManagedResources, ManagedResource{
		APIVersion: "v1",
		Kind:       "Service",
		Name:       "test-dashboard",
	})
	errs := ValidateKnowledge(k)
	assertContains(t, errs, `components[0]: duplicate managedResource name "test-dashboard"`)
}

func TestValidateKnowledge_Nil(t *testing.T) {
	errs := ValidateKnowledge(nil)
	assertContains(t, errs, "knowledge must not be nil")
}

func TestValidateKnowledge_RecoveryNegativeTimeout(t *testing.T) {
	k := validKnowledge()
	k.Recovery.ReconcileTimeout = v1alpha1.Duration{Duration: -1 * time.Second}
	errs := ValidateKnowledge(k)
	assertContains(t, errs, "recovery.reconcileTimeout must be greater than 0")
}

func TestValidateKnowledge_RecoveryNegativeCycles(t *testing.T) {
	k := validKnowledge()
	k.Recovery.MaxReconcileCycles = -5
	errs := ValidateKnowledge(k)
	assertContains(t, errs, "recovery.maxReconcileCycles must be greater than 0")
}

func TestValidateKnowledge_UnknownDependency(t *testing.T) {
	k := validKnowledge()
	k.Components[0].Dependencies = []string{"nonexistent"}
	errs := ValidateKnowledge(k)
	assertContains(t, errs, `components[0].dependencies references unknown component "nonexistent"`)
}

func TestValidateKnowledge_ValidDependency(t *testing.T) {
	k := validKnowledge()
	k.Components = append(k.Components, ComponentModel{
		Name:       "model-controller",
		Controller: "DSC",
		ManagedResources: []ManagedResource{
			{APIVersion: "apps/v1", Kind: "Deployment", Name: "mc"},
		},
	})
	k.Components[1].Dependencies = []string{"dashboard"}
	errs := ValidateKnowledge(k)
	if len(errs) > 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func assertContains(t *testing.T, errs []string, expected string) {
	t.Helper()
	for _, e := range errs {
		if e == expected {
			return
		}
	}
	t.Errorf("expected error %q not found in %v", expected, errs)
}
