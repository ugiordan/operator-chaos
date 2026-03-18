package v1alpha1

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// ChaosExperiment defines a chaos engineering experiment.
// Designed as CRD-ready: kubebuilder markers will be added
// when controller mode is implemented.
type ChaosExperiment struct {
	APIVersion string                `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`
	Kind       string                `json:"kind,omitempty" yaml:"kind,omitempty"`
	Metadata   Metadata              `json:"metadata" yaml:"metadata"`
	Spec       ChaosExperimentSpec   `json:"spec" yaml:"spec"`
	Status     ChaosExperimentStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type Metadata struct {
	Name      string            `json:"name" yaml:"name"`
	Namespace string            `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Labels    map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

type ChaosExperimentSpec struct {
	Target      TargetSpec      `json:"target" yaml:"target"`
	SteadyState SteadyStateDef  `json:"steadyState,omitempty" yaml:"steadyState,omitempty"`
	Injection   InjectionSpec   `json:"injection" yaml:"injection"`
	BlastRadius BlastRadiusSpec `json:"blastRadius" yaml:"blastRadius"`
	Hypothesis  HypothesisSpec  `json:"hypothesis" yaml:"hypothesis"`
}

type TargetSpec struct {
	Operator  string `json:"operator" yaml:"operator"`
	Component string `json:"component" yaml:"component"`
	Resource  string `json:"resource,omitempty" yaml:"resource,omitempty"`
}

type SteadyStateDef struct {
	Checks  []SteadyStateCheck `json:"checks,omitempty" yaml:"checks,omitempty"`
	Timeout Duration           `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// CheckType represents the type of a steady-state check.
type CheckType string

const (
	CheckConditionTrue  CheckType = "conditionTrue"
	CheckResourceExists CheckType = "resourceExists"
)

type SteadyStateCheck struct {
	Type          CheckType `json:"type" yaml:"type"`
	APIVersion    string `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`
	Kind          string `json:"kind,omitempty" yaml:"kind,omitempty"`
	Name          string `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace     string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	ConditionType string `json:"conditionType,omitempty" yaml:"conditionType,omitempty"`
}

type InjectionSpec struct {
	Type        InjectionType     `json:"type" yaml:"type"`
	Parameters  map[string]string `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Count       int               `json:"count,omitempty" yaml:"count,omitempty"`
	TTL         Duration          `json:"ttl,omitempty" yaml:"ttl,omitempty"`
	DangerLevel DangerLevel       `json:"dangerLevel,omitempty" yaml:"dangerLevel,omitempty"`
}

// DangerLevel represents the risk level of an injection.
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

// DefaultNamespace is the default Kubernetes namespace used by
// the chaos framework when no namespace is explicitly specified.
const DefaultNamespace = "opendatahub"

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
}

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

func ValidateInjectionType(t InjectionType) error {
	if !validInjectionTypes[t] {
		return fmt.Errorf("unknown injection type %q; valid types: %v", t, ValidInjectionTypes())
	}
	return nil
}

type BlastRadiusSpec struct {
	MaxPodsAffected    int      `json:"maxPodsAffected" yaml:"maxPodsAffected"`
	AllowedNamespaces  []string `json:"allowedNamespaces" yaml:"allowedNamespaces"`
	ForbiddenResources []string `json:"forbiddenResources,omitempty" yaml:"forbiddenResources,omitempty"`
	AllowDangerous      bool     `json:"allowDangerous,omitempty" yaml:"allowDangerous,omitempty"`
	DryRun              bool     `json:"dryRun,omitempty" yaml:"dryRun,omitempty"`
}

type HypothesisSpec struct {
	Description     string   `json:"description" yaml:"description"`
	RecoveryTimeout Duration `json:"recoveryTimeout" yaml:"recoveryTimeout"`
}

// Status types

type ChaosExperimentStatus struct {
	Phase           ExperimentPhase  `json:"phase,omitempty" yaml:"phase,omitempty"`
	Verdict         Verdict          `json:"verdict,omitempty" yaml:"verdict,omitempty"`
	StartTime       *time.Time       `json:"startTime,omitempty" yaml:"startTime,omitempty"`
	EndTime         *time.Time       `json:"endTime,omitempty" yaml:"endTime,omitempty"`
	SteadyStatePre  *CheckResult     `json:"steadyStatePre,omitempty" yaml:"steadyStatePre,omitempty"`
	SteadyStatePost *CheckResult     `json:"steadyStatePost,omitempty" yaml:"steadyStatePost,omitempty"`
	InjectionLog []InjectionEvent `json:"injectionLog,omitempty" yaml:"injectionLog,omitempty"`
}

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

type Verdict string

const (
	Resilient    Verdict = "Resilient"
	Degraded     Verdict = "Degraded"
	Failed       Verdict = "Failed"
	Inconclusive Verdict = "Inconclusive"
)

type CheckResult struct {
	Passed       bool          `json:"passed" yaml:"passed"`
	ChecksRun    int           `json:"checksRun" yaml:"checksRun"`
	ChecksPassed int           `json:"checksPassed" yaml:"checksPassed"`
	Details      []CheckDetail `json:"details,omitempty" yaml:"details,omitempty"`
	Timestamp    time.Time     `json:"timestamp" yaml:"timestamp"`
}

type CheckDetail struct {
	Check  SteadyStateCheck `json:"check" yaml:"check"`
	Passed bool             `json:"passed" yaml:"passed"`
	Value  string           `json:"value,omitempty" yaml:"value,omitempty"`
	Error  string           `json:"error,omitempty" yaml:"error,omitempty"`
}

type InjectionEvent struct {
	Timestamp time.Time         `json:"timestamp" yaml:"timestamp"`
	Type      InjectionType     `json:"type" yaml:"type"`
	Target    string            `json:"target" yaml:"target"`
	Action    string            `json:"action" yaml:"action"`
	Details   map[string]string `json:"details,omitempty" yaml:"details,omitempty"`
}

// Duration wraps time.Duration for YAML/JSON serialization
type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = dur
	return nil
}

func (d Duration) MarshalYAML() (any, error) {
	return d.String(), nil
}

func (d *Duration) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = dur
	return nil
}
