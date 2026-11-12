package db_test

import (
	"database/sql"
	"encoding/json"
	"encoding/json/jsontext"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/dbtest"
	"go.kenn.io/agentsview/internal/parser"
	"go.kenn.io/agentsview/internal/sync"
	"go.kenn.io/agentsview/internal/testjsonl"
)

func TestLegacyUpgradePreservesIncompleteRetry(t *testing.T) {
	database := dbtest.OpenTestDB(t)
	root := t.TempDir()
	id := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	body := testjsonl.CodexForkedSessionMetaJSON(id, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", root, "codex_cli_rs", "2026-10-08T12:00:00Z") + "\n" + testjsonl.CodexMsgJSON("user", "child without parent turns", "2026-10-08T12:00:01Z") + "\n"
	dbtest.WriteTestFile(t, filepath.Join(root, "rollout-"+id+".jsonl"), []byte(body))
	require.NoError(t, database.Update(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), "PRAGMA user_version=129")
		return err
	}))
	engine := sync.NewEngine(t.Context(), database, sync.EngineConfig{Machine: "local", AgentDirs: map[parser.AgentType][]string{parser.AgentCodex: {root}}})
	require.Equal(t, 1, engine.SyncAll(t.Context(), nil).Synced)
	engine.Close()
	require.Equal(t, db.CurrentDataVersion()-1, database.GetSessionDataVersion(t.Context(), "codex:"+id), "missing parent output must remain retryable")
	// Reproduce the retry stamp written by the legacy parser before reopening.
	require.NoError(t, database.SetSessionDataVersion(t.Context(), "codex:"+id, 129))
	path := database.Path()
	require.NoError(t, database.Close())
	reopened, err := db.OpenIsolated(t.Context(), path)
	require.NoError(t, err)
	assert.True(t, reopened.NeedsResync())
	assert.Equal(t, 129, reopened.GetSessionDataVersion(t.Context(), "codex:"+id))
	require.NoError(t, reopened.Close())
}

func TestSimulatedScopedUpgradeReparsesUnchangedFreebuffSource(t *testing.T) {
	version := db.CurrentDataVersion()
	t.Cleanup(db.InstallDataVersionScopesForTest(map[int][]parser.AgentType{version: {parser.AgentCodebuff}}))
	database := dbtest.OpenTestDB(t)
	root, unaffected := t.TempDir(), t.TempDir()
	source := filepath.Join(root, "project", "chats", "2026-09-15T10-00-00.000Z", "chat-messages.json")
	dbtest.WriteTestFile(t, source, []byte(`[{"id":"user-1","variant":"user","content":"hello","timestamp":"10:00 AM"}]`))
	dbtest.WriteTestFile(t, filepath.Join(filepath.Dir(source), "run-state.json"), []byte(`{"sessionState":{"mainAgentState":{"agentType":"base2-free-minimax-m3"}}}`))
	dbtest.WriteTestFile(t, filepath.Join(unaffected, "project", "unchanged.jsonl"), []byte(testjsonl.NewSessionBuilder().AddClaudeUser("2026-10-08T12:00:00Z", "unaffected").String()))
	cfg := sync.EngineConfig{Machine: "local", AgentDirs: map[parser.AgentType][]string{parser.AgentCodebuff: {root}, parser.AgentClaude: {unaffected}}}
	engine := sync.NewEngine(t.Context(), database, cfg)
	require.Equal(t, 2, engine.SyncAll(t.Context(), nil).Synced)
	engine.Close()
	var id string
	require.NoError(t, database.Reader().QueryRow(t.Context(), "SELECT id FROM sessions WHERE agent='freebuff'").Scan(&id))
	messages, err := database.GetAllMessages(t.Context(), id)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	messages[0].Content = "obsolete parser output"
	messages[0].ContentLength = len(messages[0].Content)
	require.NoError(t, database.ReplaceSessionMessages(t.Context(), id, messages))
	_, found, err := database.GetProviderStatHash(t.Context(), parser.AgentCodebuff, source)
	require.NoError(t, err)
	require.True(t, found)
	require.NoError(t, database.Update(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), fmt.Sprintf("UPDATE sessions SET data_version=%d; PRAGMA user_version=%d", version-1, version-1))
		return err
	}))
	path := database.Path()
	require.NoError(t, database.Close())
	database, err = db.OpenIsolated(t.Context(), path)
	require.NoError(t, err)
	needsSync, _, err := db.ArchiveResyncState(t.Context(), path)
	require.NoError(t, err)
	require.True(t, needsSync)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	cfg.AgentDirs[parser.AgentCodebuff] = []string{root, unaffected}
	dbtest.WriteTestFile(t, filepath.Join(unaffected, "project", "chats", "broken", "chat-messages.json"), []byte("{"))
	engine = sync.NewEngine(t.Context(), database, cfg)
	t.Cleanup(engine.Close)
	stats := engine.SyncAll(t.Context(), nil)
	assert.Equal(t, 1, stats.Synced)
	assert.Equal(t, 1, stats.Skipped)
	assert.Equal(t, 1, stats.Failed)
	needsSync, _, err = db.ArchiveResyncState(t.Context(), path)
	require.NoError(t, err)
	assert.False(t, needsSync)
	assert.Equal(t, source, database.GetSessionFilePath(t.Context(), id))
	messages, err = database.GetAllMessages(t.Context(), id)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, "hello", messages[0].Content)
}

func TestClaudeFormatPRLinkUpgradePreservesSessionIdentity(t *testing.T) {
	providers := []parser.AgentType{parser.AgentClaude, parser.AgentCowork, parser.AgentQoder, parser.AgentIcodemate}
	scope := db.ScopedDataVersionProviders(131, 132)
	assert.ElementsMatch(t, append(providers, parser.AgentGrok), scope)
	version := db.CurrentDataVersion()
	t.Cleanup(db.InstallDataVersionScopesForTest(map[int][]parser.AgentType{version: scope}))
	for _, provider := range providers {
		t.Run(string(provider), func(t *testing.T) {
			database := dbtest.OpenTestDB(t)
			root := t.TempDir()
			sessionID := "c0000000-0000-4000-8000-000000000101"
			source := filepath.Join(root, "project", sessionID+".jsonl")
			if provider == parser.AgentCowork {
				workspace := filepath.Join(root, "org", "workspace")
				localID := "local_50000000-0000-4000-8000-000000000101"
				dbtest.WriteTestFile(t, filepath.Join(workspace, localID+".json"), []byte(`{"sessionId":"`+localID+`","cliSessionId":"`+sessionID+`"}`))
				source = filepath.Join(workspace, localID, ".claude", "projects", "project", sessionID+".jsonl")
			}
			body := testjsonl.NewSessionBuilder().AddClaudeUserWithSessionID("2026-10-08T12:00:00Z", "hello", sessionID).String() + "\n" + `{"type":"pr-link","prNumber":7,"prUrl":"https://github.com/owner/repo/pull/7","prRepository":"owner/repo"}` + "\n"
			dbtest.WriteTestFile(t, source, []byte(body))
			cfg := sync.EngineConfig{Machine: "local", AgentDirs: map[parser.AgentType][]string{provider: {root}}}
			engine := sync.NewEngine(t.Context(), database, cfg)
			require.Equal(t, 1, engine.SyncAll(t.Context(), nil).Synced)
			engine.Close()
			var id string
			require.NoError(t, database.Reader().QueryRow(t.Context(), "SELECT id FROM sessions").Scan(&id))
			require.NoError(t, database.Update(t.Context(), func(tx *sql.Tx) error {
				_, err := tx.ExecContext(t.Context(), fmt.Sprintf("UPDATE sessions SET data_version=%d, pr_links=''; PRAGMA user_version=%d", version-1, version-1))
				return err
			}))
			path := database.Path()
			require.NoError(t, database.Close())
			database, err := db.OpenIsolated(t.Context(), path)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, database.Close()) })
			assert.False(t, database.NeedsResync(), "Claude upgrade must reparse in place")
			engine = sync.NewEngine(t.Context(), database, cfg)
			t.Cleanup(engine.Close)
			require.Equal(t, 1, engine.SyncAll(t.Context(), nil).Synced)
			session, err := database.GetSessionFull(t.Context(), id)
			require.NoError(t, err)
			require.NotNil(t, session)
			assert.Equal(t, source, database.GetSessionFilePath(t.Context(), id))
			require.Len(t, session.PRLinks, 1)
			assert.Equal(t, 7, session.PRLinks[0].Number)
			var count int
			require.NoError(t, database.Reader().QueryRow(t.Context(), "SELECT count(*) FROM sessions").Scan(&count))
			assert.Equal(t, 1, count)
		})
	}
}

func TestGrokUpgradePreservesParserRelationship(t *testing.T) {
	version := db.CurrentDataVersion()
	t.Cleanup(db.InstallDataVersionScopesForTest(map[int][]parser.AgentType{version: db.ScopedDataVersionProviders(131, 132)}))
	database := dbtest.OpenTestDB(t)
	root := t.TempDir()
	id := "grok:019f6000-0000-7000-8000-000000000011"
	source := filepath.Join(root, "cwd-key", "019f6000-0000-7000-8000-000000000011", "summary.json")
	dbtest.WriteTestFile(t, source, []byte(`{"session_kind":"subagent","session_summary":"Review this change","created_at":"2026-07-18T10:01:00Z","num_messages":1}`))
	cfg := sync.EngineConfig{Machine: "local", AgentDirs: map[parser.AgentType][]string{parser.AgentGrok: {root}}}
	engine := sync.NewEngine(t.Context(), database, cfg)
	require.Equal(t, 1, engine.SyncAll(t.Context(), nil).Synced)
	engine.Close()
	require.NoError(t, database.Update(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), fmt.Sprintf("UPDATE sessions SET data_version=%d, parser_relationship_type=NULL; PRAGMA user_version=%d", version-1, version-1))
		return err
	}))
	path := database.Path()
	require.NoError(t, database.Close())
	database, err := db.OpenIsolated(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	engine = sync.NewEngine(t.Context(), database, cfg)
	t.Cleanup(engine.Close)
	require.Equal(t, 1, engine.SyncAll(t.Context(), nil).Synced)
	session, err := database.GetSessionFull(t.Context(), id)
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Empty(t, session.SessionKind)
	assert.Nil(t, session.ParentSessionID)
	assert.Equal(t, "subagent", session.RelationshipType)
	assert.Equal(t, source, database.GetSessionFilePath(t.Context(), id))
	var relationship string
	require.NoError(t, database.Reader().QueryRow(t.Context(), "SELECT parser_relationship_type FROM sessions WHERE id=?", id).Scan(&relationship))
	assert.Equal(t, "subagent", relationship)
}

func TestScopedUpgradeWaitsForAuthoritativeFullPass(t *testing.T) {
	version := db.CurrentDataVersion()
	t.Cleanup(db.InstallDataVersionScopesForTest(
		map[int][]parser.AgentType{version: {parser.AgentCodebuff}},
	))
	database := dbtest.OpenTestDB(t)
	require.NoError(t, database.Update(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), fmt.Sprintf("PRAGMA user_version=%d", version-1))
		return err
	}))
	path := database.Path()
	require.NoError(t, database.Close())
	database, err := db.OpenIsolated(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	require.False(t, database.NeedsResync(), "a scoped upgrade must not force a rebuild")

	waiting := func() (startup, coldRead bool) {
		t.Helper()
		startup, _, err := db.ArchiveResyncState(t.Context(), path)
		require.NoError(t, err)
		reader, err := db.OpenReadOnly(t.Context(), path)
		require.NoError(t, err)
		defer func() { require.NoError(t, reader.Close()) }()
		return startup, reader.NeedsResync()
	}
	startup, coldRead := waiting()
	assert.True(t, startup)
	assert.True(t, coldRead, "cold reads must wait for affected sources")

	clean := sync.EngineConfig{Machine: "local", AgentDirs: map[parser.AgentType][]string{
		parser.AgentCodebuff: {t.TempDir()},
	}}
	broken := t.TempDir()
	dbtest.WriteTestFile(t, filepath.Join(broken, "opencode.db"), []byte("not a database"))
	failing := sync.EngineConfig{Machine: "local", AgentDirs: map[parser.AgentType][]string{
		parser.AgentOpenCode: {broken},
	}}
	passes := []struct {
		name string
		cfg  sync.EngineConfig
		run  func(*sync.Engine) sync.SyncStats
		done bool
	}{
		{
			name: "incremental pass",
			cfg:  clean,
			run: func(e *sync.Engine) sync.SyncStats {
				return e.SyncAllSince(t.Context(), time.Now(), nil)
			},
		},
		{
			name: "full pass with provider failure",
			cfg:  failing,
			run: func(e *sync.Engine) sync.SyncStats {
				return e.SyncAll(t.Context(), nil)
			},
		},
		{
			name: "authoritative full pass",
			cfg:  clean,
			run: func(e *sync.Engine) sync.SyncStats {
				return e.SyncAll(t.Context(), nil)
			},
			done: true,
		},
	}
	for _, pass := range passes {
		engine := sync.NewEngine(t.Context(), database, pass.cfg)
		pass.run(engine)
		engine.Close()
		startup, coldRead := waiting()
		assert.Equal(t, !pass.done, startup, pass.name)
		assert.Equal(t, !pass.done, coldRead, pass.name)
	}
}

func TestCodexGitBranchUpgradeReparsesCodexInPlace(t *testing.T) {
	scope := db.ScopedDataVersionProviders(132, 133)
	assert.ElementsMatch(t,
		[]parser.AgentType{parser.AgentCodex, parser.AgentTraeX, parser.AgentAugureCode}, scope)
	version := db.CurrentDataVersion()
	t.Cleanup(db.InstallDataVersionScopesForTest(map[int][]parser.AgentType{version: scope}))
	ctx := t.Context()
	database := dbtest.OpenTestDB(t)
	codexRoot, claudeRoot := t.TempDir(), t.TempDir()
	codexID := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	codexSource := filepath.Join(codexRoot, "rollout-"+codexID+".jsonl")
	meta := testjsonl.CodexSessionMetaWithFieldsJSON(
		codexID, codexRoot, "codex_cli_rs", "2026-10-08T12:00:00Z",
		map[string]any{"git": map[string]any{"branch": "feat/rate-limit"}},
	)
	body := meta + "\n" +
		testjsonl.CodexMsgJSON("user", "add rate limiting", "2026-10-08T12:00:01Z") + "\n"
	dbtest.WriteTestFile(t, codexSource, []byte(body))
	dbtest.WriteTestFile(t, filepath.Join(claudeRoot, "project", "unchanged.jsonl"),
		[]byte(testjsonl.NewSessionBuilder().AddClaudeUser("2026-10-08T12:00:00Z", "unaffected").String()))
	cfg := sync.EngineConfig{Machine: "local", AgentDirs: map[parser.AgentType][]string{
		parser.AgentCodex: {codexRoot}, parser.AgentClaude: {claudeRoot},
	}}
	engine := sync.NewEngine(ctx, database, cfg)
	require.Equal(t, 2, engine.SyncAll(ctx, nil).Synced)
	engine.Close()
	var claudeID string
	require.NoError(t, database.Reader().QueryRow(ctx,
		"SELECT id FROM sessions WHERE agent='claude'").Scan(&claudeID))

	// Reproduce an archive from the version before the Codex branch upgrade.
	require.NoError(t, database.Update(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, fmt.Sprintf(
			"UPDATE sessions SET git_branch='', data_version=%d; PRAGMA user_version=%d",
			version-1, version-1))
		return err
	}))
	path := database.Path()
	require.NoError(t, database.Close())
	database, err := db.OpenIsolated(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	needsSync, fullRebuild, err := db.ArchiveResyncState(ctx, path)
	require.NoError(t, err)
	assert.True(t, needsSync)
	assert.False(t, fullRebuild, "the Codex branch upgrade must not rebuild the archive")
	assert.Equal(t, version, database.GetSessionDataVersion(ctx, claudeID))
	assert.Equal(t, version-1, database.GetSessionDataVersion(ctx, "codex:"+codexID))

	engine = sync.NewEngine(ctx, database, cfg)
	t.Cleanup(engine.Close)
	assert.Equal(t, 1, engine.SyncAll(ctx, nil).Synced)
	needsSync, _, err = db.ArchiveResyncState(ctx, path)
	require.NoError(t, err)
	assert.False(t, needsSync)
	codex, err := database.GetSessionFull(ctx, "codex:"+codexID)
	require.NoError(t, err)
	require.NotNil(t, codex)
	require.NotNil(t, codex.FilePath)
	assert.Equal(t, codexSource, *codex.FilePath)
	assert.Equal(t, "feat/rate-limit", codex.GitBranch)
	assert.Equal(t, version, codex.DataVersion)
	var sessions int
	require.NoError(t, database.Reader().QueryRow(ctx, "SELECT count(*) FROM sessions").Scan(&sessions))
	assert.Equal(t, 2, sessions, "the reparse must keep the existing session identities")
}

func TestPiSubagentRelationshipUpgradeReparsesInPlace(t *testing.T) {
	scope := db.ScopedDataVersionProviders(134, 135)
	assert.ElementsMatch(t, []parser.AgentType{
		parser.AgentPi, parser.AgentOMP, parser.AgentOMO, parser.AgentPrimeAgent, parser.AgentStepCode,
	}, scope)
	version := db.CurrentDataVersion()
	t.Cleanup(db.InstallDataVersionScopesForTest(map[int][]parser.AgentType{version: scope}))
	ctx := t.Context()
	database := dbtest.OpenTestDB(t)
	root := t.TempDir()
	stem := "2026-10-09T10-00-00-000Z_parent-uuid"
	parentPath := filepath.Join(root, "encoded-cwd", stem+".jsonl")
	childPath := filepath.Join(root, "encoded-cwd", stem, "run-abc", "run-0", "session.jsonl")
	dbtest.WriteTestFile(t, parentPath, []byte(
		`{"type":"session","version":3,"id":"parent-uuid","timestamp":"2026-10-09T10:00:00Z","cwd":"/work/project"}`+"\n"+
			`{"type":"message","id":"p1","timestamp":"2026-10-09T10:00:01Z","message":{"role":"user","content":"run a subagent"}}`+"\n"))
	parentPathJSON, err := json.Marshal(parentPath)
	require.NoError(t, err)
	dbtest.WriteTestFile(t, childPath, []byte(
		`{"type":"session","version":3,"id":"child-uuid","timestamp":"2026-10-09T10:05:00Z","cwd":"/work/project","parentSession":`+string(parentPathJSON)+`}`+"\n"+
			`{"type":"message","id":"c1","timestamp":"2026-10-09T10:05:01Z","message":{"role":"user","content":"do the task"}}`+"\n"+
			`{"type":"message","id":"c2","timestamp":"2026-10-09T10:05:02Z","message":{"role":"assistant","content":"task done","thinkingLevel":"high"}}`+"\n"))
	cfg := sync.EngineConfig{Machine: "local", AgentDirs: map[parser.AgentType][]string{parser.AgentPi: {root}}}
	engine := sync.NewEngine(ctx, database, cfg)
	require.Equal(t, 2, engine.SyncAll(ctx, nil).Synced)
	engine.Close()
	const id = "pi:child-uuid"
	// Reproduce the previous parser, which typed the child as a fork.
	require.NoError(t, database.Update(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			fmt.Sprintf("UPDATE sessions SET relationship_type='fork' WHERE id=?; UPDATE messages SET reasoning_effort=''; UPDATE sessions SET data_version=%d; PRAGMA user_version=%d", version-1, version-1), id)
		return err
	}))
	path := database.Path()
	require.NoError(t, database.Close())
	database, err = db.OpenIsolated(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, fullRebuild, err := db.ArchiveResyncState(ctx, path)
	require.NoError(t, err)
	assert.False(t, fullRebuild, "the Pi subagent upgrade must reparse in place")

	engine = sync.NewEngine(ctx, database, cfg)
	t.Cleanup(engine.Close)
	require.Equal(t, 2, engine.SyncAll(ctx, nil).Synced)
	var rel, parent string
	require.NoError(t, database.Reader().QueryRow(ctx,
		"SELECT relationship_type, parent_session_id FROM sessions WHERE id=?", id).Scan(&rel, &parent))
	assert.Equal(t, "subagent", rel)
	assert.Equal(t, "pi:parent-uuid", parent)
	assert.Equal(t, childPath, database.GetSessionFilePath(ctx, id), "source path is unchanged")
	var effort string
	require.NoError(t, database.Reader().QueryRow(ctx, "SELECT reasoning_effort FROM messages WHERE session_id=? AND role='assistant'", id).Scan(&effort))
	assert.Equal(t, "high", effort)
	var sessions int
	require.NoError(t, database.Reader().QueryRow(ctx, "SELECT count(*) FROM sessions").Scan(&sessions))
	assert.Equal(t, 2, sessions)
}

func TestOMPParentLinkUpgradePreservesSessionIdentityAndCuration(t *testing.T) {
	scope := db.ScopedDataVersionProviders(135, 136)
	require.Equal(t, []parser.AgentType{parser.AgentOMP}, scope)
	version := db.CurrentDataVersion()
	t.Cleanup(db.InstallDataVersionScopesForTest(map[int][]parser.AgentType{version: scope}))
	ctx := t.Context()
	database := dbtest.OpenTestDB(t)
	ompRoot, piRoot := t.TempDir(), t.TempDir()
	project := filepath.Join(ompRoot, "synthetic-project")
	rootStem := "2026-09-20T10-00-00-000Z_root-file-token"
	parentSource := filepath.Join(project, rootStem+".jsonl")
	childSource := filepath.Join(project, rootStem, "Worker.jsonl")
	grandchildSource := filepath.Join(project, rootStem, "Worker", "Reviewer.jsonl")
	unavailableSource := filepath.Join(project, "unavailable-file-token.jsonl")
	piSource := filepath.Join(piRoot, "synthetic-project", "unaffected.jsonl")
	writeOMP := func(source, id, parent, prompt string) {
		t.Helper()
		body := fmt.Sprintf(`{"type":"title","v":1,"title":"","pad":"   "}
{"type":"session","version":3,"id":%q,"timestamp":"2026-09-20T10:00:00Z","cwd":"/synthetic/project","parentSession":%q}
{"type":"message","id":"user-message","timestamp":"2026-09-20T10:00:01Z","message":{"role":"user","content":%q}}
`, id, parent, prompt)
		dbtest.WriteTestFile(t, source, []byte(body))
	}
	writeOMP(parentSource, "parent-header-id", "", "Synthetic parent task")
	writeOMP(childSource, "child-header-id", parentSource, "Synthetic worker task")
	writeOMP(grandchildSource, "grandchild-header-id", childSource, "Synthetic review task")
	writeOMP(unavailableSource, "unavailable-header-id", "", "Synthetic retained task")
	dbtest.WriteTestFile(t, piSource, []byte(`{"type":"session","version":3,"id":"unaffected-pi-id","timestamp":"2026-09-20T10:00:00Z","cwd":"/synthetic/project"}
{"type":"message","id":"pi-user-message","timestamp":"2026-09-20T10:00:01Z","message":{"role":"user","content":"Synthetic Pi task"}}
`))
	cfg := sync.EngineConfig{Machine: "local", AgentDirs: map[parser.AgentType][]string{
		parser.AgentOMP: {ompRoot}, parser.AgentPi: {piRoot},
	}}
	engine := sync.NewEngine(ctx, database, cfg)
	t.Cleanup(engine.Close)
	require.Equal(t, 5, engine.SyncAll(ctx, nil).Synced)
	engine.Close()

	const childID = "omp:child-header-id"
	displayName, note := "Saved worker task", "Keep this worker prompt"
	require.NoError(t, database.RenameSession(ctx, childID, &displayName))
	_, err := database.SetSessionLabels(ctx, childID, []string{"workstream=review"})
	require.NoError(t, err)
	_, err = database.SetSessionSummary(ctx, childID, "Synthetic handoff summary")
	require.NoError(t, err)
	starred, err := database.StarSession(ctx, childID)
	require.NoError(t, err)
	require.True(t, starred)
	childMessages, err := database.GetAllMessages(ctx, childID)
	require.NoError(t, err)
	require.Len(t, childMessages, 1)
	pinnedMessageID := childMessages[0].ID
	pinID, err := database.PinMessage(ctx, childID, pinnedMessageID, &note)
	require.NoError(t, err)
	require.NotZero(t, pinID)

	// Pi shares the parser, but its archived output must not be invalidated by
	// this OMP-only upgrade. A reparse would replace this stored sentinel.
	piMessages, err := database.GetAllMessages(ctx, "pi:unaffected-pi-id")
	require.NoError(t, err)
	require.Len(t, piMessages, 1)
	piMessages[0].Content = "Retained Pi archive output"
	piMessages[0].ContentLength = len(piMessages[0].Content)
	require.NoError(t, database.ReplaceSessionMessages(ctx, "pi:unaffected-pi-id", piMessages))

	expected := []struct {
		id, source, parent, name, content string
		wantVersion                       int
	}{
		{"omp:parent-header-id", parentSource, "", "", "Synthetic parent task", version},
		{childID, childSource, "omp:parent-header-id", "Worker", "Synthetic worker task", version},
		{"omp:grandchild-header-id", grandchildSource, childID, "Reviewer", "Synthetic review task", version},
		{"omp:unavailable-header-id", unavailableSource, "", "", "Synthetic retained task", version - 1},
		{"pi:unaffected-pi-id", piSource, "", "", "Retained Pi archive output", version},
	}
	before := make(map[string]*db.Session, len(expected))
	for _, want := range expected {
		session, err := database.GetSessionFull(ctx, want.id)
		require.NoError(t, err)
		require.NotNil(t, session, want.id)
		before[want.id] = session
	}
	require.NoError(t, os.Remove(unavailableSource))

	// Reproduce the old parser's verbatim path parents and missing subagent
	// classifications without touching source bytes or source identities.
	require.NoError(t, database.Update(ctx, func(tx *sql.Tx) error {
		for _, old := range []struct{ id, parentSource string }{
			{childID, parentSource},
			{"omp:grandchild-header-id", childSource},
		} {
			if _, err := tx.ExecContext(ctx, `UPDATE sessions
				SET parent_session_id=?, parser_parent_session_id=?,
				    relationship_type='', parser_relationship_type='', session_name=NULL
				WHERE id=?`, "omp:"+old.parentSource, "omp:"+old.parentSource, old.id); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE sessions SET data_version=?", version-1); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", version-1))
		return err
	}))
	path := database.Path()
	require.NoError(t, database.Close())
	database, err = db.OpenIsolated(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	require.False(t, database.NeedsResync(), "the OMP repair must upgrade in place")
	needsSync, fullRebuild, err := db.ArchiveResyncState(ctx, path)
	require.NoError(t, err)
	assert.True(t, needsSync)
	assert.False(t, fullRebuild)
	assert.Equal(t, version-1, database.GetSessionDataVersion(ctx, childID))
	assert.Equal(t, version, database.GetSessionDataVersion(ctx, "pi:unaffected-pi-id"))

	engine = sync.NewEngine(ctx, database, cfg)
	t.Cleanup(engine.Close)
	stats := engine.SyncAll(ctx, nil)
	assert.Equal(t, 3, stats.Synced, "only the available OMP sources need reparsing")
	assert.Equal(t, 1, stats.Skipped)
	assert.Zero(t, stats.Failed)
	assert.False(t, stats.Aborted)
	for _, want := range expected {
		t.Run(want.id, func(t *testing.T) {
			after, err := database.GetSessionFull(ctx, want.id)
			require.NoError(t, err)
			require.NotNil(t, after)
			assert.Equal(t, before[want.id].ID, after.ID)
			require.NotNil(t, after.FilePath)
			assert.Equal(t, want.source, *after.FilePath)
			assert.Equal(t, before[want.id].FileHash, after.FileHash)
			assert.Equal(t, before[want.id].SourceSessionID, after.SourceSessionID)
			assert.Equal(t, want.wantVersion, after.DataVersion)
			var parserRelationship string
			require.NoError(t, database.Reader().QueryRow(ctx,
				"SELECT COALESCE(parser_relationship_type, '') FROM sessions WHERE id=?", want.id,
			).Scan(&parserRelationship))
			if want.parent == "" {
				assert.Nil(t, after.ParentSessionID)
				assert.Nil(t, after.ParserParentSessionID)
				assert.Empty(t, after.RelationshipType)
				assert.Empty(t, parserRelationship)
			} else {
				assert.Equal(t, &want.parent, after.ParentSessionID)
				assert.Equal(t, &want.parent, after.ParserParentSessionID)
				assert.Equal(t, "subagent", after.RelationshipType)
				assert.Equal(t, "subagent", parserRelationship)
				assert.Equal(t, &want.name, after.SessionName)
			}
			messages, err := database.GetAllMessages(ctx, want.id)
			require.NoError(t, err)
			require.Len(t, messages, 1)
			assert.Equal(t, want.content, messages[0].Content)
		})
	}

	child, err := database.GetSessionFull(ctx, childID)
	require.NoError(t, err)
	require.NotNil(t, child)
	assert.Equal(t, &displayName, child.DisplayName)
	assert.Equal(t, []string{"workstream=review"}, child.Labels)
	summary, err := database.GetSessionSummary(ctx, childID)
	require.NoError(t, err)
	assert.Equal(t, "Synthetic handoff summary", summary.Content)
	stars, err := database.ListStarredSessionIDs(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{childID}, stars)
	// The all-pins view includes the joined message content.
	pins, err := database.ListPinnedMessages(ctx, "", "")
	require.NoError(t, err)
	require.Len(t, pins, 1)
	assert.Equal(t, childID, pins[0].SessionID)
	assert.Equal(t, pinnedMessageID, pins[0].MessageID)
	assert.Equal(t, &note, pins[0].Note)
	assert.Equal(t, &childMessages[0].Content, pins[0].Content)
	var count int
	require.NoError(t, database.Reader().QueryRow(ctx, "SELECT count(*) FROM sessions").Scan(&count))
	assert.Equal(t, 5, count, "the upgrade must not replace or duplicate session identities")
}

func TestCodexCodeModeWaitOutputUpgradeReparsesInPlace(t *testing.T) {
	scope := db.ScopedDataVersionProviders(136, 137)
	assert.ElementsMatch(t,
		[]parser.AgentType{parser.AgentCodex, parser.AgentTraeX, parser.AgentAugureCode},
		scope)
	version := db.CurrentDataVersion()
	t.Cleanup(db.InstallDataVersionScopesForTest(map[int][]parser.AgentType{version: scope}))
	ctx := t.Context()
	database := dbtest.OpenTestDB(t)
	root := t.TempDir()
	const id = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	const output = testjsonl.CodexCodeModeWaitOutput
	sessionID := "codex:" + id
	source := filepath.Join(root, "rollout-"+id+".jsonl")
	body := testjsonl.JoinJSONL(
		testjsonl.CodexSessionMetaJSON(id, root, "codex_cli_rs", "2026-10-08T12:00:00Z"),
		testjsonl.CodexMsgJSON("user", "wait for the script", "2026-10-08T12:00:01Z"),
		testjsonl.CodexFunctionCallWithCallIDJSON("wait", "call_wait", map[string]any{
			"cell_id": "6", "yield_time_ms": 30000, "max_tokens": 5000,
		}, "2026-10-08T12:00:02Z"),
		testjsonl.CodexFunctionCallOutputJSON("call_wait", jsontext.Value(output), "2026-10-08T12:00:03Z"),
	)
	dbtest.WriteTestFile(t, source, []byte(body))
	cfg := sync.EngineConfig{Machine: "local", AgentDirs: map[parser.AgentType][]string{parser.AgentCodex: {root}}}
	engine := sync.NewEngine(ctx, database, cfg)
	require.Equal(t, 1, engine.SyncAll(ctx, nil).Synced)
	engine.Close()

	// Simulate the previous parser, which dropped code-mode wait results.
	require.NoError(t, database.Update(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, fmt.Sprintf(
			"DELETE FROM tool_result_events; UPDATE tool_calls SET result_content='', result_content_length=0; UPDATE sessions SET data_version=%d; PRAGMA user_version=%d",
			version-1, version-1,
		))
		return err
	}))
	path := database.Path()
	require.NoError(t, database.Close())
	database, err := db.OpenIsolated(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	engine = sync.NewEngine(ctx, database, cfg)
	t.Cleanup(engine.Close)
	require.Equal(t, 1, engine.SyncAll(ctx, nil).Synced)

	messages, err := database.GetAllMessages(ctx, sessionID)
	require.NoError(t, err)
	var resultContent string
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			if call.ToolUseID == "call_wait" {
				resultContent = call.ResultContent
			}
		}
	}
	assert.Equal(t, output, resultContent)
}

func TestClaudeFormatCommandArgsUpgradeReparsesInPlace(t *testing.T) {
	providers := []parser.AgentType{parser.AgentClaude, parser.AgentCowork, parser.AgentQoder, parser.AgentIcodemate}
	scope := db.ScopedDataVersionProviders(137, 138)
	assert.ElementsMatch(t, append(providers, parser.AgentGrok, parser.AgentIflow), scope)
	version := db.CurrentDataVersion()
	t.Cleanup(db.InstallDataVersionScopesForTest(map[int][]parser.AgentType{version: scope}))
	const envelope = "<command-message>explain</command-message>\n<command-name>/explain</command-name>\n<command-args>how app.<zone> gets its cert</command-args>"
	for _, provider := range providers {
		t.Run(string(provider), func(t *testing.T) {
			database := dbtest.OpenTestDB(t)
			root := t.TempDir()
			sessionID := "c0000000-0000-4000-8000-000000000165"
			source := filepath.Join(root, "project", sessionID+".jsonl")
			if provider == parser.AgentCowork {
				workspace := filepath.Join(root, "org", "workspace")
				localID := "local_50000000-0000-4000-8000-000000000165"
				dbtest.WriteTestFile(t, filepath.Join(workspace, localID+".json"), []byte(`{"sessionId":"`+localID+`","cliSessionId":"`+sessionID+`"}`))
				source = filepath.Join(workspace, localID, ".claude", "projects", "project", sessionID+".jsonl")
			}
			body := testjsonl.NewSessionBuilder().
				AddClaudeUserWithSessionID("2026-11-04T12:00:00Z", envelope, sessionID).
				AddClaudeAssistant("2026-11-04T12:00:05Z", "It is DNS-validated.").
				String()
			dbtest.WriteTestFile(t, source, []byte(body))
			cfg := sync.EngineConfig{Machine: "local", AgentDirs: map[parser.AgentType][]string{provider: {root}}}
			engine := sync.NewEngine(t.Context(), database, cfg)
			require.Equal(t, 1, engine.SyncAll(t.Context(), nil).Synced)
			engine.Close()
			var id string
			require.NoError(t, database.Reader().QueryRow(t.Context(), "SELECT id FROM sessions").Scan(&id))
			// Model the archive the previous parser wrote: the envelope stored raw.
			require.NoError(t, database.Update(t.Context(), func(tx *sql.Tx) error {
				if _, err := tx.ExecContext(t.Context(), "UPDATE messages SET content = ? WHERE session_id = ? AND ordinal = 0", envelope, id); err != nil {
					return err
				}
				_, err := tx.ExecContext(t.Context(), fmt.Sprintf("UPDATE sessions SET data_version=%d; PRAGMA user_version=%d", version-1, version-1))
				return err
			}))
			path := database.Path()
			require.NoError(t, database.Close())
			database, err := db.OpenIsolated(t.Context(), path)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, database.Close()) })
			assert.False(t, database.NeedsResync(), "Claude upgrade must reparse in place")
			engine = sync.NewEngine(t.Context(), database, cfg)
			t.Cleanup(engine.Close)
			require.Equal(t, 1, engine.SyncAll(t.Context(), nil).Synced)
			assert.Equal(t, source, database.GetSessionFilePath(t.Context(), id))
			var content string
			require.NoError(t, database.Reader().QueryRow(t.Context(), "SELECT content FROM messages WHERE session_id = ? AND ordinal = 0", id).Scan(&content))
			assert.Equal(t, "/explain how app.<zone> gets its cert", content)
			var count int
			require.NoError(t, database.Reader().QueryRow(t.Context(), "SELECT count(*) FROM sessions").Scan(&count))
			assert.Equal(t, 1, count)
		})
	}
}
