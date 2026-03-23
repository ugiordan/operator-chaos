package injection

import (
	"strings"
	"testing"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestValidateK8sName_Valid(t *testing.T) {
	validNames := []string{
		"my-resource",
		"foo.bar",
		"a",
		"test-123",
		"my-config-map",
	}

	for _, name := range validNames {
		t.Run(name, func(t *testing.T) {
			err := validateK8sName("name", name)
			assert.NoError(t, err)
		})
	}
}

func TestValidateK8sName_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"uppercase mixed", "My-Resource"},
		{"underscore", "foo_bar"},
		{"leading dash", "-leading-dash"},
		{"trailing dash", "trailing-dash-"},
		{"has spaces", "has spaces"},
		{"all uppercase", "UPPERCASE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateK8sName("name", tt.value)
			assert.Error(t, err)
		})
	}
}

func TestValidateK8sName_TooLong(t *testing.T) {
	longName := strings.Repeat("a", 254)
	err := validateK8sName("name", longName)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum length")
}

func TestValidateFieldName_Valid(t *testing.T) {
	validFields := []string{
		"replicas",
		"my_field",
		"fieldName",
	}

	for _, field := range validFields {
		t.Run(field, func(t *testing.T) {
			err := validateFieldName("field", field)
			assert.NoError(t, err)
		})
	}
}

func TestValidateCRDMutationRejectsObjectValue(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.CRDMutation,
		Parameters: map[string]string{
			"apiVersion": "test.example.com/v1",
			"kind":       "TestResource",
			"name":       "my-resource",
			"field":      "config",
			"value":      `{"nested": "object"}`,
		},
	}
	err := validateCRDMutationParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scalar")
}

func TestValidateCRDMutationRejectsArrayValue(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.CRDMutation,
		Parameters: map[string]string{
			"apiVersion": "test.example.com/v1",
			"kind":       "TestResource",
			"name":       "my-resource",
			"field":      "items",
			"value":      `["a", "b"]`,
		},
	}
	err := validateCRDMutationParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scalar")
}

func TestValidateFinalizerBlockCoreTypeRequiresDangerHigh(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.FinalizerBlock,
		Parameters: map[string]string{
			"kind": "Secret",
			"name": "my-secret",
		},
	}
	err := validateFinalizerBlockParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dangerLevel: high")
}

func TestValidateFinalizerBlockCoreTypeWithDangerHigh(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type:        v1alpha1.FinalizerBlock,
		DangerLevel: v1alpha1.DangerLevelHigh,
		Parameters: map[string]string{
			"kind": "Secret",
			"name": "my-secret",
		},
	}
	err := validateFinalizerBlockParams(spec)
	assert.NoError(t, err)
}

func TestValidateConfigDriftPrefixBlocking(t *testing.T) {
	prefixes := []string{"etcd-certs", "kube-apiserver-pod", "openshift-service-ca-signing"}
	for _, name := range prefixes {
		t.Run(name, func(t *testing.T) {
			spec := v1alpha1.InjectionSpec{
				Type: v1alpha1.ConfigDrift,
				Parameters: map[string]string{
					"name":  name,
					"key":   "data",
					"value": "corrupted",
				},
			}
			err := validateConfigDriftParams(spec)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "system-critical")
		})
	}
}

func TestValidateConfigDriftExpandedDenyList(t *testing.T) {
	newEntries := []string{"kubeadm-config", "kubelet-config", "kube-apiserver", "scheduler-config"}
	for _, name := range newEntries {
		t.Run(name, func(t *testing.T) {
			spec := v1alpha1.InjectionSpec{
				Type: v1alpha1.ConfigDrift,
				Parameters: map[string]string{
					"name":  name,
					"key":   "data",
					"value": "corrupted",
				},
			}
			err := validateConfigDriftParams(spec)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "system-critical")
		})
	}
}

func TestValidateTargetSpec_Valid(t *testing.T) {
	target := v1alpha1.TargetSpec{
		Operator:  "my-operator",
		Component: "my-component",
	}
	err := ValidateTargetSpec(target)
	assert.NoError(t, err)
}

func TestValidateTargetSpec_InvalidOperator(t *testing.T) {
	target := v1alpha1.TargetSpec{
		Operator:  "INVALID NAME!",
		Component: "my-component",
	}
	err := ValidateTargetSpec(target)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "operator")
}

func TestValidateTargetSpec_EmptyComponent(t *testing.T) {
	target := v1alpha1.TargetSpec{
		Operator:  "my-operator",
		Component: "",
	}
	err := ValidateTargetSpec(target)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "component")
}

func TestValidateCRDMutationRejectsNullValue(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.CRDMutation,
		Parameters: map[string]string{
			"apiVersion": "test.example.com/v1",
			"kind":       "TestResource",
			"name":       "my-resource",
			"field":      "config",
			"value":      "null",
		},
	}
	err := validateCRDMutationParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "null")
	assert.Contains(t, err.Error(), "dangerLevel: high")
}

func TestValidateCRDMutationAllowsNullWithDangerHigh(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type:        v1alpha1.CRDMutation,
		DangerLevel: v1alpha1.DangerLevelHigh,
		Parameters: map[string]string{
			"apiVersion": "test.example.com/v1",
			"kind":       "TestResource",
			"name":       "my-resource",
			"field":      "config",
			"value":      "null",
		},
	}
	err := validateCRDMutationParams(spec)
	assert.NoError(t, err)
}

func TestValidateCRDMutationRejectsSensitiveField(t *testing.T) {
	sensitiveFields := []string{
		"replicas", "serviceAccountName", "hostNetwork", "securityContext",
		"volumes", "containers", "nodeName",
	}
	for _, field := range sensitiveFields {
		t.Run(field, func(t *testing.T) {
			spec := v1alpha1.InjectionSpec{
				Type: v1alpha1.CRDMutation,
				Parameters: map[string]string{
					"apiVersion": "test.example.com/v1",
					"kind":       "TestResource",
					"name":       "my-resource",
					"field":      field,
					"value":      "0",
				},
			}
			err := validateCRDMutationParams(spec)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "sensitive field")
			assert.Contains(t, err.Error(), "dangerLevel: high")
		})
	}
}

func TestValidateCRDMutationAllowsSensitiveFieldWithDangerHigh(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type:        v1alpha1.CRDMutation,
		DangerLevel: v1alpha1.DangerLevelHigh,
		Parameters: map[string]string{
			"apiVersion": "test.example.com/v1",
			"kind":       "TestResource",
			"name":       "my-resource",
			"field":      "replicas",
			"value":      "0",
		},
	}
	err := validateCRDMutationParams(spec)
	assert.NoError(t, err)
}

func TestValidateClientFaultDelayUpperBound(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":0.5,"error":"test","delay":"10m"}}`,
		},
	}
	err := validateClientFaultParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

func TestValidateClientFaultMaxDelayUpperBound(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":0.5,"error":"test","maxDelay":"6m"}}`,
		},
	}
	err := validateClientFaultParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

func TestValidateClientFaultDelayWithinBound(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ClientFault,
		Parameters: map[string]string{
			"faults": `{"get":{"errorRate":0.5,"error":"test","delay":"4m"}}`,
		},
	}
	err := validateClientFaultParams(spec)
	assert.NoError(t, err)
}

func TestIsCoreKubernetesTypeIncludesApiextensions(t *testing.T) {
	assert.True(t, isCoreKubernetesType("apiextensions.k8s.io/v1"))
	assert.True(t, isCoreKubernetesType("apiextensions.k8s.io/v1beta1"))
	assert.True(t, isCoreKubernetesType("apiregistration.k8s.io/v1"))
	assert.False(t, isCoreKubernetesType("custom.example.com/v1"))
}

func TestValidateWebhookDisruptGatekeeperMutating(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.WebhookDisrupt,
		Parameters: map[string]string{
			"webhookName": "gatekeeper-mutating-webhook-configuration",
			"action":      "setFailurePolicy",
		},
	}
	err := validateWebhookDisruptParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "system-critical webhook")
}

func TestValidateConfigDriftSecretDenyList(t *testing.T) {
	secrets := []string{"etcd-client", "apiserver-etcd-client", "service-account-key", "oauth-serving-cert"}
	for _, name := range secrets {
		t.Run(name, func(t *testing.T) {
			spec := v1alpha1.InjectionSpec{
				Type:        v1alpha1.ConfigDrift,
				DangerLevel: v1alpha1.DangerLevelHigh,
				Parameters: map[string]string{
					"name":         name,
					"key":          "data",
					"value":        "corrupted",
					"resourceType": "Secret",
				},
			}
			err := validateConfigDriftParams(spec)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "system-critical Secret")
		})
	}
}

func TestValidateConfigDriftSecretRequiresDangerLevelHigh(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ConfigDrift,
		Parameters: map[string]string{
			"name":         "my-secret",
			"key":          "data",
			"value":        "corrupted",
			"resourceType": "Secret",
		},
	}
	err := validateConfigDriftParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dangerLevel: high")
}

func TestValidateConfigDriftSecretNotInConfigMapDenyList(t *testing.T) {
	// "coredns" is in the ConfigMap deny-list but not the Secret deny-list
	spec := v1alpha1.InjectionSpec{
		Type:        v1alpha1.ConfigDrift,
		DangerLevel: v1alpha1.DangerLevelHigh,
		Parameters: map[string]string{
			"name":         "coredns",
			"key":          "data",
			"value":        "corrupted",
			"resourceType": "Secret",
		},
	}
	err := validateConfigDriftParams(spec)
	assert.NoError(t, err)
}

func TestValidateCRDMutationSelfModificationBlocked(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type:        v1alpha1.CRDMutation,
		DangerLevel: v1alpha1.DangerLevelHigh,
		Parameters: map[string]string{
			"apiVersion": "chaos.opendatahub.io/v1alpha1",
			"kind":       "ChaosExperiment",
			"name":       "my-experiment",
			"field":      "config",
			"value":      "modified",
		},
	}
	err := validateCRDMutationParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "self-modification")
}

func TestValidateCRDMutationOversizedValue(t *testing.T) {
	largeValue := strings.Repeat("x", maxParameterValueLength+1)
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.CRDMutation,
		Parameters: map[string]string{
			"apiVersion": "test.example.com/v1",
			"kind":       "TestResource",
			"name":       "my-resource",
			"field":      "config",
			"value":      largeValue,
		},
	}
	err := validateCRDMutationParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum length")
}

func TestValidateConfigDriftOversizedValue(t *testing.T) {
	largeValue := strings.Repeat("x", maxParameterValueLength+1)
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ConfigDrift,
		Parameters: map[string]string{
			"name":  "my-configmap",
			"key":   "config.yaml",
			"value": largeValue,
		},
	}
	err := validateConfigDriftParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum length")
}

func TestValidateCRDMutationRejectsChaosManagedResource(t *testing.T) {
	prefixes := []string{"chaos-rollback-my-cr", "chaos-result-my-cr", "odh-chaos-my-cr"}
	for _, name := range prefixes {
		t.Run(name, func(t *testing.T) {
			spec := v1alpha1.InjectionSpec{
				Type:        v1alpha1.CRDMutation,
				DangerLevel: v1alpha1.DangerLevelHigh,
				Parameters: map[string]string{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"name":       name,
					"field":      "data",
					"value":      "test",
				},
			}
			err := validateCRDMutationParams(spec)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "chaos-managed resource")
		})
	}
}

func TestValidateConfigDriftRejectsChaosManagedResource(t *testing.T) {
	prefixes := []string{"chaos-rollback-my-cm", "chaos-result-my-cm", "odh-chaos-my-cm"}
	for _, name := range prefixes {
		t.Run(name, func(t *testing.T) {
			spec := v1alpha1.InjectionSpec{
				Type: v1alpha1.ConfigDrift,
				Parameters: map[string]string{
					"name":  name,
					"key":   "data",
					"value": "test",
				},
			}
			err := validateConfigDriftParams(spec)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "chaos-managed resource")
		})
	}
}

func TestValidateFinalizerBlockRejectsChaosManagedResource(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.FinalizerBlock,
		Parameters: map[string]string{
			"kind": "Deployment",
			"name": "chaos-rollback-my-deploy",
		},
	}
	err := validateFinalizerBlockParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "chaos-managed resource")
}

func TestValidateRBACRevokeRejectsChaosManagedResource(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.RBACRevoke,
		Parameters: map[string]string{
			"bindingName": "chaos-rollback-my-binding",
			"bindingType": "RoleBinding",
		},
	}
	err := validateRBACRevokeParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "chaos-managed resource")
}

func TestValidateRBACRevokeRejectsSystemCriticalBinding(t *testing.T) {
	bindings := []string{"cluster-admin", "system:node", "system:kube-controller-manager"}
	for _, name := range bindings {
		t.Run(name, func(t *testing.T) {
			spec := v1alpha1.InjectionSpec{
				Type: v1alpha1.RBACRevoke,
				Parameters: map[string]string{
					"bindingName": name,
					"bindingType": "ClusterRoleBinding",
				},
			}
			err := validateRBACRevokeParams(spec)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "not allowed")
		})
	}
}

func TestValidateWebhookDisruptRejectsSystemPrefix(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.WebhookDisrupt,
		Parameters: map[string]string{
			"webhookName": "system:some-webhook",
			"action":      "setFailurePolicy",
		},
	}
	err := validateWebhookDisruptParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "system webhook")
}

func TestValidateLabelSelectorParseError(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type:  v1alpha1.PodKill,
		Count: 1,
		Parameters: map[string]string{
			"labelSelector": "invalid===selector",
		},
	}
	err := validateLabelSelector(spec, "PodKill")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid labelSelector")
}

func TestValidateConfigDriftRollbackSecretNameTooLong(t *testing.T) {
	longName := strings.Repeat("a", 240)
	spec := v1alpha1.InjectionSpec{
		Type:        v1alpha1.ConfigDrift,
		DangerLevel: v1alpha1.DangerLevelHigh,
		Parameters: map[string]string{
			"name":         longName,
			"key":          "some-key",
			"value":        "corrupted",
			"resourceType": "Secret",
		},
	}
	err := validateConfigDriftParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
}

func TestValidateRBACRevokeMissingBindingType(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.RBACRevoke,
		Parameters: map[string]string{
			"bindingName": "my-binding",
		},
	}
	err := validateRBACRevokeParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bindingType")
}

func TestValidateRBACRevokeInvalidBindingType(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.RBACRevoke,
		Parameters: map[string]string{
			"bindingName": "my-binding",
			"bindingType": "ConfigMap",
		},
	}
	err := validateRBACRevokeParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bindingType")
}

func TestValidateConfigDriftConfigMapResourceType(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.ConfigDrift,
		Parameters: map[string]string{
			"name":         "my-configmap",
			"key":          "config.yaml",
			"value":        "corrupted",
			"resourceType": "ConfigMap",
		},
	}
	err := validateConfigDriftParams(spec)
	assert.NoError(t, err)
}

func TestValidateFieldName_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"starts with digit", "123start"},
		{"leading dot", ".leading-dot"},
		{"has spaces", "has spaces"},
		{"dot path", "spec.replicas"},
		{"nested dots", "a.b.c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFieldName("field", tt.value)
			assert.Error(t, err)
		})
	}
}

func TestValidateFinalizerBlockRejectsChaosExperimentCR(t *testing.T) {
	tests := []struct {
		name       string
		apiVersion string
		wantErr    bool
	}{
		{"chaos apiVersion", "chaos.opendatahub.io/v1alpha1", true},
		{"empty apiVersion", "", true},
		{"non-chaos apiVersion", "apps/v1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := v1alpha1.InjectionSpec{
				Type: v1alpha1.FinalizerBlock,
				Parameters: map[string]string{
					"kind":       "ChaosExperiment",
					"apiVersion": tt.apiVersion,
					"name":       "my-experiment",
				},
			}
			err := validateFinalizerBlockParams(spec)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "self-sabotage")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateFinalizerBlockRejectsCleanupFinalizerName(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.FinalizerBlock,
		Parameters: map[string]string{
			"kind":          "Deployment",
			"apiVersion":    "apps/v1",
			"name":          "my-deploy",
			"finalizerName": "chaos.opendatahub.io/cleanup",
		},
	}
	err := validateFinalizerBlockParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cleanup finalizer")
}

func TestValidateConfigDriftSecretKeySlashRejected(t *testing.T) {
	spec := v1alpha1.InjectionSpec{
		Type:        v1alpha1.ConfigDrift,
		DangerLevel: v1alpha1.DangerLevelHigh,
		Parameters: map[string]string{
			"name":         "my-secret",
			"key":          "../../etc/passwd",
			"value":        "corrupted",
			"resourceType": "Secret",
		},
	}
	err := validateConfigDriftParams(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid characters")
}
