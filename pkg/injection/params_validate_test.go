package injection

import (
	"testing"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestValidateInjectionParams_PodKill(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type:  v1alpha1.PodKill,
		Count: 1,
		Parameters: map[string]string{
			"labelSelector": "app=dashboard",
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   2,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.NoError(t, err)
}

func TestValidateInjectionParams_PodKillMissingSelector(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type:  v1alpha1.PodKill,
		Count: 1,
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   2,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "labelSelector")
}

func TestValidateInjectionParams_PodKillExceedsBlastRadius(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type:  v1alpha1.PodKill,
		Count: 5,
		Parameters: map[string]string{
			"labelSelector": "app=dashboard",
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   2,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds blast radius")
}

func TestValidateInjectionParams_NetworkPartition(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.NetworkPartition,
		Parameters: map[string]string{
			"labelSelector": "app=dashboard",
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.NoError(t, err)
}

func TestValidateInjectionParams_NetworkPartitionMissingSelector(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.NetworkPartition,
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "labelSelector")
}

func TestValidateInjectionParams_CRDMutation(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.CRDMutation,
		Parameters: map[string]string{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"name":       "my-config",
			"field":      "replicas",
			"value":      "0",
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.NoError(t, err)
}

func TestValidateInjectionParams_CRDMutationMissingParams(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]string
		errText string
	}{
		{
			name:    "missing apiVersion",
			params:  map[string]string{"kind": "ConfigMap", "name": "my-config", "field": "replicas", "value": "0"},
			errText: "apiVersion",
		},
		{
			name:    "missing kind",
			params:  map[string]string{"apiVersion": "v1", "name": "my-config", "field": "replicas", "value": "0"},
			errText: "kind",
		},
		{
			name:    "missing name",
			params:  map[string]string{"apiVersion": "v1", "kind": "ConfigMap", "field": "replicas", "value": "0"},
			errText: "name",
		},
		{
			name:    "missing field",
			params:  map[string]string{"apiVersion": "v1", "kind": "ConfigMap", "name": "my-config", "value": "0"},
			errText: "field",
		},
		{
			name:    "missing value",
			params:  map[string]string{"apiVersion": "v1", "kind": "ConfigMap", "name": "my-config", "field": "replicas"},
			errText: "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := v1alpha1.InjectionSpec{
				Type:       v1alpha1.CRDMutation,
				Parameters: tt.params,
			}
			blast := v1alpha1.BlastRadiusSpec{
				MaxPodsAffected:   1,
				AllowedNamespaces: []string{"test"},
			}
			err := ValidateInjectionParams(spec, blast)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errText)
		})
	}
}

func TestValidateInjectionParams_ConfigDrift(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ConfigDrift,
		Parameters: map[string]string{
			"name":  "my-config",
			"key":   "settings.json",
			"value": "corrupted",
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.NoError(t, err)
}

func TestValidateInjectionParams_ConfigDriftMissingName(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ConfigDrift,
		Parameters: map[string]string{
			"key":   "settings.json",
			"value": "corrupted",
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestValidateInjectionParams_WebhookDisrupt(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.WebhookDisrupt,
		Parameters: map[string]string{
			"webhookName": "my-webhook",
			"action":      "setFailurePolicy",
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.NoError(t, err)
}

func TestValidateInjectionParams_WebhookDisruptMissingWebhookName(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.WebhookDisrupt,
		Parameters: map[string]string{
			"action": "setFailurePolicy",
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "webhookName")
}

func TestValidateInjectionParams_RBACRevoke(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.RBACRevoke,
		Parameters: map[string]string{
			"bindingName": "my-binding",
			"bindingType": "ClusterRoleBinding",
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.NoError(t, err)
}

func TestValidateInjectionParams_RBACRevokeMissingBindingName(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.RBACRevoke,
		Parameters: map[string]string{
			"bindingType": "ClusterRoleBinding",
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bindingName")
}

func TestValidateInjectionParams_FinalizerBlock(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.FinalizerBlock,
		Parameters: map[string]string{
			"kind": "ConfigMap",
			"name": "my-config",
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.NoError(t, err)
}

func TestValidateInjectionParams_FinalizerBlockMissingKind(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.FinalizerBlock,
		Parameters: map[string]string{
			"name": "my-config",
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "kind")
}

func TestValidateInjectionParams_UnknownType(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: "UnknownType",
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown injection type")
}

func TestValidateInjectionParams_ConfigDriftInvalidResourceType(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ConfigDrift,
		Parameters: map[string]string{
			"name":         "my-config",
			"key":          "settings.json",
			"value":        "corrupted",
			"resourceType": "Deployment",
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resourceType must be")
}

func TestValidateInjectionParams_WebhookDisruptUnsupportedAction(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.WebhookDisrupt,
		Parameters: map[string]string{
			"webhookName": "my-webhook",
			"action":      "deleteWebhook",
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported action")
}

func TestValidateInjectionParams_RBACRevokeInvalidBindingType(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.RBACRevoke,
		Parameters: map[string]string{
			"bindingName": "my-binding",
			"bindingType": "ServiceAccount",
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bindingType must be")
}
