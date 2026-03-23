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

// maxParameterValueLength limits user-provided parameter values to prevent
// etcd size exhaustion via oversized annotations or ConfigMap data.
const maxParameterValueLength = 65536 // 64KB

// chaosConfigMapPrefix is the required prefix for ClientFault ConfigMap names.
const chaosConfigMapPrefix = "odh-chaos-"

// chaosManagedPrefixes are resource name prefixes used by the chaos framework.
// Targeting these resources with chaos experiments is forbidden to prevent
// self-destruction or rollback corruption.
var chaosManagedPrefixes = []string{"chaos-rollback-", "chaos-result-", "odh-chaos-", "chaos-controller-"}

var validNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9.\-]*[a-z0-9])?$`)
var validFieldPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_\-]*$`)

// ValidateTargetSpec validates the target spec fields are valid Kubernetes names.
func ValidateTargetSpec(target v1alpha1.TargetSpec) error {
	if err := validateK8sName("operator", target.Operator); err != nil {
		return err
	}
	if err := validateK8sName("component", target.Component); err != nil {
		return err
	}
	return nil
}

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
	default:
		return fmt.Errorf("no parameter validation implemented for injection type %q", spec.Type)
	}
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
	apiVersion, ok := spec.Parameters["apiVersion"]
	if !ok || apiVersion == "" {
		return fmt.Errorf("CRDMutation requires non-empty 'apiVersion' parameter")
	}
	kind := spec.Parameters["kind"]
	if kind == "" {
		return fmt.Errorf("CRDMutation requires non-empty 'kind' parameter")
	}
	if kind == "ChaosExperiment" && strings.Contains(apiVersion, "chaos.opendatahub.io") {
		return fmt.Errorf("CRDMutation cannot target ChaosExperiment CRs (self-modification is forbidden)")
	}
	if _, ok := spec.Parameters["name"]; !ok {
		return fmt.Errorf("CRDMutation requires 'name' parameter")
	}
	if err := validateK8sName("name", spec.Parameters["name"]); err != nil {
		return err
	}
	if err := rejectChaosManagedResource("CRDMutation", spec.Parameters["name"]); err != nil {
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

	// Reject oversized values that could exhaust etcd annotation limits.
	value := spec.Parameters["value"]
	if len(value) > maxParameterValueLength {
		return fmt.Errorf("CRDMutation 'value' exceeds maximum length of %d bytes", maxParameterValueLength)
	}

	// Reject JSON objects and arrays as values via actual JSON parsing.
	var jsonProbe any
	if json.Unmarshal([]byte(value), &jsonProbe) == nil {
		switch jsonProbe.(type) {
		case map[string]any, []any:
			return fmt.Errorf("CRDMutation 'value' must be a scalar (string, number, boolean), not a JSON object or array")
		}
	}

	// Reject null value without dangerLevel: high (causes field deletion via merge patch)
	if strings.TrimSpace(value) == "null" && spec.DangerLevel != v1alpha1.DangerLevelHigh {
		return fmt.Errorf("CRDMutation 'value' of 'null' causes field deletion via merge patch; requires dangerLevel: high")
	}

	// Reject sensitive spec fields without dangerLevel: high
	field := spec.Parameters["field"]
	if sensitiveSpecFields[field] && spec.DangerLevel != v1alpha1.DangerLevelHigh {
		return fmt.Errorf("CRDMutation targeting sensitive field %q requires dangerLevel: high", field)
	}

	// Block core Kubernetes types unless the experiment has dangerLevel: high
	if isCoreKubernetesType(apiVersion) {
		if spec.DangerLevel != v1alpha1.DangerLevelHigh {
			return fmt.Errorf("CRDMutation targeting core Kubernetes type (apiVersion=%s) requires dangerLevel: high", apiVersion)
		}
	}

	return nil
}

// sensitiveSpecFields is a deny-list of spec fields that require dangerLevel: high
// for CRDMutation experiments.
var sensitiveSpecFields = map[string]bool{
	"replicas":           true,
	"serviceAccountName": true,
	"serviceAccount":     true,
	"hostNetwork":        true,
	"hostPID":            true,
	"hostIPC":            true,
	"securityContext":    true,
	"volumes":            true,
	"containers":         true,
	"initContainers":     true,
	"nodeName":           true,
	"nodeSelector":       true,
	"template":           true,
	"selector":           true,
	"strategy":           true,
	"updateStrategy":     true,
	"affinity":           true,
	"tolerations":        true,
	"priorityClassName":              true,
	"runtimeClassName":               true,
	"automountServiceAccountToken":   true,
	"dnsPolicy":                      true,
	"shareProcessNamespace":          true,
	"schedulerName":                  true,
}

// isCoreKubernetesType returns true if the apiVersion represents a core
// Kubernetes type rather than a CRD.
func isCoreKubernetesType(apiVersion string) bool {
	// Core types: v1, apps/v1, batch/v1, etc.
	return apiVersion == "v1" ||
		strings.HasPrefix(apiVersion, "apps/") ||
		strings.HasPrefix(apiVersion, "batch/") ||
		strings.HasPrefix(apiVersion, "networking.k8s.io/") ||
		strings.HasPrefix(apiVersion, "policy/") ||
		strings.HasPrefix(apiVersion, "rbac.authorization.k8s.io/") ||
		strings.HasPrefix(apiVersion, "storage.k8s.io/") ||
		strings.HasPrefix(apiVersion, "admissionregistration.k8s.io/") ||
		strings.HasPrefix(apiVersion, "authentication.k8s.io/") ||
		strings.HasPrefix(apiVersion, "authorization.k8s.io/") ||
		strings.HasPrefix(apiVersion, "autoscaling/") ||
		strings.HasPrefix(apiVersion, "certificates.k8s.io/") ||
		strings.HasPrefix(apiVersion, "coordination.k8s.io/") ||
		strings.HasPrefix(apiVersion, "discovery.k8s.io/") ||
		strings.HasPrefix(apiVersion, "events.k8s.io/") ||
		strings.HasPrefix(apiVersion, "flowcontrol.apiserver.k8s.io/") ||
		strings.HasPrefix(apiVersion, "node.k8s.io/") ||
		strings.HasPrefix(apiVersion, "scheduling.k8s.io/") ||
		strings.HasPrefix(apiVersion, "apiextensions.k8s.io/") ||
		strings.HasPrefix(apiVersion, "apiregistration.k8s.io/")
}

// systemCriticalConfigs is a deny-list of ConfigMaps/Secrets that should never
// be targeted by chaos experiments.
var systemCriticalConfigs = map[string]bool{
	"kube-root-ca.crt":                      true,
	"coredns":                               true,
	"kube-proxy":                            true,
	"cluster-info":                          true,
	"extension-apiserver-authentication":    true,
	"kubeadm-config":                        true,
	"kubelet-config":                        true,
	"kube-apiserver":                        true,
	"scheduler-config":                      true,
}

// systemCriticalSecrets is a deny-list of Secrets that should never be targeted
// by chaos experiments.
var systemCriticalSecrets = map[string]bool{
	"etcd-client":           true,
	"apiserver-etcd-client": true,
	"service-account-key":   true,
	"oauth-serving-cert":    true,
}

// systemCriticalConfigPrefixes are name prefixes that indicate system-critical configs.
var systemCriticalConfigPrefixes = []string{"etcd-", "kube-apiserver-", "openshift-service-ca"}

func validateConfigDriftParams(spec v1alpha1.InjectionSpec) error {
	if _, ok := spec.Parameters["name"]; !ok {
		return fmt.Errorf("ConfigDrift requires 'name' parameter")
	}
	if err := validateK8sName("name", spec.Parameters["name"]); err != nil {
		return err
	}

	// Reject chaos-managed resources to prevent rollback corruption
	name := spec.Parameters["name"]
	if err := rejectChaosManagedResource("ConfigDrift", name); err != nil {
		return err
	}

	// Reject system-critical resources based on resourceType
	resourceType := spec.Parameters["resourceType"]
	if resourceType == "Secret" {
		// Targeting Secrets creates plaintext rollback copies. Require dangerLevel: high
		// to acknowledge that original Secret values will be stored in a rollback Secret.
		if spec.DangerLevel != v1alpha1.DangerLevelHigh {
			return fmt.Errorf("ConfigDrift targeting Secrets requires dangerLevel: high (rollback stores original values in plaintext)")
		}
		if systemCriticalSecrets[name] {
			return fmt.Errorf("ConfigDrift cannot target system-critical Secret %q", name)
		}
	} else {
		if systemCriticalConfigs[name] {
			return fmt.Errorf("targeting system-critical config %q is not allowed", name)
		}
	}

	// Block system-critical config prefixes for both types.
	for _, prefix := range systemCriticalConfigPrefixes {
		if strings.HasPrefix(name, prefix) {
			return fmt.Errorf("ConfigDrift cannot target system-critical config %q (matches prefix %q)", name, prefix)
		}
	}

	if _, ok := spec.Parameters["key"]; !ok {
		return fmt.Errorf("ConfigDrift requires 'key' parameter (data key to modify)")
	}
	if _, ok := spec.Parameters["value"]; !ok {
		return fmt.Errorf("ConfigDrift requires 'value' parameter (corrupted value)")
	}
	if len(spec.Parameters["value"]) > maxParameterValueLength {
		return fmt.Errorf("ConfigDrift 'value' exceeds maximum length of %d bytes", maxParameterValueLength)
	}
	if resourceType != "" && resourceType != "ConfigMap" && resourceType != "Secret" {
		return fmt.Errorf("ConfigDrift resourceType must be 'ConfigMap' or 'Secret', got %q", resourceType)
	}
	if resourceType == "Secret" {
		key := spec.Parameters["key"]
		if strings.Contains(key, "/") || strings.Contains(key, "..") {
			return fmt.Errorf("ConfigDrift 'key' contains invalid characters for rollback Secret name construction")
		}
		rollbackName := "chaos-rollback-" + spec.Parameters["name"] + "-" + key
		if len(rollbackName) > maxNameLength {
			return fmt.Errorf("rollback Secret name %q exceeds %d character limit", rollbackName, maxNameLength)
		}
	}
	return nil
}

func validateWebhookDisruptParams(spec v1alpha1.InjectionSpec) error {
	webhookName, ok := spec.Parameters["webhookName"]
	if !ok {
		return fmt.Errorf("WebhookDisrupt requires 'webhookName' parameter")
	}

	// Check deny-lists and prefix blocks BEFORE K8s name validation, because
	// some system webhook names may contain characters invalid for DNS subdomains.
	if systemCriticalWebhooks[webhookName] {
		return fmt.Errorf("targeting system-critical webhook %q is not allowed", webhookName)
	}
	if strings.HasPrefix(webhookName, "system:") {
		return fmt.Errorf("targeting system webhook %q is not allowed", webhookName)
	}
	if strings.HasPrefix(webhookName, "openshift-") {
		return fmt.Errorf("targeting OpenShift webhook %q is not allowed", webhookName)
	}

	if err := validateK8sName("webhookName", webhookName); err != nil {
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

// systemCriticalWebhooks is a deny-list of webhooks that should never be
// targeted by chaos experiments.
var systemCriticalWebhooks = map[string]bool{
	"validate-policy.kyverno.io":                  true,
	"gatekeeper-validating-webhook-configuration": true,
	"gatekeeper-mutating-webhook-configuration":   true,
	"cert-manager-webhook":                        true,
	"istio-sidecar-injector":                      true,
}

func validateRBACRevokeParams(spec v1alpha1.InjectionSpec) error {
	bindingName, ok := spec.Parameters["bindingName"]
	if !ok {
		return fmt.Errorf("RBACRevoke requires 'bindingName' parameter")
	}
	bindingType, ok := spec.Parameters["bindingType"]
	if !ok {
		return fmt.Errorf("RBACRevoke requires 'bindingType' parameter")
	}
	if bindingType != "ClusterRoleBinding" && bindingType != "RoleBinding" {
		return fmt.Errorf("RBACRevoke bindingType must be 'ClusterRoleBinding' or 'RoleBinding', got %q", bindingType)
	}

	// Check deny-lists and prefix blocks BEFORE K8s name validation, because
	// system binding names (e.g. "system:node") contain ":" which is valid for
	// ClusterRoleBinding names but fails DNS subdomain validation.
	if bindingType == "ClusterRoleBinding" {
		if systemCriticalBindings[bindingName] {
			return fmt.Errorf("targeting system-critical ClusterRoleBinding %q is not allowed", bindingName)
		}
	}
	if strings.HasPrefix(bindingName, "system:") {
		return fmt.Errorf("targeting system binding %q is not allowed", bindingName)
	}

	if err := validateK8sName("bindingName", bindingName); err != nil {
		return err
	}
	if err := rejectChaosManagedResource("RBACRevoke", bindingName); err != nil {
		return err
	}

	return nil
}

// systemCriticalBindings is a deny-list of ClusterRoleBindings that should
// never be targeted by chaos experiments.
var systemCriticalBindings = map[string]bool{
	"cluster-admin":                  true,
	"system:node":                    true,
	"system:kube-controller-manager": true,
	"system:kube-scheduler":          true,
	"system:volume-scheduler":        true,
	"system:kube-dns":                true,
	"system:discovery":               true,
	"system:basic-user":              true,
	"system:public-info-viewer":      true,
}

// finalizerBlockForbiddenKinds is a deny-list of resource kinds that should
// never be targeted by FinalizerBlock experiments.
var finalizerBlockForbiddenKinds = map[string]bool{
	"Namespace": true,
	"Node":      true,
}

func validateFinalizerBlockParams(spec v1alpha1.InjectionSpec) error {
	kind, ok := spec.Parameters["kind"]
	if !ok || kind == "" {
		return fmt.Errorf("FinalizerBlock requires non-empty 'kind' parameter")
	}

	// Reject forbidden resource kinds
	if finalizerBlockForbiddenKinds[kind] {
		return fmt.Errorf("FinalizerBlock targeting %q resources is not allowed", kind)
	}

	// Reject self-targeting of ChaosExperiment CRs to prevent deadlock:
	// a stuck finalizer on a ChaosExperiment would block its own deletion/cleanup.
	apiVersion := spec.Parameters["apiVersion"]
	if kind == "ChaosExperiment" && (apiVersion == "" || strings.Contains(apiVersion, "chaos.opendatahub.io")) {
		return fmt.Errorf("FinalizerBlock cannot target ChaosExperiment CRs (self-sabotage is forbidden)")
	}

	if _, ok := spec.Parameters["name"]; !ok {
		return fmt.Errorf("FinalizerBlock requires 'name' parameter")
	}
	if err := validateK8sName("name", spec.Parameters["name"]); err != nil {
		return err
	}
	if err := rejectChaosManagedResource("FinalizerBlock", spec.Parameters["name"]); err != nil {
		return err
	}

	// Reject targeting resources with the controller's cleanup finalizer name
	if spec.Parameters["finalizerName"] == "chaos.opendatahub.io/cleanup" {
		return fmt.Errorf("FinalizerBlock cannot use finalizer name %q (conflicts with controller cleanup finalizer)", spec.Parameters["finalizerName"])
	}

	// Core K8s types require dangerLevel: high.
	coreKinds := map[string]bool{
		"Secret": true, "ConfigMap": true, "ServiceAccount": true,
		"PersistentVolume": true, "PersistentVolumeClaim": true,
		"Service": true, "Endpoints": true,
	}
	if apiVersion == "" || apiVersion == "v1" {
		if coreKinds[kind] {
			if spec.DangerLevel != v1alpha1.DangerLevelHigh {
				return fmt.Errorf("FinalizerBlock targeting core Kubernetes type %q requires dangerLevel: high", kind)
			}
		}
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

	const maxClientFaultDelay = 5 * time.Minute

	for op, spec := range faults {
		if !validClientFaultOperations[op] {
			return fmt.Errorf("ClientFault: unknown operation %q; valid operations: get, list, create, update, delete, patch, deleteAllOf, reconcile, apply", op)
		}
		if spec.ErrorRate < 0 || spec.ErrorRate > 1 {
			return fmt.Errorf("ClientFault: operation %q errorRate must be in [0.0, 1.0], got %f", op, spec.ErrorRate)
		}
		if spec.Delay != "" {
			d, err := time.ParseDuration(spec.Delay)
			if err != nil {
				return fmt.Errorf("ClientFault: operation %q has invalid delay %q: %w", op, spec.Delay, err)
			}
			if d > maxClientFaultDelay {
				return fmt.Errorf("ClientFault: operation %q delay %v exceeds maximum allowed %v", op, d, maxClientFaultDelay)
			}
		}
		if spec.MaxDelay != "" {
			d, err := time.ParseDuration(spec.MaxDelay)
			if err != nil {
				return fmt.Errorf("ClientFault: operation %q has invalid maxDelay %q: %w", op, spec.MaxDelay, err)
			}
			if d > maxClientFaultDelay {
				return fmt.Errorf("ClientFault: operation %q maxDelay %v exceeds maximum allowed %v", op, d, maxClientFaultDelay)
			}
		}
	}

	if name := spec.Parameters["configMapName"]; name != "" {
		if err := validateK8sName("configMapName", name); err != nil {
			return err
		}
		if !strings.HasPrefix(name, chaosConfigMapPrefix) {
			return fmt.Errorf("ClientFault configMapName must start with '%s' prefix, got %q", chaosConfigMapPrefix, name)
		}
	}

	return nil
}

// rejectChaosManagedResource returns an error if the given name matches a
// chaos-managed resource prefix, preventing experiments from corrupting
// rollback data, result ConfigMaps, or other chaos infrastructure.
func rejectChaosManagedResource(injType, name string) error {
	for _, prefix := range chaosManagedPrefixes {
		if strings.HasPrefix(name, prefix) {
			return fmt.Errorf("%s cannot target chaos-managed resource %q (prefix %q is reserved)", injType, name, prefix)
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
