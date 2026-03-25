-- Note: schema_version table is created by the migration runner itself (migrate.go),
-- so it is NOT included here. Only application tables go in migration files.

CREATE TABLE IF NOT EXISTS experiments (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    namespace       TEXT NOT NULL,
    operator        TEXT NOT NULL,
    component       TEXT NOT NULL,
    injection_type  TEXT NOT NULL,
    phase           TEXT NOT NULL,
    verdict         TEXT,
    danger_level    TEXT,
    recovery_ms     INTEGER,
    start_time      TEXT,
    end_time        TEXT,
    suite_name      TEXT,
    suite_run_id    TEXT,
    operator_version TEXT,
    cleanup_error   TEXT,
    spec_json       TEXT NOT NULL,
    status_json     TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_experiments_namespace ON experiments(namespace);
CREATE INDEX IF NOT EXISTS idx_experiments_operator ON experiments(operator);
CREATE INDEX IF NOT EXISTS idx_experiments_component ON experiments(component);
CREATE INDEX IF NOT EXISTS idx_experiments_verdict ON experiments(verdict);
CREATE INDEX IF NOT EXISTS idx_experiments_phase ON experiments(phase);
CREATE INDEX IF NOT EXISTS idx_experiments_injection_type ON experiments(injection_type);
CREATE INDEX IF NOT EXISTS idx_experiments_start_time ON experiments(start_time);
CREATE INDEX IF NOT EXISTS idx_experiments_suite_run_id ON experiments(suite_run_id);
CREATE INDEX IF NOT EXISTS idx_experiments_suite_name ON experiments(suite_name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_experiments_natural_key ON experiments(namespace, name, start_time);
