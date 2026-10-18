//go:build !(windows && arm64)

package duckdb

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalMirrorRejectsExternalReads(t *testing.T) {
	dir := t.TempDir()
	csv := filepath.Join(dir, "synthetic.csv")
	require.NoError(t, os.WriteFile(csv, []byte("value\nsynthetic\n"), 0600))
	conn, err := Open(t.Context(), filepath.Join(dir, "mirror.duckdb"))
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.ExecContext(t.Context(), "CREATE TABLE local_values(value INTEGER)")
	require.NoError(t, err)
	_, err = conn.ExecContext(t.Context(), "INSERT INTO local_values VALUES (42)")
	require.NoError(t, err)
	var value int
	require.NoError(t, conn.QueryRowContext(t.Context(), "SELECT value FROM local_values").Scan(&value))
	require.Equal(t, 42, value)
	rows, err := conn.QueryContext(t.Context(), "SELECT * FROM read_csv(?)", csv)
	if rows != nil {
		rows.Close()
	}
	require.Error(t, err)
	remote, err := NewQuackStore(t.Context(), "quack:127.0.0.1:1", "synthetic", false, 0)
	require.ErrorContains(t, err, "removed")
	require.Nil(t, remote)
}
