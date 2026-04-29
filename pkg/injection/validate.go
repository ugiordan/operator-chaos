package injection

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	v1alpha1 "github.com/opendatahub-io/operator-chaos/api/v1alpha1"
	"k8s.io/apimachinery/pkg/labels"
)

const maxNameLength = 253

// maxParameterValueLength limits user-provided parameter values to prevent
// etcd size exhaustion via oversized annotations or ConfigMap data.
const maxParameterValueLength = 65536 // 64KB

// chaosConfigMapPrefix is the required prefix for ClientFault ConfigMap names.
const chaosConfigMapPrefix = "operator-chaos-"

// chaosManagedPrefixes are resource name prefixes used by the chaos framework.
// Targeting these resources with chaos experiments is forbidden to prevent
// self-destruction or rollback corruption.
var chaosManagedPrefixes = []string{"chaos-rollback-", "chaos-result-", "operator-chaos-", "chaos-controller-"}

var validNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9.\-]*[a-z0-9])?$`)
var validFieldPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_\-]*$`)
var validPathSegmentPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_\-]*$`)

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
	case v1alpha1.OwnerRefOrphan:
		return validateOwnerRefOrphanParams(spec)
	case v1alpha1.QuotaExhaustion:
		return validateQuotaExhaustionParams(spec)
	case v1alpha1.WebhookLatency:
		return validateWebhookLatencyParams(spec)
	case v1alpha1.NamespaceDeletion:
		return validateNamespaceDeletionParams(spec)
	case v1alpha1.LabelStomping:
		return validateLabelStompingParams(spec)
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
	if kind == "ChaosExperiment" && strings.Contains(apiVersion, "chaos.operatorchaos.io") {
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

	// Accept either 'path' (dot-notation like "spec.replicas") or 'field' (legacy, implies spec.{field}).
	// Both cannot be set simultaneously.
	hasField := spec.Parameters["field"] != ""
	hasPath := spec.Parameters["path"] != ""
	if hasField && hasPath {
		return fmt.Errorf("CRDMutation accepts either 'field' or 'path' parameter, not both")
	}
	if !hasField && !hasPath {
		return fmt.Errorf("CRDMutation requires 'field' or 'path' parameter")
	}

	if hasField {
		if err := validateFieldName("field", spec.Parameters["field"]); err != nil {
			return err
		}
	}
	if hasPath {
		if err := validateJSONPath("path", spec.Parameters["path"]); err != nil {
			return err
		}
	}

	if _, ok := spec.Parameters["value"]; !ok {
		return fmt.Errorf("CRDMutation requires 'value' parameter (JSON value to set)")
	}

	// Reject oversized values that could exhaust etcd annotation limits.
	value := spec.Parameters["value"]
	if len(value) > maxParameterValueLength {
		return fmt.Errorf("CRDMutation 'value' exceeds maximum length of %d bytes", maxParameterValueLength)
	}

	// Reject JSON objects and arrays as values unless dangerLevel is high.
	// Gateway API resources (HTTPRoute, Gateway) use arrays for hostnames,
	// rules, and parentRefs, so complex values are needed for those mutations.
	var jsonProbe any
	if json.Unmarshal([]byte(value), &jsonProbe) == nil {
		switch jsonProbe.(type) {
		case map[string]any, []any:
			if spec.DangerLevel != v1alpha1.DangerLevelHigh {
				return fmt.Errorf("CRDMutation 'value' must be a scalar (string, number, boolean), not a JSON object or array; set dangerLevel: high to allow complex values")
			}
		}
	}

	// Reject null value without dangerLevel: high (causes field deletion via merge patch)
	if strings.TrimSpace(value) == "null" && spec.DangerLevel != v1alpha1.DangerLevelHigh {
		return fmt.Errorf("CRDMutation 'value' of 'null' causes field deletion via merge patch; requires dangerLevel: high")
	}

	// Reject sensitive spec fields without dangerLevel: high.
	// For 'field' param, check the field name directly.
	// For 'path' param, check the last segment (the leaf field name).
	targetField := spec.Parameters["field"]
	if hasPath {
		segments := strings.Split(spec.Parameters["path"], ".")
		targetField = segments[len(segments)-1]
	}
	if sensitiveSpecFields[targetField] && spec.DangerLevel != v1alpha1.DangerLevelHigh {
		return fmt.Errorf("CRDMutation targeting sensitive field %q requires dangerLevel: high", targetField)
	}

	// Block core Kubernetes types unless the experiment has dangerLevel: high
	if isCoreKubernetesType(apiVersion) {
		if spec.DangerLevel != v1alpha1.DangerLevelHigh {
			return fmt.Errorf("CRDMutation targeting core Kubernetes type (apiVersion=%s) requires dangerLevel: high", apiVersion)
		}
	}

	return nil
}

// dangerousPaths are JSON paths that must never be mutated by CRDMutation
// because they would corrupt the resource identity or break Kubernetes internals.
var dangerousPaths = map[string]bool{
	"apiVersion":                  true,
	"kind":                        true,
	"metadata.name":               true,
	"metadata.namespace":          true,
	"metadata.uid":                true,
	"metadata.resourceVersion":    true,
	"metadata.generation":         true,
	"metadata.creationTimestamp":  true,
	"metadata.deletionTimestamp":  true,
	"metadata.deletionGracePeriodSeconds": true,
}

// validateJSONPath validates a dot-notation path for CRDMutation.
// Paths must have at least one segment, each segment must be a valid field name,
// and the path must not target dangerous resource identity fields.
func validateJSONPath(paramName, path string) error {
	if len(path) == 0 {
		return fmt.Errorf("%s must not be empty", paramName)
	}
	if len(path) > maxNameLength {
		return fmt.Errorf("%s exceeds maximum length of %d characters", paramName, maxNameLength)
	}
	if strings.HasPrefix(path, ".") || strings.HasSuffix(path, ".") {
		return fmt.Errorf("%s %q must not start or end with a dot", paramName, path)
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("%s %q contains empty segment (consecutive dots)", paramName, path)
	}

	segments := strings.Split(path, ".")
	for _, seg := range segments {
		if !validPathSegmentPattern.MatchString(seg) {
			return fmt.Errorf("%s segment %q is not a valid field name", paramName, seg)
		}
	}

	if dangerousPaths[path] {
		return fmt.Errorf("CRDMutation cannot target %q (resource identity field)", path)
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
// Kubernetes or OpenShift infrastructure type rather than a CRD.
func isCoreKubernetesType(apiVersion string) bool {
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
		strings.HasPrefix(apiVersion, "apiregistration.k8s.io/") ||
		strings.HasPrefix(apiVersion, "route.openshift.io/")
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

	if wt, ok := spec.Parameters["webhookType"]; ok {
		if wt != "validating" && wt != "mutating" {
			return fmt.Errorf("invalid webhookType %q; must be 'validating' or 'mutating'", wt)
		}
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
	if kind == "ChaosExperiment" && (apiVersion == "" || strings.Contains(apiVersion, "chaos.operatorchaos.io")) {
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
	if spec.Parameters["finalizerName"] == "chaos.operatorchaos.io/cleanup" {
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

// ownerRefOrphanForbiddenKinds are resource kinds that should never have
// their ownerReferences removed.
var ownerRefOrphanForbiddenKinds = map[string]bool{
	"Namespace": true,
	"Node":      true,
}

func validateOwnerRefOrphanParams(spec v1alpha1.InjectionSpec) error {
	apiVersion := spec.Parameters["apiVersion"]
	if apiVersion == "" {
		return fmt.Errorf("OwnerRefOrphan requires non-empty 'apiVersion' parameter")
	}
	kind := spec.Parameters["kind"]
	if kind == "" {
		return fmt.Errorf("OwnerRefOrphan requires non-empty 'kind' parameter")
	}
	if ownerRefOrphanForbiddenKinds[kind] {
		return fmt.Errorf("OwnerRefOrphan targeting %q resources is not allowed", kind)
	}
	if kind == "ChaosExperiment" && strings.Contains(apiVersion, "chaos.operatorchaos.io") {
		return fmt.Errorf("OwnerRefOrphan cannot target ChaosExperiment CRs")
	}
	name := spec.Parameters["name"]
	if name == "" {
		return fmt.Errorf("OwnerRefOrphan requires 'name' parameter")
	}
	if err := validateK8sName("name", name); err != nil {
		return err
	}
	return rejectChaosManagedResource("OwnerRefOrphan", name)
}

func validateQuotaExhaustionParams(spec v1alpha1.InjectionSpec) error {
	quotaName := spec.Parameters["quotaName"]
	if quotaName == "" {
		return fmt.Errorf("QuotaExhaustion requires non-empty 'quotaName' parameter")
	}
	if err := validateK8sName("quotaName", quotaName); err != nil {
		return err
	}
	if err := rejectChaosManagedResource("QuotaExhaustion", quotaName); err != nil {
		return err
	}
	// At minimum, one resource limit must be specified
	hasLimit := false
	for _, key := range []string{"cpu", "memory", "pods", "services", "configmaps", "secrets"} {
		if spec.Parameters[key] != "" {
			hasLimit = true
			break
		}
	}
	if !hasLimit {
		return fmt.Errorf("QuotaExhaustion requires at least one resource limit (cpu, memory, pods, services, configmaps, secrets)")
	}
	return nil
}

func validateWebhookLatencyParams(spec v1alpha1.InjectionSpec) error {
	if spec.DangerLevel != v1alpha1.DangerLevelHigh {
		return fmt.Errorf("WebhookLatency requires dangerLevel: high (deploys webhook pods)")
	}
	resources := spec.Parameters["resources"]
	if resources == "" {
		return fmt.Errorf("WebhookLatency requires 'resources' parameter (comma-separated list of resources to intercept, e.g. 'deployments,services')")
	}
	apiGroups := spec.Parameters["apiGroups"]
	if apiGroups == "" {
		return fmt.Errorf("WebhookLatency requires 'apiGroups' parameter (comma-separated API groups, e.g. 'apps' or '*')")
	}
	delay := spec.Parameters["delay"]
	if delay != "" {
		d, err := time.ParseDuration(delay)
		if err != nil {
			return fmt.Errorf("WebhookLatency: invalid delay %q: %w", delay, err)
		}
		if d > 29*time.Second {
			return fmt.Errorf("WebhookLatency: delay %v exceeds 29s (API server timeout is 30s)", d)
		}
		if d < 1*time.Second {
			return fmt.Errorf("WebhookLatency: delay %v is too short to be useful (minimum 1s)", d)
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

// systemLabelPatterns are label key patterns that require dangerLevel: high
// for LabelStomping experiments.
var systemLabelPatterns = []string{
	"kubernetes.io/",
	"k8s.io/",
	"node-role.kubernetes.io/",
}

func validateLabelStompingParams(spec v1alpha1.InjectionSpec) error {
	apiVersion := spec.Parameters["apiVersion"]
	if apiVersion == "" {
		return fmt.Errorf("LabelStomping requires non-empty 'apiVersion' parameter")
	}
	kind := spec.Parameters["kind"]
	if kind == "" {
		return fmt.Errorf("LabelStomping requires non-empty 'kind' parameter")
	}
	name := spec.Parameters["name"]
	if name == "" {
		return fmt.Errorf("LabelStomping requires 'name' parameter")
	}
	if err := validateK8sName("name", name); err != nil {
		return err
	}
	if err := rejectChaosManagedResource("LabelStomping", name); err != nil {
		return err
	}

	labelKey := spec.Parameters["labelKey"]
	if labelKey == "" {
		return fmt.Errorf("LabelStomping requires non-empty 'labelKey' parameter")
	}
	if err := validateLabelKey(labelKey); err != nil {
		return fmt.Errorf("LabelStomping 'labelKey': %w", err)
	}

	// Reject chaos-owned labels
	if labelKey == "app.kubernetes.io/managed-by" {
		return fmt.Errorf("LabelStomping cannot modify chaos-owned label %q", labelKey)
	}
	if strings.HasPrefix(labelKey, "chaos.operatorchaos.io/") {
		return fmt.Errorf("LabelStomping cannot modify chaos-owned label %q (prefix chaos.operatorchaos.io/ is reserved)", labelKey)
	}

	action := spec.Parameters["action"]
	if action == "" {
		return fmt.Errorf("LabelStomping requires 'action' parameter ('overwrite' or 'delete')")
	}
	if action != "overwrite" && action != "delete" {
		return fmt.Errorf("LabelStomping action must be 'overwrite' or 'delete', got %q", action)
	}

	// Validate newValue for overwrite action
	if action == "overwrite" {
		nv := spec.Parameters["newValue"]
		if nv == "" {
			nv = "chaos-stomped"
		}
		if err := validateLabelValue(nv); err != nil {
			return fmt.Errorf("LabelStomping 'newValue': %w", err)
		}
	}

	// System labels require dangerLevel: high
	for _, pattern := range systemLabelPatterns {
		if strings.Contains(labelKey, pattern) {
			if spec.DangerLevel != v1alpha1.DangerLevelHigh {
				return fmt.Errorf("LabelStomping targeting system label %q (matches %q) requires dangerLevel: high", labelKey, pattern)
			}
			break
		}
	}

	return nil
}

// validLabelNamePattern matches the name portion of a Kubernetes label key.
var validLabelNamePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$`)

// validLabelValuePattern matches a valid Kubernetes label value.
var validLabelValuePattern = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?)?$`)

// validateLabelKey checks that a label key conforms to Kubernetes label key rules:
// optional prefix (DNS subdomain, max 253 chars) + "/" + name (max 63 chars).
func validateLabelKey(key string) error {
	if len(key) == 0 {
		return fmt.Errorf("must not be empty")
	}

	var name string
	if idx := strings.LastIndex(key, "/"); idx >= 0 {
		prefix := key[:idx]
		name = key[idx+1:]
		if len(prefix) == 0 {
			return fmt.Errorf("prefix before '/' must not be empty")
		}
		if len(prefix) > 253 {
			return fmt.Errorf("prefix exceeds 253 characters")
		}
		if !validNamePattern.MatchString(prefix) {
			return fmt.Errorf("prefix %q is not a valid DNS subdomain", prefix)
		}
	} else {
		name = key
	}

	if len(name) == 0 {
		return fmt.Errorf("name portion must not be empty")
	}
	if len(name) > 63 {
		return fmt.Errorf("name portion exceeds 63 characters")
	}
	if !validLabelNamePattern.MatchString(name) {
		return fmt.Errorf("name %q is not valid (must match [a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?)", name)
	}
	return nil
}

// validateLabelValue checks that a label value conforms to Kubernetes rules:
// max 63 chars, matching [a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])? or empty.
func validateLabelValue(value string) error {
	if len(value) > 63 {
		return fmt.Errorf("exceeds 63 characters")
	}
	if !validLabelValuePattern.MatchString(value) {
		return fmt.Errorf("%q is not a valid label value", value)
	}
	return nil
}

// forbiddenNamespaces is a deny-list of namespaces that must never be deleted.
var forbiddenNamespaces = map[string]bool{
	"default":         true,
	"kube-system":     true,
	"kube-public":     true,
	"kube-node-lease": true,
}

// forbiddenNamespacePrefixes are namespace name prefixes that must never be deleted.
// Operator-specific namespaces (e.g. vendor prefixes) are not forbidden here
// because they are legitimate chaos targets. Safety is enforced via
// dangerLevel: high + allowDangerous: true.
var forbiddenNamespacePrefixes = []string{
	"openshift-",
	"chaos-",
}

// controllerNamespace is the namespace where the chaos controller runs.
const controllerNamespace = "operator-chaos-system"

func validateNamespaceDeletionParams(spec v1alpha1.InjectionSpec) error {
	if spec.DangerLevel != v1alpha1.DangerLevelHigh {
		return fmt.Errorf("NamespaceDeletion requires dangerLevel: high")
	}

	ns := spec.Parameters["namespace"]
	if ns == "" {
		return fmt.Errorf("NamespaceDeletion requires non-empty 'namespace' parameter")
	}
	if err := validateK8sName("namespace", ns); err != nil {
		return err
	}

	if forbiddenNamespaces[ns] {
		return fmt.Errorf("NamespaceDeletion cannot target protected namespace %q", ns)
	}
	if ns == controllerNamespace {
		return fmt.Errorf("NamespaceDeletion cannot target controller namespace %q", ns)
	}
	for _, prefix := range forbiddenNamespacePrefixes {
		if strings.HasPrefix(ns, prefix) {
			return fmt.Errorf("NamespaceDeletion cannot target namespace %q (prefix %q is protected)", ns, prefix)
		}
	}

	return nil
}
