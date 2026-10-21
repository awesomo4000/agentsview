package db

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionSummarySearchAndPreservation(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "archive.db")
	d, err := Open(ctx, path)
	require.NoError(t, err)
	insertSession(t, d, "one", "alpha")
	insertSession(t, d, "two", "beta")
	insertMessages(t, d, userMsg("one", 0, "original transcript"), userMsg("two", 0, "different transcript"))
	_, err = d.SetSessionLabels(ctx, "one", []string{"workstream=auth"})
	require.NoError(t, err)
	_, err = d.SetSessionSummary(ctx, "one", "  Investigated zephyr authentication retries <local>  ")
	require.NoError(t, err)
	page, err := d.Search(ctx, SearchFilter{Query: "zephyr", Limit: 1})
	require.NoError(t, err)
	require.Len(t, page.Results, 1)
	assert.Equal(t, "one", page.Results[0].SessionID)
	assert.Contains(t, page.Results[0].Snippet, "&lt;local&gt;")
	page, err = d.Search(ctx, SearchFilter{Query: "zephyr", Project: "beta"})
	require.NoError(t, err)
	assert.Empty(t, page.Results)
	// A parser refresh changes transcript data without replacing operator annotations.
	require.NoError(t, d.ReplaceSessionMessages(ctx, "one", []Message{userMsg("one", 0, "fresh transcript")}))
	require.NoError(t, d.Close())
	d, err = Open(ctx, path)
	require.NoError(t, err)
	defer d.Close()
	summary, err := d.GetSessionSummary(ctx, "one")
	require.NoError(t, err)
	assert.Equal(t, "Investigated zephyr authentication retries <local>", summary.Content)
	session, err := d.GetSession(ctx, "one")
	require.NoError(t, err)
	assert.Equal(t, []string{"workstream=auth"}, session.Labels)
	// Rebuild metadata copies retain summaries and populate their new FTS index.
	copy, err := Open(ctx, filepath.Join(t.TempDir(), "copy.db"))
	require.NoError(t, err)
	defer copy.Close()
	insertSession(t, copy, "one", "alpha")
	insertMessages(t, copy, userMsg("one", 0, "fresh transcript"))
	require.NoError(t, copy.CopySessionMetadataFrom(path))
	page, err = copy.Search(ctx, SearchFilter{Query: "zephyr"})
	require.NoError(t, err)
	require.Len(t, page.Results, 1)
	_, err = d.SetSessionSummary(ctx, "one", "resolved aurora issue")
	require.NoError(t, err)
	page, err = d.Search(ctx, SearchFilter{Query: "zephyr"})
	require.NoError(t, err)
	assert.Empty(t, page.Results)
	_, err = d.SetSessionSummary(ctx, "one", "")
	require.NoError(t, err)
	page, err = d.Search(ctx, SearchFilter{Query: "aurora"})
	require.NoError(t, err)
	assert.Empty(t, page.Results)
	_, err = d.SetSessionSummary(ctx, "one", strings.Repeat("x", 16001))
	require.ErrorIs(t, err, ErrSessionSummaryInvalid)
}

func TestSessionSummarySearchWithoutMessages(t *testing.T) {
	d := testDB(t)
	insertSession(t, d, "one", "alpha")
	insertSession(t, d, "two", "alpha")
	for _, id := range []string{"one", "two"} {
		_, err := d.SetSessionSummary(t.Context(), id, "Zephyr release planning")
		require.NoError(t, err)
	}
	page, err := d.Search(t.Context(), SearchFilter{Query: "zephyr", Limit: 1})
	require.NoError(t, err)
	require.Len(t, page.Results, 1)
	assert.Equal(t, "one", page.Results[0].SessionID)
	require.NotZero(t, page.NextCursor)
	page, err = d.Search(t.Context(), SearchFilter{Query: "zephyr", Limit: 1, Cursor: page.NextCursor})
	require.NoError(t, err)
	require.Len(t, page.Results, 1)
	assert.Equal(t, "two", page.Results[0].SessionID)
}
