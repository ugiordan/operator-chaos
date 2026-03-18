package injection

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"k8s.io/apimachinery/pkg/labels"
)

const maxNameLength = 253

var validNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9.\-]*[a-z0-9])?$`)
var validFieldPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_\-]*$`)

func validateK8sName(paramName, value string) error {
	if len(value) == 0 {
		return fmt.Errorf("%s must not be empty", paramName)
	}
	if len(value) > maxNameLength {
		return fmt.Errorf("%s exceeds maximum length of %d characters", paramName, maxNameLength)
	}
	if !validNamePattern.MatchString(value) {
		return fmt.Errorf("%s %q is not a valid Kubernetes name (must match RFC 1123 DNS subdomain)", paramName, value)
	}
	return nil
}

// validateLabelSelector checks that the labelSelector parameter in the injection
// spec is non-empty, parseable, and has at least one requirement to prevent
// accidentally matching all pods.
func validateLabelSelector(spec v1alpha1.InjectionSpec, injectorName string) error {
	selector := spec.Parameters["labelSelector"]
	if selector == "" {
		return fmt.Errorf("%s requires non-empty 'labelSelector' parameter", injectorName)
	}
	parsed, err := labels.Parse(selector)
	if err != nil {
		return fmt.Errorf("invalid labelSelector %q: %w", selector, err)
	}
	reqs, _ := parsed.Requirements()
	if len(reqs) == 0 {
		return fmt.Errorf("labelSelector must have at least one requirement to prevent matching all pods")
	}
	return nil
}

func ValidateInjectionParams(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error {
	if err := v1alpha1.ValidateInjectionType(spec.Type); err != nil {
		return err
	}

	switch spec.Type {
	case v1alpha1.PodKill:
		return validatePodKillParams(spec, blast)
	case v1alpha1.NetworkPartition:
		return validateNetworkPartitionParams(spec)
	case v1alpha1.CRDMutation:
		return validateCRDMutationParams(spec)
	case v1alpha1.ConfigDrift:
		return validateConfigDriftParams(spec)
	case v1alpha1.WebhookDisrupt:
		return validateWebhookDisruptParams(spec)
	case v1alpha1.RBACRevoke:
		return validateRBACRevokeParams(spec)
	case v1alpha1.FinalizerBlock:
		return validateFinalizerBlockParams(spec)
	case v1alpha1.ClientFault:
		return validateClientFaultParams(spec)
	}
	return nil
}

func validatePodKillParams(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error {
	count := spec.Count
	if count <= 0 {
		count = 1
	}
	if count > blast.MaxPodsAffected {
		return fmt.Errorf("pod kill count %d exceeds blast radius %d", count, blast.MaxPodsAffected)
	}
	return validateLabelSelector(spec, "PodKill")
}

func validateNetworkPartitionParams(spec v1alpha1.InjectionSpec) error {
	return validateLabelSelector(spec, "NetworkPartition")
}

func validateCRDMutationParams(spec v1alpha1.InjectionSpec) error {
	if _, ok := spec.Parameters["apiVersion"]; !ok {
		return fmt.Errorf("CRDMutation requires 'apiVersion' parameter")
	}
	if _, ok := spec.Parameters["kind"]; !ok {
		return fmt.Errorf("CRDMutation requires 'kind' parameter")
	}
	if _, ok := spec.Parameters["name"]; !ok {
		return fmt.Errorf("CRDMutation requires 'name' parameter")
	}
	if err := validateK8sName("name", spec.Parameters["name"]); err != nil {
		return err
	}
	if _, ok := spec.Parameters["field"]; !ok {
		return fmt.Errorf("CRDMutation requires 'field' parameter (JSON path to mutate)")
	}
	if err := validateFieldName("field", spec.Parameters["field"]); err != nil {
		return err
	}
	if _, ok := spec.Parameters["value"]; !ok {
		return fmt.Errorf("CRDMutation requires 'value' parameter (JSON value to set)")
	}
	return nil
}

func validateConfigDriftParams(spec v1alpha1.InjectionSpec) error {
	if _, ok := spec.Parameters["name"]; !ok {
		return fmt.Errorf("ConfigDrift requires 'name' parameter")
	}
	if err := validateK8sName("name", spec.Parameters["name"]); err != nil {
		return err
	}
	if _, ok := spec.Parameters["key"]; !ok {
		return fmt.Errorf("ConfigDrift requires 'key' parameter (data key to modify)")
	}
	if _, ok := spec.Parameters["value"]; !ok {
		return fmt.Errorf("ConfigDrift requires 'value' parameter (corrupted value)")
	}
	resourceType := spec.Parameters["resourceType"]
	if resourceType != "" && resourceType != "ConfigMap" && resourceType != "Secret" {
		return fmt.Errorf("ConfigDrift resourceType must be 'ConfigMap' or 'Secret', got %q", resourceType)
	}
	if resourceType == "Secret" {
		rollbackName := "chaos-rollback-" + spec.Parameters["name"] + "-" + spec.Parameters["key"]
		if len(rollbackName) > maxNameLength {
			return fmt.Errorf("rollback Secret name %q exceeds %d character limit", rollbackName, maxNameLength)
		}
	}
	return nil
}

func validateWebhookDisruptParams(spec v1alpha1.InjectionSpec) error {
	if _, ok := spec.Parameters["webhookName"]; !ok {
		return fmt.Errorf("WebhookDisrupt requires 'webhookName' parameter")
	}
	if err := validateK8sName("webhookName", spec.Parameters["webhookName"]); err != nil {
		return err
	}
	action, ok := spec.Parameters["action"]
	if !ok {
		return fmt.Errorf("WebhookDisrupt requires 'action' parameter")
	}
	if action != "setFailurePolicy" {
		return fmt.Errorf("unsupported action %q; supported actions: setFailurePolicy", action)
	}
	return nil
}

func validateRBACRevokeParams(spec v1alpha1.InjectionSpec) error {
	if _, ok := spec.Parameters["bindingName"]; !ok {
		return fmt.Errorf("RBACRevoke requires 'bindingName' parameter")
	}
	if err := validateK8sName("bindingName", spec.Parameters["bindingName"]); err != nil {
		return err
	}
	bindingType, ok := spec.Parameters["bindingType"]
	if !ok {
		return fmt.Errorf("RBACRevoke requires 'bindingType' parameter")
	}
	if bindingType != "ClusterRoleBinding" && bindingType != "RoleBinding" {
		return fmt.Errorf("RBACRevoke bindingType must be 'ClusterRoleBinding' or 'RoleBinding', got %q", bindingType)
	}
	return nil
}

func validateFinalizerBlockParams(spec v1alpha1.InjectionSpec) error {
	if _, ok := spec.Parameters["kind"]; !ok {
		return fmt.Errorf("FinalizerBlock requires 'kind' parameter")
	}
	if _, ok := spec.Parameters["name"]; !ok {
		return fmt.Errorf("FinalizerBlock requires 'name' parameter")
	}
	if err := validateK8sName("name", spec.Parameters["name"]); err != nil {
		return err
	}
	return nil
}

// validClientFaultOperations lists all SDK operations that can be fault-injected.
var validClientFaultOperations = map[string]bool{
	"get": true, "list": true, "create": true, "update": true,
	"delete": true, "patch": true, "deleteAllOf": true,
	"reconcile": true, "apply": true,
}

func validateClientFaultParams(spec v1alpha1.InjectionSpec) error {
	faultsJSON, ok := spec.Parameters["faults"]
	if !ok || faultsJSON == "" {
		return fmt.Errorf("ClientFault requires 'faults' parameter (JSON map of operation to fault spec)")
	}

	var faults map[string]struct {
		ErrorRate float64 `json:"errorRate"`
		Error     string  `json:"error"`
		Delay     string  `json:"delay,omitempty"`
		MaxDelay  string  `json:"maxDelay,omitempty"`
	}
	if err := json.Unmarshal([]byte(faultsJSON), &faults); err != nil {
		return fmt.Errorf("ClientFault: error parsing 'faults' parameter: %w", err)
	}

	if len(faults) == 0 {
		return fmt.Errorf("ClientFault 'faults' must contain at least one operation entry")
	}

	for op, spec := range faults {
		if !validClientFaultOperations[op] {
			return fmt.Errorf("ClientFault: unknown operation %q; valid operations: get, list, create, update, delete, patch, deleteAllOf, reconcile, apply", op)
		}
		if spec.ErrorRate < 0 || spec.ErrorRate > 1 {
			return fmt.Errorf("ClientFault: operation %q errorRate must be in [0.0, 1.0], got %f", op, spec.ErrorRate)
		}
		if spec.Delay != "" {
			if _, err := time.ParseDuration(spec.Delay); err != nil {
				return fmt.Errorf("ClientFault: operation %q has invalid delay %q: %w", op, spec.Delay, err)
			}
		}
		if spec.MaxDelay != "" {
			if _, err := time.ParseDuration(spec.MaxDelay); err != nil {
				return fmt.Errorf("ClientFault: operation %q has invalid maxDelay %q: %w", op, spec.MaxDelay, err)
			}
		}
	}

	if name := spec.Parameters["configMapName"]; name != "" {
		if err := validateK8sName("configMapName", name); err != nil {
			return err
		}
		if !strings.HasPrefix(name, "odh-chaos-") {
			return fmt.Errorf("ClientFault configMapName must start with 'odh-chaos-' prefix, got %q", name)
		}
	}

	return nil
}

func validateFieldName(paramName, value string) error {
	if len(value) == 0 {
		return fmt.Errorf("%s must not be empty", paramName)
	}
	if len(value) > maxNameLength {
		return fmt.Errorf("%s exceeds maximum length of %d characters", paramName, maxNameLength)
	}
	if !validFieldPattern.MatchString(value) {
		return fmt.Errorf("%s %q is not a valid field name", paramName, value)
	}
	return nil
}
