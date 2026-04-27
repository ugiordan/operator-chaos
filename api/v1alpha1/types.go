package v1alpha1

import (
	"fmt"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DefaultRecoveryTimeout is the default recovery timeout applied when none is specified.
	DefaultRecoveryTimeout = 60 * time.Second
	// MaxRecoveryTimeout is the maximum allowed recovery timeout (1 hour).
	MaxRecoveryTimeout = 1 * time.Hour
	// MaxInjectionTTL is the maximum allowed injection TTL (1 hour).
	MaxInjectionTTL = 1 * time.Hour
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=chaos;ce
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Verdict",type=string,JSONPath=`.status.verdict`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.injection.type`
// +kubebuilder:printcolumn:name="Tier",type=integer,JSONPath=`.spec.tier`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.target.component`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ChaosExperiment defines a chaos engineering experiment.
type ChaosExperiment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChaosExperimentSpec   `json:"spec"`
	Status ChaosExperimentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ChaosExperimentList contains a list of ChaosExperiment.
type ChaosExperimentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChaosExperiment `json:"items"`
}

type ChaosExperimentSpec struct {
	Target      TargetSpec      `json:"target"`
	SteadyState SteadyStateSpec  `json:"steadyState,omitempty"`
	Injection   InjectionSpec   `json:"injection"`
	BlastRadius BlastRadiusSpec `json:"blastRadius"`
	Hypothesis  HypothesisSpec  `json:"hypothesis"`
	// Tier indicates the fidelity tier of this experiment (1-6).
	// Tier 1: basic recovery (PodKill), safe for CI/kind.
	// Tier 2: config/network faults (ConfigDrift, NetworkPartition).
	// Tier 3: resource mutation (CRDMutation, FinalizerBlock, OwnerRefOrphan, LabelStomping, ClientFault).
	// Tier 4: cluster-wide impact (WebhookDisrupt, RBACRevoke, WebhookLatency).
	// Tier 5: destructive (NamespaceDeletion, QuotaExhaustion).
	// Tier 6: multi-fault and upgrade scenarios.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=6
	// +kubebuilder:default=1
	Tier int32 `json:"tier,omitempty"`
}

const (
	MinTier = 1
	MaxTier = 6
)

type TargetSpec struct {
	// +kubebuilder:validation:MinLength=1
	Operator  string `json:"operator"`
	// +kubebuilder:validation:MinLength=1
	Component string `json:"component"`
	Resource  string `json:"resource,omitempty"`
}

type SteadyStateSpec struct {
	// +listType=atomic
	Checks  []SteadyStateCheck `json:"checks,omitempty"`
	Timeout metav1.Duration    `json:"timeout,omitempty"`
}

// CheckType represents the type of a steady-state check.
// +kubebuilder:validation:Enum=conditionTrue;resourceExists
type CheckType string

const (
	CheckConditionTrue  CheckType = "conditionTrue"
	CheckResourceExists CheckType = "resourceExists"
)

// SteadyStateCheck defines a single check for steady-state verification.
// Note: APIVersion refers to the target resource's API version (e.g. "apps/v1"),
// not the CRD's own TypeMeta API version.
type SteadyStateCheck struct {
	Type          CheckType `json:"type"`
	APIVersion    string    `json:"apiVersion,omitempty"`
	Kind          string    `json:"kind,omitempty"`
	Name          string    `json:"name,omitempty"`
	Namespace     string    `json:"namespace,omitempty"`
	ConditionType string    `json:"conditionType,omitempty"`
}

type InjectionSpec struct {
	Type        InjectionType     `json:"type"`
	Parameters  map[string]string `json:"parameters,omitempty"`
	// Count is the number of targets to affect. Defaults to 1.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=1
	Count       int32             `json:"count,omitempty"`
	TTL         metav1.Duration   `json:"ttl,omitempty"`
	DangerLevel DangerLevel       `json:"dangerLevel,omitempty"`
}

// InjectionType represents the type of fault injection.
// +kubebuilder:validation:Enum=PodKill;NetworkPartition;CRDMutation;ConfigDrift;WebhookDisrupt;RBACRevoke;FinalizerBlock;ClientFault;OwnerRefOrphan;QuotaExhaustion;WebhookLatency;NamespaceDeletion;LabelStomping
type InjectionType string

const (
	PodKill          InjectionType = "PodKill"
	NetworkPartition InjectionType = "NetworkPartition"
	CRDMutation      InjectionType = "CRDMutation"
	ConfigDrift      InjectionType = "ConfigDrift"
	WebhookDisrupt   InjectionType = "WebhookDisrupt"
	RBACRevoke       InjectionType = "RBACRevoke"
	FinalizerBlock   InjectionType = "FinalizerBlock"
	ClientFault      InjectionType = "ClientFault"
	OwnerRefOrphan   InjectionType = "OwnerRefOrphan"
	QuotaExhaustion  InjectionType = "QuotaExhaustion"
	WebhookLatency   InjectionType = "WebhookLatency"
	NamespaceDeletion InjectionType = "NamespaceDeletion"
	LabelStomping     InjectionType = "LabelStomping"
)

var validInjectionTypes = map[InjectionType]bool{
	PodKill:          true,
	NetworkPartition: true,
	CRDMutation:      true,
	ConfigDrift:      true,
	WebhookDisrupt:   true,
	RBACRevoke:       true,
	FinalizerBlock:   true,
	ClientFault:      true,
	OwnerRefOrphan:   true,
	QuotaExhaustion:  true,
	WebhookLatency:   true,
	NamespaceDeletion: true,
	LabelStomping:     true,
}

// ValidInjectionTypes returns all valid injection types in sorted order.
func ValidInjectionTypes() []InjectionType {
	types := make([]InjectionType, 0, len(validInjectionTypes))
	for t := range validInjectionTypes {
		types = append(types, t)
	}
	sort.Slice(types, func(i, j int) bool {
		return string(types[i]) < string(types[j])
	})
	return types
}

// ValidateInjectionType returns an error if the given InjectionType is not one of the known types.
func ValidateInjectionType(t InjectionType) error {
	if !validInjectionTypes[t] {
		return fmt.Errorf("unknown injection type %q; valid types: %v", t, ValidInjectionTypes())
	}
	return nil
}

// DangerLevel represents the risk level of an injection.
// +kubebuilder:validation:Enum="";low;medium;high
type DangerLevel string

const (
	DangerLevelLow    DangerLevel = "low"
	DangerLevelMedium DangerLevel = "medium"
	DangerLevelHigh   DangerLevel = "high"
)

var validDangerLevels = map[DangerLevel]bool{
	DangerLevelLow:    true,
	DangerLevelMedium: true,
	DangerLevelHigh:   true,
}

// ValidDangerLevels returns all valid danger levels in sorted order.
func ValidDangerLevels() []DangerLevel {
	levels := make([]DangerLevel, 0, len(validDangerLevels))
	for l := range validDangerLevels {
		levels = append(levels, l)
	}
	sort.Slice(levels, func(i, j int) bool {
		return string(levels[i]) < string(levels[j])
	})
	return levels
}

// ValidateDangerLevel returns an error if the given DangerLevel is not one of the known levels.
// An empty DangerLevel is considered valid (it means "unset").
func ValidateDangerLevel(d DangerLevel) error {
	if d == "" {
		return nil
	}
	if !validDangerLevels[d] {
		return fmt.Errorf("unknown danger level %q; valid levels: %v", d, ValidDangerLevels())
	}
	return nil
}

// ResolvedRecoveryTimeout returns the recovery timeout for the experiment,
// falling back to DefaultRecoveryTimeout when unset.
func (e *ChaosExperiment) ResolvedRecoveryTimeout() time.Duration {
	if e.Spec.Hypothesis.RecoveryTimeout.Duration > 0 {
		return e.Spec.Hypothesis.RecoveryTimeout.Duration
	}
	return DefaultRecoveryTimeout
}

// DefaultNamespace is the default Kubernetes namespace used by
// the chaos framework when no namespace is explicitly specified.
const DefaultNamespace = "default"

type BlastRadiusSpec struct {
	// +kubebuilder:validation:Minimum=1
	MaxPodsAffected int32 `json:"maxPodsAffected"`
	// +listType=set
	AllowedNamespaces []string `json:"allowedNamespaces,omitempty"`
	// +listType=set
	ForbiddenResources []string `json:"forbiddenResources,omitempty"`
	AllowDangerous     bool     `json:"allowDangerous,omitempty"`
	DryRun             bool     `json:"dryRun,omitempty"`
}

type HypothesisSpec struct {
	// +kubebuilder:validation:MinLength=1
	Description     string          `json:"description"`
	// +optional
	RecoveryTimeout metav1.Duration `json:"recoveryTimeout,omitempty"`
}

// Status types

// ExperimentPhase represents the current phase of the experiment.
// +kubebuilder:validation:Enum="";Pending;SteadyStatePre;Injecting;Observing;SteadyStatePost;Evaluating;Complete;Aborted
type ExperimentPhase string

const (
	PhasePending         ExperimentPhase = "Pending"
	PhaseSteadyStatePre  ExperimentPhase = "SteadyStatePre"
	PhaseInjecting       ExperimentPhase = "Injecting"
	PhaseObserving       ExperimentPhase = "Observing"
	PhaseSteadyStatePost ExperimentPhase = "SteadyStatePost"
	PhaseEvaluating      ExperimentPhase = "Evaluating"
	PhaseComplete        ExperimentPhase = "Complete"
	PhaseAborted         ExperimentPhase = "Aborted"
)

// Verdict represents the outcome of a chaos experiment.
// +kubebuilder:validation:Enum="";Resilient;Degraded;Failed;Inconclusive
type Verdict string

const (
	Resilient    Verdict = "Resilient"
	Degraded     Verdict = "Degraded"
	Failed       Verdict = "Failed"
	Inconclusive Verdict = "Inconclusive"
)

// Condition type constants for ChaosExperiment status conditions.
const (
	ConditionSteadyStateEstablished = "SteadyStateEstablished"
	ConditionFaultInjected          = "FaultInjected"
	ConditionRecoveryObserved       = "RecoveryObserved"
	ConditionComplete               = "Complete"
)

type ChaosExperimentStatus struct {
	Phase              ExperimentPhase    `json:"phase,omitempty"`
	Verdict            Verdict            `json:"verdict,omitempty"`
	// +kubebuilder:validation:Minimum=0
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	// Message provides a human-readable description of the current status.
	// +optional
	Message            string             `json:"message,omitempty"`
	StartTime          *metav1.Time       `json:"startTime,omitempty"`
	EndTime            *metav1.Time       `json:"endTime,omitempty"`
	InjectionStartedAt *metav1.Time       `json:"injectionStartedAt,omitempty"`
	SteadyStatePre     *CheckResult       `json:"steadyStatePre,omitempty"`
	SteadyStatePost    *CheckResult       `json:"steadyStatePost,omitempty"`
	// +listType=atomic
	InjectionLog       []InjectionEvent   `json:"injectionLog,omitempty"`
	EvaluationResult   *EvaluationSummary `json:"evaluationResult,omitempty"`
	CleanupError       string             `json:"cleanupError,omitempty"`
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// EvaluationSummary is the CRD-embeddable evaluation result.
type EvaluationSummary struct {
	Verdict         Verdict `json:"verdict"`
	Confidence      string  `json:"confidence,omitempty"`
	RecoveryTime    string  `json:"recoveryTime,omitempty"`
	ReconcileCycles int     `json:"reconcileCycles,omitempty"`
	// +listType=atomic
	Deviations []string `json:"deviations,omitempty"`
}

type CheckResult struct {
	Passed       bool          `json:"passed"`
	ChecksRun    int32         `json:"checksRun"`
	ChecksPassed int32         `json:"checksPassed"`
	// +listType=atomic
	Details      []CheckDetail `json:"details,omitempty"`
	Timestamp    metav1.Time   `json:"timestamp"`
}

type CheckDetail struct {
	Check  SteadyStateCheck `json:"check"`
	Passed bool             `json:"passed"`
	Value  string           `json:"value,omitempty"`
	Error  string           `json:"error,omitempty"`
}

type InjectionEvent struct {
	Timestamp metav1.Time       `json:"timestamp"`
	Type      InjectionType     `json:"type"`
	Target    string            `json:"target"`
	Action    string            `json:"action"`
	Details   map[string]string `json:"details,omitempty"`
}
