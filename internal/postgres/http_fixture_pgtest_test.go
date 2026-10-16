//go:build pgtest

package postgres_test

import (
	"database/sql"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/postgres"
	"os"
	"testing"
)

const pgE2ESchema = "agentsview_schema_test"

// newPGE2ETestDatabase opens the shared test schema wiped clean, with every
// raw-sync table provisioned, and returns a throwaway data directory.
func newPGE2ETestDatabase(t *testing.T) (*sql.DB, string) {
	t.Helper()
	pgURL := os.Getenv("TEST_PG_URL")
	if pgURL == "" {
		t.Skip("TEST_PG_URL not set; skipping PG tests")
	}
	pgE2ECleanSchema(t, pgURL)
	t.Cleanup(func() { pgE2ECleanSchema(t, pgURL) })
	pg, err := postgres.Open(pgURL, pgE2ESchema, true)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pg.Close()) })
	require.NoError(t, postgres.EnsureSchema(t.Context(), pg, pgE2ESchema))
	return pg, t.TempDir()
}

func pgE2ECleanSchema(t *testing.T, pgURL string) {
	t.Helper()
	pg, err := sql.Open("pgx", pgURL)
	require.NoError(t, err, "connecting to pg")
	defer pg.Close()
	_, err = pg.Exec("DROP SCHEMA IF EXISTS " + pgE2ESchema + " CASCADE")
	require.NoError(t, err, "dropping test schema")
}
