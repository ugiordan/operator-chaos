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

func TestValidateInjectionParams_ClientFault_Valid(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":0.3,"error":"throttled"}}`,
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.NoError(t, err)
}

func TestValidateInjectionParams_ClientFault_MissingFaults(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type:       v1alpha1.ClientFault,
		Parameters: map[string]string{},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "faults")
}

func TestValidateInjectionParams_ClientFault_InvalidJSON(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{not valid json}`,
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing")
}

func TestValidateInjectionParams_ClientFault_InvalidOperation(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"foobar":{"errorRate":0.5,"error":"bad"}}`,
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "foobar")
}

func TestValidateInjectionParams_ClientFault_InvalidErrorRate(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":1.5,"error":"too high"}}`,
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "errorRate")
}

func TestValidateInjectionParams_ClientFault_EmptyFaults(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{}`,
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one")
}

func TestValidateInjectionParams_ClientFault_NegativeErrorRate(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":-0.5,"error":"negative"}}`,
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "errorRate")
}

func TestValidateInjectionParams_ClientFault_ZeroErrorRate(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":0.0,"error":"no errors"}}`,
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.NoError(t, err)
}

func TestValidateInjectionParams_ClientFault_MaxErrorRate(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":1.0,"error":"always fail"}}`,
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.NoError(t, err)
}

func TestValidateInjectionParams_ClientFault_InvalidConfigMapName(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults":        `{"get":{"errorRate":0.5,"error":"test"}}`,
			"configMapName": "INVALID NAME!",
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "configMapName")
}

func TestValidateInjectionParams_ClientFault_ConfigMapNameMissingPrefix(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults":        `{"get":{"errorRate":0.5,"error":"test"}}`,
			"configMapName": "my-custom-config",
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "odh-chaos-")
}

func TestValidateInjectionParams_ClientFault_ValidDelay(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":0.3,"error":"slow","delay":"500ms","maxDelay":"2s"}}`,
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.NoError(t, err)
}

func TestValidateInjectionParams_ClientFault_InvalidDelay(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":0.3,"error":"slow","delay":"not-a-duration"}}`,
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}
	err := ValidateInjectionParams(spec, blast)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delay")
}
