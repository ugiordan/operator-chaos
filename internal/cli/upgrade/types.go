// internal/cli/upgrade/types.go
package upgrade

import (
	"encoding/json"
	"fmt"
	"time"

	pkgupgrade "github.com/opendatahub-io/odh-platform-chaos/pkg/upgrade"
)

// PlaybookSpec is the top-level structure for an upgrade or chaos playbook YAML file.
type PlaybookSpec struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   PlaybookMetadata `yaml:"metadata"`
	Upgrade    *UpgradeSpec     `yaml:"upgrade,omitempty"`
	Chaos      *ChaosSpec       `yaml:"chaos,omitempty"`
}

// Steps returns the execution steps for this playbook, regardless of kind.
func (pb *PlaybookSpec) Steps() []PlaybookStep {
	if pb.Upgrade != nil {
		return pb.Upgrade.Steps
	}
	if pb.Chaos != nil {
		return pb.Chaos.Steps
	}
	return nil
}

// ChaosSpec defines a chaos playbook's configuration.
type ChaosSpec struct {
	KnowledgeDir string         `yaml:"knowledgeDir"`
	Steps        []PlaybookStep `yaml:"steps"`
}

// PlaybookMetadata contains playbook identification.
type PlaybookMetadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// UpgradeSpec defines the source/target versions, upgrade path, and execution steps.
type UpgradeSpec struct {
	Source   VersionRef                `yaml:"source"`
	Target   VersionRef               `yaml:"target"`
	Path     *UpgradePath             `yaml:"path,omitempty"`
	Paths    []UpgradePath            `yaml:"paths,omitempty"`
	Rollback *pkgupgrade.RollbackConfig `yaml:"rollback,omitempty"`
	Steps    []PlaybookStep           `yaml:"steps"`
}

// VersionRef points to a versioned knowledge directory.
type VersionRef struct {
	KnowledgeDir string `yaml:"knowledgeDir"`
	Version      string `yaml:"version"`
}

// UpgradePath defines the OLM operator and channel hops for the upgrade.
type UpgradePath struct {
	Name      string   `yaml:"name,omitempty"`
	Operator  string   `yaml:"operator"`
	Namespace string   `yaml:"namespace"`
	Hops      []Hop    `yaml:"hops"`
	DependsOn []string `yaml:"dependsOn,omitempty"`
}

// Hop is a single OLM channel transition.
type Hop struct {
	Channel string   `yaml:"channel"`
	MaxWait Duration `yaml:"maxWait"`
}

// Duration is a wrapper around time.Duration that can be unmarshaled from YAML strings.
type Duration struct {
	time.Duration
}

// UnmarshalYAML implements yaml.Unmarshaler for Duration.
func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	if s == "" {
		d.Duration = 0
		return nil
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = dur
	return nil
}

// MarshalYAML implements yaml.Marshaler for Duration.
func (d Duration) MarshalYAML() (interface{}, error) {
	if d.Duration == 0 {
		return "", nil
	}
	return d.String(), nil
}

// UnmarshalJSON implements json.Unmarshaler for Duration.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s == "" {
		d.Duration = 0
		return nil
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = dur
	return nil
}

// MarshalJSON implements json.Marshaler for Duration.
func (d Duration) MarshalJSON() ([]byte, error) {
	if d.Duration == 0 {
		return []byte(`""`), nil
	}
	return json.Marshal(d.String())
}

// PlaybookStep is a single step in the upgrade playbook.
type PlaybookStep struct {
	Name         string          `yaml:"name"`
	Type         string          `yaml:"type"` // validate-version, kubectl, manual, olm, chaos
	KnowledgeDir string          `yaml:"knowledgeDir,omitempty"`
	Commands     []string        `yaml:"commands,omitempty"`     // kubectl type
	Verify       *VerifyCondition `yaml:"verify,omitempty"`      // kubectl type
	Description  string          `yaml:"description,omitempty"` // manual type
	AutoCheck    string          `yaml:"autoCheck,omitempty"`   // manual type
	Experiments  []string        `yaml:"experiments,omitempty"` // chaos type
	Knowledge    string          `yaml:"knowledge,omitempty"`   // chaos type
	DependsOn    []string        `yaml:"dependsOn,omitempty"`
	PathRef      string          `yaml:"pathRef,omitempty"`
	Synthetic    bool            `yaml:"-" json:"-"`
}

// VerifyCondition is a post-step verification for kubectl steps.
type VerifyCondition struct {
	Type          string `yaml:"type"` // resourceExists
	APIVersion    string `yaml:"apiVersion"`
	Kind          string `yaml:"kind"`
	Namespace     string `yaml:"namespace"`
	LabelSelector string `yaml:"labelSelector"`
}

// PlaybookState tracks execution progress for halt/resume.
type PlaybookState struct {
	PlaybookName   string                `json:"playbookName"`
	StartedAt      time.Time             `json:"startedAt"`
	CompletedSteps map[string]StepResult `json:"completedSteps"`
	FailedStep     string                `json:"failedStep,omitempty"`
	FailedAt       time.Time             `json:"failedAt,omitempty"`
	FailedError    string                `json:"failedError,omitempty"`
}

// StepResult records the outcome of a single step execution.
type StepResult struct {
	Status            string    `json:"status"` // "completed", "skipped"
	FinishedAt        time.Time `json:"finishedAt"`
	Output            string    `json:"output,omitempty"`
	CompletedHops     []string  `json:"completedHops,omitempty"` // OLM steps: per-hop tracking
	RollbackAttempted bool      `json:"rollbackAttempted,omitempty"`
	RollbackSucceeded bool      `json:"rollbackSucceeded,omitempty"`
	RollbackError     string    `json:"rollbackError,omitempty"`
}

// DefaultMaxWait is the default timeout for an OLM channel hop.
const DefaultMaxWait = 20 * time.Minute
