package diff

// ComponentChange classifies how a component changed between versions.
type ComponentChange string

const (
	ComponentAdded    ComponentChange = "Added"
	ComponentRemoved  ComponentChange = "Removed"
	ComponentModified ComponentChange = "Modified"
	ComponentRenamed  ComponentChange = "Renamed"
)

// ResourceChange classifies how a managed resource changed between versions.
type ResourceChange string

const (
	ResourceAdded    ResourceChange = "Added"
	ResourceRemoved  ResourceChange = "Removed"
	ResourceModified ResourceChange = "Modified"
	ResourceRenamed  ResourceChange = "Renamed"
	ResourceMoved    ResourceChange = "Moved"
)

// DiffChange is a generic add/remove/modify classifier.
type DiffChange string

const (
	DiffAdded    DiffChange = "Added"
	DiffRemoved  DiffChange = "Removed"
	DiffModified DiffChange = "Modified"
)

// SchemaChangeType classifies CRD schema changes.
type SchemaChangeType string

const (
	FieldRemoved      SchemaChangeType = "FieldRemoved"
	FieldAdded        SchemaChangeType = "FieldAdded"
	FieldRenamed      SchemaChangeType = "FieldRenamed"
	TypeChanged       SchemaChangeType = "TypeChanged"
	EnumValueRemoved  SchemaChangeType = "EnumValueRemoved"
	EnumValueAdded    SchemaChangeType = "EnumValueAdded"
	RequiredAdded     SchemaChangeType = "RequiredAdded"
	RequiredRemoved   SchemaChangeType = "RequiredRemoved"
	DefaultChanged    SchemaChangeType = "DefaultChanged"
	APIVersionRemoved SchemaChangeType = "APIVersionRemoved"
)

// Severity classifies how impactful a change is.
type Severity string

const (
	SeverityBreaking Severity = "Breaking"
	SeverityWarning  Severity = "Warning"
	SeverityInfo     Severity = "Info"
)

// UpgradeDiff is the top-level result of comparing two versioned knowledge sets.
type UpgradeDiff struct {
	SourceVersion string          `json:"sourceVersion"`
	TargetVersion string          `json:"targetVersion"`
	Platform      string          `json:"platform"`
	Components    []ComponentDiff `json:"components"`
	Summary       DiffSummary     `json:"summary"`
}

// ComponentDiff describes what changed for a single operator component.
type ComponentDiff struct {
	Operator        string           `json:"operator"`
	Component       string           `json:"component"`
	ChangeType      ComponentChange  `json:"changeType"`
	RenamedFrom     string           `json:"renamedFrom,omitempty"`
	NamespaceChange *NamespaceChange `json:"namespaceChange,omitempty"`
	ResourceDiffs   []ResourceDiff   `json:"resourceDiffs,omitempty"`
	WebhookDiffs    []WebhookDiff    `json:"webhookDiffs,omitempty"`
	FinalizerDiffs  []FinalizerDiff  `json:"finalizerDiffs,omitempty"`
	DependencyDiffs []DependencyDiff `json:"dependencyDiffs,omitempty"`
}

// IsBreaking returns true if any aspect of this component diff is a breaking change.
func (cd *ComponentDiff) IsBreaking() bool {
	if cd.ChangeType == ComponentRenamed || cd.NamespaceChange != nil {
		return true
	}
	for _, r := range cd.ResourceDiffs {
		if r.ChangeType == ResourceRenamed || r.ChangeType == ResourceMoved {
			return true
		}
	}
	for _, w := range cd.WebhookDiffs {
		if w.ChangeType == DiffRemoved || (w.OldType != "" && w.NewType != "" && w.OldType != w.NewType) {
			return true
		}
	}
	for _, f := range cd.FinalizerDiffs {
		if f.ChangeType == DiffRemoved {
			return true
		}
	}
	for _, d := range cd.DependencyDiffs {
		if d.ChangeType == DiffAdded || d.ChangeType == DiffRemoved {
			return true
		}
	}
	return false
}

// NamespaceChange records a namespace move.
type NamespaceChange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ResourceDiff describes what changed for a single managed resource.
type ResourceDiff struct {
	Kind         string         `json:"kind"`
	Name         string         `json:"name"`
	ChangeType   ResourceChange `json:"changeType"`
	RenamedFrom  string         `json:"renamedFrom,omitempty"`
	MovedFrom    string         `json:"movedFrom,omitempty"`
	FieldChanges []FieldChange  `json:"fieldChanges,omitempty"`
}

// FieldChange records a changed field value.
type FieldChange struct {
	Path     string `json:"path"`
	OldValue string `json:"oldValue"`
	NewValue string `json:"newValue"`
}

// WebhookDiff describes webhook changes.
type WebhookDiff struct {
	Name       string     `json:"name"`
	ChangeType DiffChange `json:"changeType"`
	OldType    string     `json:"oldType,omitempty"`
	NewType    string     `json:"newType,omitempty"`
	OldPath    string     `json:"oldPath,omitempty"`
	NewPath    string     `json:"newPath,omitempty"`
}

// FinalizerDiff describes finalizer changes.
type FinalizerDiff struct {
	Finalizer  string     `json:"finalizer"`
	ChangeType DiffChange `json:"changeType"`
}

// DependencyDiff describes dependency graph changes.
type DependencyDiff struct {
	Dependency string     `json:"dependency"`
	ChangeType DiffChange `json:"changeType"`
}

// DiffSummary provides a quick overview of all changes.
type DiffSummary struct {
	ComponentsAdded   int `json:"componentsAdded"`
	ComponentsRemoved int `json:"componentsRemoved"`
	ComponentsRenamed int `json:"componentsRenamed"`
	NamespaceMoves    int `json:"namespaceMoves"`
	ResourceChanges   int `json:"resourceChanges"`
	WebhookChanges    int `json:"webhookChanges"`
	FinalizerChanges  int `json:"finalizerChanges"`
	DependencyChanges int `json:"dependencyChanges"`
	BreakingChanges   int `json:"breakingChanges"`
}

// CRD schema diff types

// CRDDiffReport is the top-level result of comparing CRD schemas.
type CRDDiffReport struct {
	CRDs []CRDDiff `json:"crds"`
}

// CRDDiff describes what changed for a single CRD.
type CRDDiff struct {
	CRDName     string           `json:"crdName"`
	ChangeType  DiffChange       `json:"changeType"`
	APIVersions []APIVersionDiff `json:"apiVersions,omitempty"`
}

// APIVersionDiff describes changes within a single API version of a CRD.
type APIVersionDiff struct {
	Version       string         `json:"version"`
	ChangeType    DiffChange     `json:"changeType"`
	SchemaChanges []SchemaChange `json:"schemaChanges,omitempty"`
	StorageChange *bool          `json:"storageChange,omitempty"`
	ServedChange  *bool          `json:"servedChange,omitempty"`
}

// SchemaChange describes a single change in a CRD's OpenAPI v3 schema.
type SchemaChange struct {
	Path     string           `json:"path"`
	Type     SchemaChangeType `json:"type"`
	Severity Severity         `json:"severity"`
	Detail   string           `json:"detail"`
}
