// pkg/olm/types.go
package olm

// ChannelInfo describes an available OLM channel from a PackageManifest.
type ChannelInfo struct {
	Name        string `json:"name"`        // channel name, e.g. "stable-3.3"
	HeadVersion string `json:"headVersion"` // version at the head of this channel
	CSVName     string `json:"csvName"`     // CSV name for the head version
}

// UpgradeStatus tracks the current state of an OLM upgrade hop.
type UpgradeStatus struct {
	Phase               UpgradePhase `json:"phase"`
	Subscription        string       `json:"subscription"`
	Channel             string       `json:"channel"`
	InstallPlan         string       `json:"installPlan,omitempty"`
	InstallPlanApproval string       `json:"installPlanApproval,omitempty"` // "Automatic" or "Manual"
	CurrentCSV          string       `json:"currentCSV,omitempty"`
	InstalledCSV        string       `json:"installedCSV,omitempty"`
	Message             string       `json:"message,omitempty"`
}

// UpgradePhase represents a phase in the OLM upgrade chain.
type UpgradePhase string

const (
	PhasePending             UpgradePhase = "Pending"
	PhaseInstallPlanCreated  UpgradePhase = "InstallPlanCreated"
	PhaseInstallPlanApproved UpgradePhase = "InstallPlanApproved"
	PhaseInstalling          UpgradePhase = "Installing"
	PhaseSucceeded           UpgradePhase = "Succeeded"
	PhaseFailed              UpgradePhase = "Failed"
)
