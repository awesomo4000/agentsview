package harness

import (
	"context"
	"encoding/json/v2"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExecutionAccounting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executable requires POSIX shell")
	}
	fixture, err := os.ReadFile("../../x/external-harness-result.example.json")
	require.NoError(t, err)
	for _, tc := range []struct {
		name, reason, exit, status string
		fail                       bool
	}{
		{"complete", "stop", "0", "response_received", false},
		{"truncated", "length", "0", "failed", true},
		{"process failure", "stop", "9", "failed", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			binary := filepath.Join(dir, "client")
			path := filepath.Join(dir, "ledger.db")
			var e Envelope
			require.NoError(t, json.Unmarshal(fixture, &e))
			e.Completion.Reason = tc.reason
			payload, err := json.Marshal(e)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\n[ \"$1\" = \"--pipe\" ] || exit 8\n[ \"$(cat)\" = \"synthetic prompt\" ] || exit 7\ncat <<'RESULT'\n"+string(payload)+"\nRESULT\nexit "+tc.exit+"\n"), 0700))
			cfg := Config{ID: "demo-cli", Binary: binary, Args: []string{"--pipe"}, BackendID: "backend-a", DeploymentID: "deployment-a", RequestedModel: "requested-alias"}
			r, answer, err := Run(context.Background(), cfg, "synthetic prompt", path)
			if tc.fail {
				require.Error(t, err)
				require.Empty(t, answer)
			} else {
				require.NoError(t, err)
				require.Equal(t, "Synthetic summary for project demo.", answer)
			}
			persisted, err := Read(path, r.ID)
			require.NoError(t, err)
			require.Equal(t, tc.status, persisted.Status)
			require.NotNil(t, persisted.Observation)
			require.NotNil(t, persisted.Observation.Usage)
			require.EqualValues(t, 1450, *persisted.Observation.Usage.Total)
			require.Nil(t, persisted.Observation.Usage.CacheWrite)
			require.Equal(t, "requested-alias", persisted.RequestedModel)
			require.Equal(t, "demo-model-revision-1", *persisted.Observation.Identity["reported_model"].Value)
			require.Empty(t, persisted.Observation.Result)
			require.NotNil(t, persisted.Ended)
			require.NotNil(t, persisted.ExitCode)
			if !tc.fail {
				require.NoError(t, Finish(path, r.ID, 42))
				require.NoError(t, Finish(path, r.ID, 0))
				saved, err := Read(path, r.ID)
				require.NoError(t, err)
				require.Equal(t, "saved", saved.Status)
				require.EqualValues(t, 42, *saved.InsightID)
				cfg.BackendID = "backend-b"
				cfg.DeploymentID = "deployment-b"
				other, _, err := Run(context.Background(), cfg, "synthetic prompt", path)
				require.NoError(t, err)
				require.NotEqual(t, r.ID, other.ID)
				require.Equal(t, "backend-b", other.BackendID)
				require.Equal(t, "backend-a", persisted.BackendID)
			}
		})
	}
}
func TestTextAndCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executable requires POSIX shell")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "client")
	path := filepath.Join(dir, "ledger.db")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\ncat >/dev/null\nprintf 'plain answer'\n"), 0700))
	cfg := Config{ID: "plain-client", Binary: binary, Format: "text"}
	r, answer, err := Run(context.Background(), cfg, "synthetic", path)
	require.NoError(t, err)
	require.Equal(t, "plain answer", answer)
	require.Nil(t, r.Observation)
	require.NoError(t, Finish(path, r.ID, 12))
	saved, err := Read(path, r.ID)
	require.NoError(t, err)
	require.Equal(t, "saved_unverified", saved.Status)
	cfg.Format = "json"
	r, _, err = Run(context.Background(), cfg, "synthetic", path)
	require.Error(t, err)
	require.Equal(t, "failed", r.Status)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, _, err = Run(ctx, cfg, "synthetic", path)
	require.Error(t, err)
	require.Equal(t, "failed", r.Status)
	require.Nil(t, r.ExitCode)
}
func TestEnvelopeRejectsAmbiguity(t *testing.T) {
	for _, input := range []string{
		`{"schema_version":"agentsview.execution-result.v1","result":"yes","result":"no","completion":{"reason":"stop"}}`,
		`{"schema_version":"agentsview.execution-result.v1","result":"yes","completion":{"reason":"stop"},"usage":{"input_tokens":-1}}`,
		`{"schema_version":"agentsview.execution-result.v1","result":"yes","completion":{"reason":"unknown"}}`,
	} {
		_, err := decode([]byte(input))
		require.Error(t, err)
	}
}

func TestOversizedOutputIsNeverSaved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executable requires POSIX shell")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "client")
	path := filepath.Join(dir, "ledger.db")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\ncat >/dev/null\nexec awk 'BEGIN { for(i=0;i<1000000;i++) print \"0123456789\" }'\n"), 0700))
	r, answer, err := Run(context.Background(), Config{ID: "oversized", Binary: binary, Format: "text"}, "synthetic", path)
	require.Error(t, err)
	require.Empty(t, answer)
	saved, err := Read(path, r.ID)
	require.NoError(t, err)
	require.Equal(t, "failed", saved.Status)
	require.NoError(t, Finish(path, r.ID, 1))
	saved, err = Read(path, r.ID)
	require.NoError(t, err)
	require.Nil(t, saved.InsightID)
}
