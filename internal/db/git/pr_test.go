package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPRLookupNeverLaunchesGH(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "called")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0700))
	t.Setenv("PATH", dir)
	result, err := AggregatePRs(t.Context(), dir, "2026-01-01", "2026-12-31", "synthetic-token")
	require.NoError(t, err)
	require.Nil(t, result)
	_, err = os.Stat(marker)
	require.True(t, os.IsNotExist(err))
}
