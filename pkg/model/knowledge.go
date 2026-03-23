package model

import (
	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OperatorKnowledge encodes the full semantics of an operator:
// its components, managed resources, ownership chains, steady-state
// expectations, and recovery behavior. This is loaded from YAML files
// that teams maintain per-operator.
type OperatorKnowledge struct {
	Operator   OperatorMeta         `json:"operator" yaml:"operator"`
	Components []ComponentModel     `json:"components" yaml:"components"`
	Recovery   RecoveryExpectations `json:"recovery" yaml:"recovery"`
}

// OperatorMeta contains identifying information about the operator.
type OperatorMeta struct {
	Name       string `json:"name" yaml:"name"`
	Namespace  string `json:"namespace" yaml:"namespace"`
	Repository string `json:"repository,omitempty" yaml:"repository,omitempty"`
}

// ComponentModel describes a single component managed by the operator,
// including its controller, managed resources, dependencies, and
// steady-state definition.
type ComponentModel struct {
	Name             string                  `json:"name" yaml:"name"`
	Controller       string                  `json:"controller" yaml:"controller"`
	ManagedResources []ManagedResource       `json:"managedResources" yaml:"managedResources"`
	Dependencies     []string                `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	SteadyState      v1alpha1.SteadyStateSpec `json:"steadyState,omitempty" yaml:"steadyState,omitempty"`
	Webhooks         []WebhookSpec           `json:"webhooks,omitempty" yaml:"webhooks,omitempty"`
	Finalizers       []string                `json:"finalizers,omitempty" yaml:"finalizers,omitempty"`
}

// ManagedResource describes a Kubernetes resource managed by a component,
// including its identity, labels, owner reference, and expected spec fields.
type ManagedResource struct {
	APIVersion   string                 `json:"apiVersion" yaml:"apiVersion"`
	Kind         string                 `json:"kind" yaml:"kind"`
	Name         string                 `json:"name" yaml:"name"`
	Namespace    string                 `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Labels       map[string]string      `json:"labels,omitempty" yaml:"labels,omitempty"`
	OwnerRef     string                 `json:"ownerRef,omitempty" yaml:"ownerRef,omitempty"`
	ExpectedSpec map[string]any `json:"expectedSpec,omitempty" yaml:"expectedSpec,omitempty"`
}

// WebhookSpec describes a webhook associated with a component.
type WebhookSpec struct {
	Name string `json:"name" yaml:"name"`
	Type string `json:"type" yaml:"type"` // validating, mutating
	Path string `json:"path" yaml:"path"`
}

// RecoveryExpectations defines how long the framework should wait for
// operator recovery and how many reconcile cycles to tolerate.
type RecoveryExpectations struct {
	ReconcileTimeout   metav1.Duration `json:"reconcileTimeout" yaml:"reconcileTimeout"`
	MaxReconcileCycles int             `json:"maxReconcileCycles" yaml:"maxReconcileCycles"`
}

// GetComponent returns the component with the given name, or nil if not found.
func (k *OperatorKnowledge) GetComponent(name string) *ComponentModel {
	for i := range k.Components {
		if k.Components[i].Name == name {
			return &k.Components[i]
		}
	}
	return nil
}
