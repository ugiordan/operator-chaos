package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting busy timeout: %w", err)
	}
	if err := Migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrating: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

func (s *SQLiteStore) Upsert(exp Experiment) error {
	var startStr, endStr *string
	if exp.StartTime != nil {
		v := exp.StartTime.Format(time.RFC3339)
		startStr = &v
	}
	if exp.EndTime != nil {
		v := exp.EndTime.Format(time.RFC3339)
		endStr = &v
	}
	_, err := s.db.Exec(`
		INSERT INTO experiments (id, name, namespace, operator, component, injection_type, phase,
			verdict, danger_level, recovery_ms, start_time, end_time, suite_name, suite_run_id,
			operator_version, cleanup_error, spec_json, status_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			phase=excluded.phase, verdict=excluded.verdict, danger_level=excluded.danger_level,
			recovery_ms=excluded.recovery_ms, end_time=excluded.end_time,
			cleanup_error=excluded.cleanup_error, status_json=excluded.status_json,
			updated_at=datetime('now')`,
		exp.ID, exp.Name, exp.Namespace, exp.Operator, exp.Component, exp.InjectionType,
		exp.Phase, exp.Verdict, exp.DangerLevel, exp.RecoveryMs, startStr, endStr,
		exp.SuiteName, exp.SuiteRunID, exp.OperatorVersion, exp.CleanupError,
		exp.SpecJSON, exp.StatusJSON)
	return err
}

func (s *SQLiteStore) Get(namespace, name string) (*Experiment, error) {
	row := s.db.QueryRow(`
		SELECT id, name, namespace, operator, component, injection_type, phase,
			verdict, danger_level, recovery_ms, start_time, end_time, suite_name,
			suite_run_id, operator_version, cleanup_error, spec_json, status_json,
			created_at, updated_at
		FROM experiments WHERE namespace=? AND name=?
		ORDER BY start_time DESC LIMIT 1`, namespace, name)
	return scanExperiment(row)
}

func (s *SQLiteStore) List(f ListFilter) (ListResult, error) {
	var where []string
	var args []interface{}
	if f.Namespace != "" { where = append(where, "namespace=?"); args = append(args, f.Namespace) }
	if f.Operator != "" { where = append(where, "operator=?"); args = append(args, f.Operator) }
	if f.Component != "" { where = append(where, "component=?"); args = append(args, f.Component) }
	if f.Type != "" { where = append(where, "injection_type=?"); args = append(args, f.Type) }
	if f.Verdict != "" { where = append(where, "verdict=?"); args = append(args, f.Verdict) }
	if f.Phase != "" { where = append(where, "phase=?"); args = append(args, f.Phase) }
	if f.Search != "" {
		where = append(where, "name LIKE ?")
		args = append(args, "%"+f.Search+"%")
	}
	if f.Since != nil { where = append(where, "start_time >= ?"); args = append(args, f.Since.Format(time.RFC3339)) }

	whereClause := ""
	if len(where) > 0 { whereClause = "WHERE " + strings.Join(where, " AND ") }

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM experiments "+whereClause, args...).Scan(&total); err != nil {
		return ListResult{}, err
	}

	orderCol := "start_time"
	switch f.Sort {
	case "name": orderCol = "name"
	case "date": orderCol = "start_time"
	case "recovery": orderCol = "recovery_ms"
	}
	orderDir := "DESC"
	if f.Order == "asc" { orderDir = "ASC" }

	page := f.Page; if page < 1 { page = 1 }
	pageSize := f.PageSize; if pageSize < 1 { pageSize = 10 }
	offset := (page - 1) * pageSize

	query := fmt.Sprintf(`SELECT id, name, namespace, operator, component, injection_type, phase,
		verdict, danger_level, recovery_ms, start_time, end_time, suite_name,
		suite_run_id, operator_version, cleanup_error, spec_json, status_json,
		created_at, updated_at FROM experiments %s ORDER BY %s %s LIMIT ? OFFSET ?`, whereClause, orderCol, orderDir)

	listArgs := append(args, pageSize, offset)
	rows, err := s.db.Query(query, listArgs...)
	if err != nil { return ListResult{}, err }
	defer func() { _ = rows.Close() }()

	items := []Experiment{}
	for rows.Next() {
		exp, err := scanExperimentRows(rows)
		if err != nil { return ListResult{}, err }
		items = append(items, *exp)
	}
	if err := rows.Err(); err != nil { return ListResult{}, err }
	return ListResult{Items: items, TotalCount: total}, nil
}

var runningPhases = []string{"Pending", "SteadyStatePre", "Injecting", "Observing", "SteadyStatePost", "Evaluating"}

func (s *SQLiteStore) ListRunning() ([]Experiment, error) {
	placeholders := make([]string, len(runningPhases))
	args := make([]interface{}, len(runningPhases))
	for i, p := range runningPhases { placeholders[i] = "?"; args[i] = p }
	rows, err := s.db.Query(fmt.Sprintf(`SELECT id, name, namespace, operator, component, injection_type, phase,
		verdict, danger_level, recovery_ms, start_time, end_time, suite_name,
		suite_run_id, operator_version, cleanup_error, spec_json, status_json,
		created_at, updated_at FROM experiments WHERE phase IN (%s) ORDER BY start_time DESC`,
		strings.Join(placeholders, ",")), args...)
	if err != nil { return nil, err }
	defer func() { _ = rows.Close() }()
	items := []Experiment{}
	for rows.Next() {
		exp, err := scanExperimentRows(rows)
		if err != nil { return nil, err }
		items = append(items, *exp)
	}
	if err := rows.Err(); err != nil { return nil, err }
	return items, nil
}

func (s *SQLiteStore) OverviewStats(since *time.Time) (OverviewStats, error) {
	whereClause := ""
	var args []interface{}
	if since != nil { whereClause = "WHERE start_time >= ?"; args = append(args, since.Format(time.RFC3339)) }

	var stats OverviewStats
	if err := s.db.QueryRow("SELECT COUNT(*) FROM experiments "+whereClause, args...).Scan(&stats.Total); err != nil {
		return stats, err
	}

	for _, v := range []struct { verdict string; dest *int }{
		{"Resilient", &stats.Resilient}, {"Degraded", &stats.Degraded},
		{"Failed", &stats.Failed}, {"Inconclusive", &stats.Inconclusive},
	} {
		vq := "SELECT COUNT(*) FROM experiments " + whereClause
		va := append([]interface{}{}, args...)
		if whereClause == "" { vq += " WHERE verdict=?" } else { vq += " AND verdict=?" }
		va = append(va, v.verdict)
		if err := s.db.QueryRow(vq, va...).Scan(v.dest); err != nil { return stats, err }
	}

	placeholders := make([]string, len(runningPhases))
	rArgs := append([]interface{}{}, args...)
	for i, p := range runningPhases { placeholders[i] = "?"; rArgs = append(rArgs, p) }
	rq := "SELECT COUNT(*) FROM experiments "
	if whereClause == "" { rq += "WHERE phase IN (" + strings.Join(placeholders, ",") + ")" } else { rq += whereClause + " AND phase IN (" + strings.Join(placeholders, ",") + ")" }
	if err := s.db.QueryRow(rq, rArgs...).Scan(&stats.Running); err != nil { return stats, err }

	return stats, nil
}

func (s *SQLiteStore) AvgRecoveryByType(since *time.Time) ([]RecoveryAvg, error) {
	whereClause := "WHERE recovery_ms IS NOT NULL"
	var args []interface{}
	if since != nil { whereClause += " AND start_time >= ?"; args = append(args, since.Format(time.RFC3339)) }
	rows, err := s.db.Query(fmt.Sprintf("SELECT injection_type, CAST(AVG(recovery_ms) AS INTEGER) FROM experiments %s GROUP BY injection_type ORDER BY injection_type", whereClause), args...)
	if err != nil { return nil, err }
	defer func() { _ = rows.Close() }()
	result := []RecoveryAvg{}
	for rows.Next() {
		var r RecoveryAvg
		if err := rows.Scan(&r.InjectionType, &r.AvgMs); err != nil { return nil, err }
		result = append(result, r)
	}
	if err := rows.Err(); err != nil { return nil, err }
	return result, nil
}

func (s *SQLiteStore) Trends(since *time.Time) (TrendStats, error) {
	// Compare current period vs previous period of equal length.
	// If since is nil, use last 7 days as default.
	now := time.Now()
	if since == nil {
		t := now.Add(-7 * 24 * time.Hour)
		since = &t
	}
	duration := now.Sub(*since)
	prevStart := since.Add(-duration)

	var current, previous TrendStats
	for _, period := range []struct {
		start time.Time
		end   time.Time
		dest  *TrendStats
	}{
		{*since, now, &current},
		{prevStart, *since, &previous},
	} {
		row := s.db.QueryRow(`SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN verdict='Resilient' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN verdict='Degraded' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN verdict='Failed' THEN 1 ELSE 0 END), 0)
			FROM experiments WHERE start_time >= ? AND start_time < ?`,
			period.start.Format(time.RFC3339), period.end.Format(time.RFC3339))
		if err := row.Scan(&period.dest.Total, &period.dest.Resilient, &period.dest.Degraded, &period.dest.Failed); err != nil {
			return TrendStats{}, err
		}
	}

	return TrendStats{
		Total:     current.Total - previous.Total,
		Resilient: current.Resilient - previous.Resilient,
		Degraded:  current.Degraded - previous.Degraded,
		Failed:    current.Failed - previous.Failed,
	}, nil
}

func (s *SQLiteStore) VerdictTimeline(days int) ([]DayVerdicts, error) {
	if days <= 0 {
		days = 30
	}
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	rows, err := s.db.Query(`SELECT
		date(start_time) as d,
		SUM(CASE WHEN verdict='Resilient' THEN 1 ELSE 0 END),
		SUM(CASE WHEN verdict='Degraded' THEN 1 ELSE 0 END),
		SUM(CASE WHEN verdict='Failed' THEN 1 ELSE 0 END)
		FROM experiments WHERE start_time >= ? AND verdict IS NOT NULL
		GROUP BY d ORDER BY d`, since.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []DayVerdicts{}
	for rows.Next() {
		var dv DayVerdicts
		if err := rows.Scan(&dv.Date, &dv.Resilient, &dv.Degraded, &dv.Failed); err != nil {
			return nil, err
		}
		result = append(result, dv)
	}
	if err := rows.Err(); err != nil { return nil, err }
	return result, nil
}

func (s *SQLiteStore) ListOperators(since *time.Time) ([]string, error) {
	whereClause := ""
	var args []interface{}
	if since != nil { whereClause = "WHERE start_time >= ?"; args = append(args, since.Format(time.RFC3339)) }
	rows, err := s.db.Query("SELECT DISTINCT operator FROM experiments "+whereClause+" ORDER BY operator", args...)
	if err != nil { return nil, err }
	defer func() { _ = rows.Close() }()
	ops := []string{}
	for rows.Next() {
		var op string
		if err := rows.Scan(&op); err != nil { return nil, err }
		ops = append(ops, op)
	}
	if err := rows.Err(); err != nil { return nil, err }
	return ops, nil
}

func (s *SQLiteStore) ListByOperator(operator string, since *time.Time) ([]Experiment, error) {
	where := "WHERE operator=?"
	args := []interface{}{operator}
	if since != nil { where += " AND start_time >= ?"; args = append(args, since.Format(time.RFC3339)) }
	rows, err := s.db.Query(fmt.Sprintf(`SELECT id, name, namespace, operator, component, injection_type, phase,
		verdict, danger_level, recovery_ms, start_time, end_time, suite_name,
		suite_run_id, operator_version, cleanup_error, spec_json, status_json,
		created_at, updated_at FROM experiments %s ORDER BY start_time DESC`, where), args...)
	if err != nil { return nil, err }
	defer func() { _ = rows.Close() }()
	items := []Experiment{}
	for rows.Next() {
		exp, err := scanExperimentRows(rows)
		if err != nil { return nil, err }
		items = append(items, *exp)
	}
	if err := rows.Err(); err != nil { return nil, err }
	return items, nil
}

func (s *SQLiteStore) ListBySuiteRunID(runID string) ([]Experiment, error) {
	rows, err := s.db.Query(`SELECT id, name, namespace, operator, component, injection_type, phase,
		verdict, danger_level, recovery_ms, start_time, end_time, suite_name,
		suite_run_id, operator_version, cleanup_error, spec_json, status_json,
		created_at, updated_at FROM experiments WHERE suite_run_id=? ORDER BY name`, runID)
	if err != nil { return nil, err }
	defer func() { _ = rows.Close() }()
	items := []Experiment{}
	for rows.Next() {
		exp, err := scanExperimentRows(rows)
		if err != nil { return nil, err }
		items = append(items, *exp)
	}
	if err := rows.Err(); err != nil { return nil, err }
	return items, nil
}

func (s *SQLiteStore) ListSuiteRuns() ([]SuiteRun, error) {
	rows, err := s.db.Query(`SELECT COALESCE(suite_name, ''), suite_run_id, COALESCE(operator_version, ''),
		COUNT(*) as total, SUM(CASE WHEN verdict='Resilient' THEN 1 ELSE 0 END),
		SUM(CASE WHEN verdict='Degraded' THEN 1 ELSE 0 END),
		SUM(CASE WHEN verdict='Failed' THEN 1 ELSE 0 END)
		FROM experiments WHERE suite_run_id IS NOT NULL AND suite_run_id != ''
		GROUP BY suite_name, suite_run_id, operator_version ORDER BY MAX(start_time) DESC`)
	if err != nil { return nil, err }
	defer func() { _ = rows.Close() }()
	runs := []SuiteRun{}
	for rows.Next() {
		var r SuiteRun
		if err := rows.Scan(&r.SuiteName, &r.SuiteRunID, &r.OperatorVersion, &r.Total, &r.Resilient, &r.Degraded, &r.Failed); err != nil { return nil, err }
		runs = append(runs, r)
	}
	if err := rows.Err(); err != nil { return nil, err }
	return runs, nil
}

func (s *SQLiteStore) CompareSuiteRuns(suiteName, runIDA, runIDB string) ([]Experiment, []Experiment, error) {
	query := `SELECT id, name, namespace, operator, component, injection_type, phase,
		verdict, danger_level, recovery_ms, start_time, end_time, suite_name,
		suite_run_id, operator_version, cleanup_error, spec_json, status_json,
		created_at, updated_at FROM experiments WHERE suite_name=? AND suite_run_id=? ORDER BY name`
	a, err := s.querySuiteExperiments(query, suiteName, runIDA)
	if err != nil { return nil, nil, err }
	b, err := s.querySuiteExperiments(query, suiteName, runIDB)
	if err != nil { return nil, nil, err }
	return a, b, nil
}

func (s *SQLiteStore) querySuiteExperiments(query, suiteName, runID string) ([]Experiment, error) {
	rows, err := s.db.Query(query, suiteName, runID)
	if err != nil { return nil, err }
	defer func() { _ = rows.Close() }()
	items := []Experiment{}
	for rows.Next() {
		exp, err := scanExperimentRows(rows)
		if err != nil { return nil, err }
		items = append(items, *exp)
	}
	if err := rows.Err(); err != nil { return nil, err }
	return items, nil
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanExperimentFromScanner(sc scanner) (*Experiment, error) {
	var exp Experiment
	var startStr, endStr, createdStr, updatedStr sql.NullString
	var recoveryMs sql.NullInt64
	var verdict, dangerLevel, suiteName, suiteRunID, opVersion, cleanupErr sql.NullString

	err := sc.Scan(&exp.ID, &exp.Name, &exp.Namespace, &exp.Operator, &exp.Component,
		&exp.InjectionType, &exp.Phase, &verdict, &dangerLevel, &recoveryMs,
		&startStr, &endStr, &suiteName, &suiteRunID, &opVersion, &cleanupErr,
		&exp.SpecJSON, &exp.StatusJSON, &createdStr, &updatedStr)
	if err != nil {
		if err == sql.ErrNoRows { return nil, nil }
		return nil, err
	}

	exp.Verdict = verdict.String
	exp.DangerLevel = dangerLevel.String
	exp.SuiteName = suiteName.String
	exp.SuiteRunID = suiteRunID.String
	exp.OperatorVersion = opVersion.String
	exp.CleanupError = cleanupErr.String

	if recoveryMs.Valid { exp.RecoveryMs = &recoveryMs.Int64 }
	if startStr.Valid { if t, err := time.Parse(time.RFC3339, startStr.String); err == nil { exp.StartTime = &t } }
	if endStr.Valid { if t, err := time.Parse(time.RFC3339, endStr.String); err == nil { exp.EndTime = &t } }
	if createdStr.Valid { exp.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdStr.String) }
	if updatedStr.Valid { exp.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedStr.String) }

	return &exp, nil
}

func scanExperiment(row *sql.Row) (*Experiment, error) { return scanExperimentFromScanner(row) }
func scanExperimentRows(rows *sql.Rows) (*Experiment, error) { return scanExperimentFromScanner(rows) }
