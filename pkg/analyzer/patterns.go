package analyzer

type PatternType string

const (
	PatternIgnoredError    PatternType = "IgnoredError"
	PatternGoroutineLaunch PatternType = "GoroutineLaunch"
	PatternNetworkCall     PatternType = "NetworkCall"
	PatternDatabaseCall    PatternType = "DatabaseCall"
	PatternK8sAPICall      PatternType = "K8sAPICall"
)

type Finding struct {
	Type     PatternType `json:"type"`
	File     string      `json:"file"`
	Line     int         `json:"line"`
	Detail   string      `json:"detail"`
	Severity string      `json:"severity"` // info, warning, critical
}
