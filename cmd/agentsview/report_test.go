package main

import (
	"bytes"
	"encoding/json/jsontext"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/db"
)

func TestReportCLIReadOnlyCurationAndMarkdown(t *testing.T) {
	archive := newTestDB(t)
	ctx := t.Context()
	upsertSession(t, archive, "pi:work", "pi", "2026-11-01T00:00:00Z")
	upsertSession(t, archive, "pi:personal", "pi", "2026-11-01T00:00:00Z")
	for _, id := range []string{"pi:work", "pi:personal"} {
		require.NoError(t, archive.InsertMessages(ctx, []db.Message{{SessionID: id, Ordinal: 0, Role: "assistant", Content: "synthetic zephyr", Timestamp: "2026-11-04T00:00:00Z", Model: "model-a", ProviderID: "backend-a", TokenUsage: jsontext.Value(`{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":30,"cache_creation_input_tokens":10}`)}}))
	}
	name := "[unsafe](https://example.invalid) | title\nnext"
	require.NoError(t, archive.RenameSession(ctx, "pi:work", &name))
	path := archive.Path()
	require.NoError(t, archive.Close())
	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		cmd := newReportCommand()
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append([]string{"--db", path}, args...))
		err := cmd.ExecuteContext(ctx)
		return out.String(), err
	}
	ids, err := run("sessions", "--query", "zephyr", "--ids-only")
	require.NoError(t, err)
	assert.Equal(t, "pi:personal\npi:work\n", ids)
	exclude := filepath.Join(t.TempDir(), "exclude.txt")
	require.NoError(t, os.WriteFile(exclude, []byte("# omit personal\npi:personal\n"), 0o600))
	monthly, err := run("monthly", "--month", "2026-11", "--exclude", exclude, "--member", "member-a")
	require.NoError(t, err)
	assert.Contains(t, monthly, "| 2026-11 | model-a | backend-a | pi | 1 | 100 | 20 | 30 | 10 | 160 | 0 |")
	assert.NotContains(t, monthly, "unsafe")
	assert.NotContains(t, monthly, "synthetic zephyr")
	show, err := run("show", "pi:work")
	require.NoError(t, err)
	assert.Contains(t, show, `\[unsafe\]`)
	assert.Contains(t, show, `\| title next`)
	empty := filepath.Join(t.TempDir(), "empty.txt")
	require.NoError(t, os.WriteFile(empty, nil, 0o600))
	out, err := run("sessions", "--include", empty, "--ids-only")
	require.NoError(t, err)
	assert.Empty(t, out)
	_, err = run("monthly", "--month", "not-a-month")
	require.Error(t, err)
	_, err = run("monthly", "--month", "2026-11", "--from", "2026-10")
	require.Error(t, err)
	_, err = run("monthly", "--from", "2026-12", "--to", "2026-11")
	require.Error(t, err)
	_, err = run("monthly", "--timezone", "not-a-zone")
	require.Error(t, err)
}
