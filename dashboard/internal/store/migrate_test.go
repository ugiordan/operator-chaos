package store

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrate_AppliesInitialSchema(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	err = Migrate(db)
	require.NoError(t, err)

	// Verify experiments table exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM experiments").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify schema_version was recorded
	var version int
	err = db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version)
	assert.NoError(t, err)
	assert.Equal(t, 1, version)
}

func TestMigrate_IsIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, Migrate(db))
	require.NoError(t, Migrate(db))

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}
