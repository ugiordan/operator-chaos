package store

import "time"

type Experiment struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Namespace       string     `json:"namespace"`
	Operator        string     `json:"operator"`
	Component       string     `json:"component"`
	InjectionType   string     `json:"type"`
	Phase           string     `json:"phase"`
	Verdict         string     `json:"verdict,omitempty"`
	DangerLevel     string     `json:"dangerLevel,omitempty"`
	RecoveryMs      *int64     `json:"recoveryMs,omitempty"`
	StartTime       *time.Time `json:"startTime,omitempty"`
	EndTime         *time.Time `json:"endTime,omitempty"`
	SuiteName       string     `json:"suiteName,omitempty"`
	SuiteRunID      string     `json:"suiteRunId,omitempty"`
	OperatorVersion string     `json:"operatorVersion,omitempty"`
	CleanupError    string     `json:"cleanupError,omitempty"`
	SpecJSON        string     `json:"specJson"`
	StatusJSON      string     `json:"statusJson"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type ListFilter struct {
	Namespace  string
	Operator   string
	Component  string
	Type       string
	Verdict    string
	Phase      string
	Search     string
	Since      *time.Time
	Sort       string
	Order      string
	Page       int
	PageSize   int
}

type ListResult struct {
	Items      []Experiment `json:"items"`
	TotalCount int          `json:"totalCount"`
}

type OverviewStats struct {
	Total        int
	Resilient    int
	Degraded     int
	Failed       int
	Inconclusive int
	Running      int
}

type RecoveryAvg struct {
	InjectionType string `json:"injectionType"`
	AvgMs         int64  `json:"avgMs"`
}

type SuiteRun struct {
	SuiteName       string `json:"suiteName"`
	SuiteRunID      string `json:"suiteRunId"`
	OperatorVersion string `json:"operatorVersion"`
	Total           int    `json:"total"`
	Resilient       int    `json:"resilient"`
	Degraded        int    `json:"degraded"`
	Failed          int    `json:"failed"`
}

type TrendStats struct {
	Total     int `json:"total"`
	Resilient int `json:"resilient"`
	Degraded  int `json:"degraded"`
	Failed    int `json:"failed"`
}

type DayVerdicts struct {
	Date      string `json:"date"`
	Resilient int    `json:"resilient"`
	Degraded  int    `json:"degraded"`
	Failed    int    `json:"failed"`
}

type Store interface {
	Upsert(exp Experiment) error
	Get(namespace, name string) (*Experiment, error)
	List(filter ListFilter) (ListResult, error)
	ListRunning() ([]Experiment, error)
	OverviewStats(since *time.Time) (OverviewStats, error)
	AvgRecoveryByType(since *time.Time) ([]RecoveryAvg, error)
	Trends(since *time.Time) (TrendStats, error)
	VerdictTimeline(days int) ([]DayVerdicts, error)
	ListOperators(since *time.Time) ([]string, error)
	ListByOperator(operator string, since *time.Time) ([]Experiment, error)
	ListSuiteRuns() ([]SuiteRun, error)
	ListBySuiteRunID(runID string) ([]Experiment, error)
	CompareSuiteRuns(suiteNameA, runIDA, runIDB string) ([]Experiment, []Experiment, error)
	Close() error
}
