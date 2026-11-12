package db

import (
	"encoding/json/jsontext"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenReportMonthsModelsSelectionAndCoverage(t *testing.T) {
	d := testDB(t)
	ctx := t.Context()
	for _, id := range []string{"one", "two", "missing"} {
		insertSession(t, d, id, "project-hint", func(s *Session) {
			s.Agent = "pi"
			s.StartedAt = Ptr("2026-10-01T00:00:00Z")
			s.EndedAt = Ptr("2026-11-03T00:00:00Z")
		})
	}
	insertMessages(t, d,
		Message{SessionID: "one", Ordinal: 0, Role: "user", Content: "zephyr workstream"},
		Message{SessionID: "one", Ordinal: 1, Role: "assistant", Timestamp: "2026-10-31T23:30:00Z", Model: "model-a", ProviderID: "backend-a", TokenUsage: jsontext.Value(`{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":30,"cache_creation_input_tokens":10}`)},
		Message{SessionID: "one", Ordinal: 2, Role: "assistant", Timestamp: "2026-11-01T00:30:00Z", Model: "model-a", ProviderID: "backend-b", TokenUsage: jsontext.Value(`{"input_tokens":200,"output_tokens":40}`)},
		Message{SessionID: "two", Ordinal: 0, Role: "assistant", Timestamp: "2026-11-02T00:00:00Z", Model: "model-b", TokenUsage: jsontext.Value(`{"input_tokens":7,"output_tokens":3}`)},
	)
	_, err := d.SetSessionSummary(ctx, "two", "zephyr followup")
	require.NoError(t, err)
	got, err := d.TokenReport(ctx, TokenReportOptions{From: "2026-10-01", To: "2026-11-30", Query: "zephyr"})
	require.NoError(t, err)
	require.Len(t, got.Sessions, 2)
	require.Len(t, got.Usage, 3)
	assert.Equal(t, TokenReportUsage{SessionID: "one", Month: "2026-10", Model: "model-a", Provider: "backend-a", Input: 100, Output: 20, CacheRead: 30, CacheWrite: 10, Records: 1}, got.Usage[0])
	assert.Equal(t, "backend-b", got.Usage[1].Provider)
	assert.Empty(t, got.Usage[2].Provider, "unknown backend must not be guessed from model")
	selected, err := d.TokenReport(ctx, TokenReportOptions{From: "2026-11-01", To: "2026-11-30", IncludeIDs: []string{"one", "two"}, ExcludeIDs: []string{"two"}})
	require.NoError(t, err)
	require.Len(t, selected.Usage, 1)
	assert.EqualValues(t, 200, selected.Usage[0].Input)
	pacific, err := d.TokenReport(ctx, TokenReportOptions{From: "2026-10-01", To: "2026-10-31", Timezone: "America/Los_Angeles", Models: []string{"model-a"}})
	require.NoError(t, err)
	require.Len(t, pacific.Usage, 2)
	assert.Equal(t, "2026-10", pacific.Usage[1].Month)
	empty, err := d.TokenReport(ctx, TokenReportOptions{IncludeIDs: []string{}})
	require.NoError(t, err)
	assert.Empty(t, empty.Sessions)
	assert.Empty(t, empty.Usage)
	_, err = d.TokenReport(ctx, TokenReportOptions{IncludeIDs: []string{"typo"}})
	require.ErrorContains(t, err, "unknown session ID")
	all, err := d.TokenReport(ctx, TokenReportOptions{})
	require.NoError(t, err)
	assert.Len(t, all.Sessions, 3)
	require.NoError(t, d.Close())
	ro, err := OpenReadOnly(ctx, filepath.Clean(d.Path()))
	require.NoError(t, err)
	defer ro.Close()
	again, err := ro.TokenReport(ctx, TokenReportOptions{Models: []string{"model-b"}})
	require.NoError(t, err)
	require.Len(t, again.Usage, 1)
	assert.EqualValues(t, 7, again.Usage[0].Input)
}

func TestTokenReportKeepsCanonicalSnapshotOwnership(t *testing.T) {
	d := testDB(t)
	ctx := t.Context()
	for _, id := range []string{"owner", "copy"} {
		insertSession(t, d, id, "project", func(s *Session) { s.Agent = "claude"; s.StartedAt = Ptr("2026-10-01T00:00:00Z") })
	}
	insertMessages(t, d,
		Message{SessionID: "owner", Ordinal: 0, Role: "assistant", Timestamp: "2026-10-01T10:00:00Z", Model: "model-a", ClaudeMessageID: "request-message", ClaudeRequestID: "request-id", TokenUsage: jsontext.Value(`{"input_tokens":10,"output_tokens":5}`)},
		Message{SessionID: "copy", Ordinal: 0, Role: "assistant", Timestamp: "2026-10-01T10:00:01Z", Model: "model-a", ClaudeMessageID: "request-message", ClaudeRequestID: "request-id", TokenUsage: jsontext.Value(`{"input_tokens":10,"output_tokens":30}`)},
	)
	got, err := d.TokenReport(ctx, TokenReportOptions{})
	require.NoError(t, err)
	require.Len(t, got.Usage, 1)
	assert.Equal(t, "owner", got.Usage[0].SessionID)
	assert.EqualValues(t, 30, got.Usage[0].Output)
	filtered, err := d.TokenReport(ctx, TokenReportOptions{ExcludeIDs: []string{"owner"}})
	require.NoError(t, err)
	assert.Empty(t, filtered.Usage, "excluding the owner must not reassign copied usage")
}

func TestTokenReportIncludesUsageWithoutTranscriptMessages(t *testing.T) {
	d := testDB(t)
	insertSession(t, d, "usage-only", "project", func(s *Session) {
		s.Agent = "codex"
		s.StartedAt = Ptr("2026-10-01T00:00:00Z")
	})
	require.NoError(t, d.ReplaceSessionUsageEvents(t.Context(), "usage-only", []UsageEvent{{
		Source: "session", Model: "model-a", ProviderID: "backend-a",
		InputTokens: 100, OutputTokens: 20, CacheReadInputTokens: 30,
		ReasoningTokens: 5, OccurredAt: "2026-11-02T12:00:00Z", DedupKey: "event-one",
	}}))
	got, err := d.TokenReport(t.Context(), TokenReportOptions{From: "2026-11-01", To: "2026-11-30"})
	require.NoError(t, err)
	require.Len(t, got.Usage, 1)
	assert.Equal(t, TokenReportUsage{SessionID: "usage-only", Month: "2026-11", Model: "model-a", Provider: "backend-a", Input: 100, Output: 20, CacheRead: 30, Reasoning: 5, Records: 1}, got.Usage[0])
}
