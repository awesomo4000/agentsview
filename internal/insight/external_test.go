package insight

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/harness"
)

func TestExternalUsesConfiguredIdentityAndPreservesAttempt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executable requires POSIX shell")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "external")
	ledger := filepath.Join(dir, "executions.db")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\ncat >/dev/null\nprintf '%s' '{\"schema_version\":\"agentsview.execution-result.v1\",\"result\":\"synthetic answer\",\"completion\":{\"reason\":\"stop\"}}'\n"), 0700))
	result, err := GenerateStreamWithOptions(context.Background(), "external", "synthetic prompt", nil, GenerateOptions{External: &harness.Config{ID: "demo-cli", Binary: binary, RequestedModel: "requested-only"}, LedgerPath: ledger})
	require.NoError(t, err)
	require.Equal(t, "demo-cli", result.Agent)
	require.Equal(t, "synthetic answer", result.Content)
	require.Empty(t, result.Model)
	require.NotEmpty(t, result.ExecutionID)
	require.NotNil(t, result.Finish)
	require.NoError(t, result.Finish(123))
	record, err := harness.Read(ledger, result.ExecutionID)
	require.NoError(t, err)
	require.Equal(t, "saved", record.Status)
	require.EqualValues(t, 123, *record.InsightID)
	require.Nil(t, record.Observation.Usage)
}
