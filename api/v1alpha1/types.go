package v1alpha1

import (
	"encoding/json"
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
	Observation ObservationSpec `json:"observation,omitempty" yaml:"observation,omitempty"`
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

type SteadyStateCheck struct {
	Type          string `json:"type" yaml:"type"`
	APIVersion    string `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`
	Kind          string `json:"kind,omitempty" yaml:"kind,omitempty"`
	Name          string `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace     string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	ConditionType string `json:"conditionType,omitempty" yaml:"conditionType,omitempty"`
	Query         string `json:"query,omitempty" yaml:"query,omitempty"`
	Operator      string `json:"operator,omitempty" yaml:"operator,omitempty"`
	Value         string `json:"value,omitempty" yaml:"value,omitempty"`
	For           string `json:"for,omitempty" yaml:"for,omitempty"`
}

type InjectionSpec struct {
	Type        InjectionType     `json:"type" yaml:"type"`
	Parameters  map[string]string `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Duration    Duration          `json:"duration,omitempty" yaml:"duration,omitempty"`
	Count       int               `json:"count,omitempty" yaml:"count,omitempty"`
	TTL         Duration          `json:"ttl,omitempty" yaml:"ttl,omitempty"`
	DangerLevel string            `json:"dangerLevel,omitempty" yaml:"dangerLevel,omitempty"`
}

type InjectionType string

const (
	PodKill            InjectionType = "PodKill"
	PodFailure         InjectionType = "PodFailure"
	NetworkPartition   InjectionType = "NetworkPartition"
	NetworkLatency     InjectionType = "NetworkLatency"
	ResourceExhaustion InjectionType = "ResourceExhaustion"
	CRDMutation        InjectionType = "CRDMutation"
	ConfigDrift        InjectionType = "ConfigDrift"
	WebhookDisrupt     InjectionType = "WebhookDisrupt"
	RBACRevoke         InjectionType = "RBACRevoke"
	FinalizerBlock     InjectionType = "FinalizerBlock"
	OwnerRefOrphan     InjectionType = "OwnerRefOrphan"
	SourceHook         InjectionType = "SourceHook"
)

// Phase 2 injection types (SDK middleware-based, one-line change)
const (
	ClientThrottle     InjectionType = "ClientThrottle"
	APIServerError     InjectionType = "APIServerError"
	WatchDisconnect    InjectionType = "WatchDisconnect"
	LeaderElectionLoss InjectionType = "LeaderElectionLoss"
	WebhookTimeout     InjectionType = "WebhookTimeout"
	WebhookReject      InjectionType = "WebhookReject"
)

type ObservationSpec struct {
	Interval             Duration `json:"interval,omitempty" yaml:"interval,omitempty"`
	Duration             Duration `json:"duration,omitempty" yaml:"duration,omitempty"`
	TrackReconcileCycles bool     `json:"trackReconcileCycles,omitempty" yaml:"trackReconcileCycles,omitempty"`
}

type BlastRadiusSpec struct {
	MaxPodsAffected     int      `json:"maxPodsAffected" yaml:"maxPodsAffected"`
	MaxConcurrentFaults int      `json:"maxConcurrentFaults,omitempty" yaml:"maxConcurrentFaults,omitempty"`
	AllowedNamespaces   []string `json:"allowedNamespaces" yaml:"allowedNamespaces"`
	ForbiddenResources  []string `json:"forbiddenResources,omitempty" yaml:"forbiddenResources,omitempty"`
	RequireLabel        string   `json:"requireLabel,omitempty" yaml:"requireLabel,omitempty"`
	AllowDangerous      bool     `json:"allowDangerous,omitempty" yaml:"allowDangerous,omitempty"`
	DryRun              bool     `json:"dryRun,omitempty" yaml:"dryRun,omitempty"`
}

type HypothesisSpec struct {
	Description      string   `json:"description" yaml:"description"`
	ExpectedBehavior string   `json:"expectedBehavior" yaml:"expectedBehavior"`
	RecoveryTimeout  Duration `json:"recoveryTimeout" yaml:"recoveryTimeout"`
}

// Status types

type ChaosExperimentStatus struct {
	Phase           ExperimentPhase  `json:"phase,omitempty" yaml:"phase,omitempty"`
	Verdict         Verdict          `json:"verdict,omitempty" yaml:"verdict,omitempty"`
	StartTime       *time.Time       `json:"startTime,omitempty" yaml:"startTime,omitempty"`
	EndTime         *time.Time       `json:"endTime,omitempty" yaml:"endTime,omitempty"`
	SteadyStatePre  *CheckResult     `json:"steadyStatePre,omitempty" yaml:"steadyStatePre,omitempty"`
	SteadyStatePost *CheckResult     `json:"steadyStatePost,omitempty" yaml:"steadyStatePost,omitempty"`
	InjectionLog    []InjectionEvent `json:"injectionLog,omitempty" yaml:"injectionLog,omitempty"`
	Observations    []Observation    `json:"observations,omitempty" yaml:"observations,omitempty"`
}

type ExperimentPhase string

const (
	PhasePending         ExperimentPhase = "Pending"
	PhaseSteadyStatePre  ExperimentPhase = "SteadyStatePre"
	PhaseInjecting       ExperimentPhase = "Injecting"
	PhaseObserving       ExperimentPhase = "Observing"
	PhaseSteadyStatePost ExperimentPhase = "SteadyStatePost"
	PhaseEvaluating      ExperimentPhase = "Evaluating"
	PhaseCleanup         ExperimentPhase = "Cleanup"
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

type Observation struct {
	Timestamp time.Time              `json:"timestamp" yaml:"timestamp"`
	Metrics   map[string]interface{} `json:"metrics,omitempty" yaml:"metrics,omitempty"`
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

func (d Duration) MarshalYAML() (interface{}, error) {
	return d.String(), nil
}

func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
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
