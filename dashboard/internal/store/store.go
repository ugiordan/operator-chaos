package store

import "time"

type Experiment struct {
	ID              string
	Name            string
	Namespace       string
	Operator        string
	Component       string
	InjectionType   string
	Phase           string
	Verdict         string
	DangerLevel     string
	RecoveryMs      *int64
	StartTime       *time.Time
	EndTime         *time.Time
	SuiteName       string
	SuiteRunID      string
	OperatorVersion string
	CleanupError    string
	SpecJSON        string
	StatusJSON      string
	CreatedAt       time.Time
	UpdatedAt       time.Time
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
	Items      []Experiment
	TotalCount int
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
	InjectionType string
	AvgMs         int64
}

type SuiteRun struct {
	SuiteName       string
	SuiteRunID      string
	OperatorVersion string
	Total           int
	Resilient       int
	Degraded        int
	Failed          int
}

type Store interface {
	Upsert(exp Experiment) error
	Get(namespace, name string) (*Experiment, error)
	List(filter ListFilter) (ListResult, error)
	ListRunning() ([]Experiment, error)
	OverviewStats(since *time.Time) (OverviewStats, error)
	AvgRecoveryByType(since *time.Time) ([]RecoveryAvg, error)
	ListOperators(since *time.Time) ([]string, error)
	ListByOperator(operator string, since *time.Time) ([]Experiment, error)
	ListSuiteRuns() ([]SuiteRun, error)
	ListBySuiteRunID(runID string) ([]Experiment, error)
	CompareSuiteRuns(suiteNameA, runIDA, runIDB string) ([]Experiment, []Experiment, error)
	Close() error
}
