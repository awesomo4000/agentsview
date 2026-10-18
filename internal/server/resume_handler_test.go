package server_test

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

type failingResumeModelCountsStore struct {
	readOnlyTestStore
}

func (failingResumeModelCountsStore) GetResumeModelCounts(
	context.Context, string,
) ([]db.ModelCount, error) {
	return nil, errors.New("boom")
}

type resumeCountsOnlyStore struct {
	readOnlyTestStore
}

func (resumeCountsOnlyStore) GetAllMessages(
	context.Context, string,
) ([]db.Message, error) {
	return nil, errors.New("unexpected GetAllMessages call")
}

func canonicalTestPath(path string) string {
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		clean = filepath.Clean(resolved)
	}
	if runtime.GOOS == "darwin" && strings.HasPrefix(clean, "/private/") {
		publicPath := filepath.Clean(strings.TrimPrefix(clean, "/private"))
		if info, err := os.Stat(publicPath); err == nil && info.IsDir() {
			return publicPath
		}
	}
	return clean
}

func assertSamePath(t *testing.T, label, got, want string) {
	t.Helper()
	got = canonicalTestPath(got)
	want = canonicalTestPath(want)
	if got == want {
		return
	}
	gotInfo, gotErr := os.Stat(got)
	wantInfo, wantErr := os.Stat(want)
	if gotErr == nil && wantErr == nil && os.SameFile(gotInfo, wantInfo) {
		return
	}
	assert.Fail(t, "path mismatch", "%s = %q, want %q", label, got, want)
}

func messagePointPromptGlob(t *testing.T, sessionID string, ordinal int) string {
	t.Helper()
	cacheDir, err := os.UserCacheDir()
	require.NoError(t, err)
	return filepath.Join(
		cacheDir,
		"agentsview",
		"claude-message-points",
		fmt.Sprintf("%s-ordinal-%d-*.txt", sessionID, ordinal),
	)
}

func removeMessagePointPrompts(t *testing.T, sessionID string, ordinal int) {
	t.Helper()
	matches, err := filepath.Glob(
		messagePointPromptGlob(t, sessionID, ordinal),
	)
	require.NoError(t, err)
	for _, match := range matches {
		_ = os.Remove(match)
	}
}

func findSingleMessagePointPrompt(
	t *testing.T, sessionID string, ordinal int,
) string {
	t.Helper()
	matches, err := filepath.Glob(
		messagePointPromptGlob(t, sessionID, ordinal),
	)
	require.NoError(t, err)
	require.Len(t, matches, 1)
	return matches[0]
}

func assertNoMessagePointPrompts(
	t *testing.T, sessionID string, ordinal int,
) {
	t.Helper()
	matches, err := filepath.Glob(
		messagePointPromptGlob(t, sessionID, ordinal),
	)
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func assertMessagePointCommandForRuntime(
	t *testing.T, command string, promptPath string,
) {
	t.Helper()

	if runtime.GOOS == "windows" {
		script := decodeMessagePointPowerShellCommandForTest(t, command)
		quotedPromptPath := powerShellSingleQuoteForTest(promptPath)
		assert.Contains(t, script,
			"Get-Content -Raw -Encoding UTF8 -LiteralPath "+
				quotedPromptPath)
		assert.Contains(t, script,
			"Remove-Item -LiteralPath "+quotedPromptPath+
				" -Force -ErrorAction SilentlyContinue")
		assert.NotContains(t, command, " < ")
		assert.NotContains(t, command, "rm -f --")
		assert.NotContains(t, script, " < ")
		assert.NotContains(t, script, "rm -f --")
		return
	}
	assert.Contains(t, command, "claude <")
	assert.Contains(t, command, "rm -f --")
}

func decodeMessagePointPowerShellCommandForTest(
	t *testing.T, command string,
) string {
	t.Helper()

	const prefix = "powershell.exe -NoProfile -EncodedCommand "
	require.True(t, strings.HasPrefix(command, prefix), "command = %q", command)
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(command, prefix))
	require.NoError(t, err)
	require.Zero(t, len(raw)%2, "UTF-16LE byte length must be even")

	codeUnits := make([]uint16, len(raw)/2)
	for i := range codeUnits {
		codeUnits[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	return string(utf16.Decode(codeUnits))
}

func powerShellSingleQuoteForTest(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

type resumeTestResponse struct {
	Command string `json:"command"`
}

func TestGetSessionDirectory(t *testing.T) {
	te := setup(t)

	t.Run("removed_absolute_paths", func(t *testing.T) {
		type testCase struct {
			name    string
			id      string
			project string
			setup   func(*db.Session)
			want    string
		}

		removedChild := func(t *testing.T, name string) string {
			t.Helper()
			parent := t.TempDir()
			path := filepath.Join(parent, name)
			require.NoError(t, os.Mkdir(path, 0o755))
			require.NoError(t, os.Remove(path))
			return path
		}

		embeddedPath := removedChild(t, "embedded path")
		embeddedLater := t.TempDir()
		embeddedSource := filepath.Join(t.TempDir(), "session.jsonl")
		embeddedJSON, err := json.Marshal(embeddedPath)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(
			embeddedSource,
			[]byte(`{"cwd":`+string(embeddedJSON)+"}\n"),
			0o600,
		))

		cachedPath := removedChild(t, "cached path")
		cachedLater := t.TempDir()
		projectPath := removedChild(t, "project path")

		cases := []testCase{
			{
				name:    "embedded_cwd",
				id:      "dir-removed-embedded",
				project: embeddedLater,
				setup: func(s *db.Session) {
					s.FilePath = &embeddedSource
				},
				want: embeddedPath,
			},
			{
				name:    "cached_cwd",
				id:      "dir-removed-cached",
				project: cachedLater,
				setup: func(s *db.Session) {
					s.Cwd = cachedPath
				},
				want: cachedPath,
			},
			{
				name:    "project",
				id:      "dir-removed-project",
				project: projectPath,
				want:    projectPath,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				var opts []func(*db.Session)
				if tc.setup != nil {
					opts = append(opts, tc.setup)
				}
				te.seedSession(t, tc.id, tc.project, 1, opts...)
				w := te.get(t, "/api/v1/sessions/"+tc.id+"/directory")
				assertStatus(t, w, http.StatusOK)
				var resp struct {
					Path string `json:"path"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				t.Logf("status=%d path=%q want=%q", w.Code, resp.Path, tc.want)
				assert.Equal(t, tc.want, resp.Path)
			})
		}
	})

	t.Run("candidate_order", func(t *testing.T) {
		embeddedDir := t.TempDir()
		cachedDir := t.TempDir()
		projectDir := t.TempDir()
		sessionFile := filepath.Join(t.TempDir(), "session.jsonl")
		embeddedJSON, err := json.Marshal(embeddedDir)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(
			sessionFile,
			[]byte(`{"cwd":`+string(embeddedJSON)+"}\n"),
			0o600,
		))
		te.seedSession(t, "dir-order-embedded", projectDir, 1, func(s *db.Session) {
			s.Cwd = cachedDir
			s.FilePath = &sessionFile
		})
		w := te.get(t, "/api/v1/sessions/dir-order-embedded/directory")
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Path string `json:"path"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		t.Logf("embedded precedence status=%d path=%q", w.Code, resp.Path)
		assert.Equal(t, embeddedDir, resp.Path)

		cursorWorkspace := filepath.Join(t.TempDir(), "workspace-root", "cursor-project")
		cursorFallback := t.TempDir()
		removedCursorCwd := filepath.Join(t.TempDir(), "removed-cwd")
		require.NoError(t, os.Mkdir(removedCursorCwd, 0o755))
		require.NoError(t, os.Remove(removedCursorCwd))
		cursorClean := filepath.Clean(cursorWorkspace)
		cursorTokens := []string{}
		if volume := filepath.VolumeName(cursorClean); volume != "" {
			cursorTokens = append(cursorTokens, strings.TrimSuffix(volume, ":"))
			cursorClean = strings.TrimPrefix(cursorClean, volume)
		}
		for part := range strings.SplitSeq(cursorClean, string(filepath.Separator)) {
			if part == "" {
				continue
			}
			cursorTokens = append(cursorTokens, strings.FieldsFunc(part, func(r rune) bool {
				return r == '-' || r == '.' || r == '_'
			})...)
		}
		cursorEncoded := strings.Join(cursorTokens, "-")
		cursorTranscript := filepath.Join(
			t.TempDir(), ".cursor", "projects",
			cursorEncoded,
			"agent-transcripts", "cursor-order", "cursor-order.jsonl",
		)
		require.NoError(t, os.MkdirAll(filepath.Dir(cursorTranscript), 0o755))
		require.NoError(t, os.MkdirAll(cursorWorkspace, 0o755))
		require.NoError(t, os.WriteFile(cursorTranscript, []byte("{}\n"), 0o600))
		te.seedSession(t, "dir-order-cursor", cursorFallback, 1, func(s *db.Session) {
			s.Agent = "cursor"
			s.Cwd = removedCursorCwd
			s.FilePath = &cursorTranscript
		})
		w = te.get(t, "/api/v1/sessions/dir-order-cursor/directory")
		assertStatus(t, w, http.StatusOK)
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		t.Logf("cursor precedence status=%d path=%q", w.Code, resp.Path)
		assert.Equal(t, removedCursorCwd, resp.Path)

		te.seedSession(t, "dir-order-cursor-workspace", cursorFallback, 1, func(s *db.Session) {
			s.Agent = "cursor"
			s.FilePath = &cursorTranscript
		})
		w = te.get(t, "/api/v1/sessions/dir-order-cursor-workspace/directory")
		assertStatus(t, w, http.StatusOK)
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		t.Logf("cursor workspace status=%d path=%q", w.Code, resp.Path)
		assertSamePath(t, "path", resp.Path, cursorWorkspace)
	})

	t.Run("relative_metadata_falls_back", func(t *testing.T) {
		projectDir := t.TempDir()
		relativeSource := filepath.Join(t.TempDir(), "relative.jsonl")
		relativeJSON, err := json.Marshal(filepath.Join("relative", "cwd"))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(
			relativeSource,
			[]byte(`{"cwd":`+string(relativeJSON)+"}\n"),
			0o600,
		))

		cases := []struct {
			name  string
			id    string
			setup func(*db.Session)
		}{
			{
				name: "relative_embedded_cwd",
				id:   "dir-relative-embedded",
				setup: func(s *db.Session) {
					s.FilePath = &relativeSource
				},
			},
			{
				name: "nested_relative_cached_cwd",
				id:   "dir-relative-cached",
				setup: func(s *db.Session) {
					s.Cwd = filepath.Join("nested", "relative")
				},
			},
			{
				name: "empty_metadata",
				id:   "dir-relative-empty",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				var opts []func(*db.Session)
				if tc.setup != nil {
					opts = append(opts, tc.setup)
				}
				te.seedSession(t, tc.id, projectDir, 1, opts...)
				w := te.get(t, "/api/v1/sessions/"+tc.id+"/directory")
				assertStatus(t, w, http.StatusOK)
				var resp struct {
					Path string `json:"path"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				t.Logf("status=%d path=%q want=%q", w.Code, resp.Path, projectDir)
				assert.Equal(t, projectDir, resp.Path)
			})
		}

		te.seedSession(t, "dir-relative-label", "project-label", 1, func(s *db.Session) {
			s.Cwd = "relative/cwd"
		})
		w := te.get(t, "/api/v1/sessions/dir-relative-label/directory")
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Path string `json:"path"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		t.Logf("label fallback status=%d path=%q", w.Code, resp.Path)
		assert.Empty(t, resp.Path)
	})

	t.Run("absolute_file_path", func(t *testing.T) {
		recordedFile := filepath.Join(t.TempDir(), "recorded.txt")
		require.NoError(t, os.WriteFile(recordedFile, []byte("synthetic"), 0o600))
		projectDir := t.TempDir()
		te.seedSession(t, "dir-regular-file", projectDir, 1, func(s *db.Session) {
			s.Cwd = recordedFile
		})
		w := te.get(t, "/api/v1/sessions/dir-regular-file/directory")
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Path string `json:"path"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		t.Logf("status=%d path=%q want=%q", w.Code, resp.Path, recordedFile)
		assert.Equal(t, recordedFile, resp.Path)
	})

	t.Run("stored_cwd_fallback", func(t *testing.T) {
		cachedDir := t.TempDir()
		missingSource := filepath.Join(t.TempDir(), "missing.jsonl")
		virtualPath := filepath.Join(t.TempDir(), "data.sqlite3") + "#sqlite-session"

		hashRoot := filepath.Join(t.TempDir(), "project#dev")
		hashCwd := filepath.Join(hashRoot, "workspace")
		hashSource := filepath.Join(hashRoot, "session.jsonl")
		require.NoError(t, os.MkdirAll(hashCwd, 0o755))
		hashJSON, err := json.Marshal(hashCwd)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(
			hashSource,
			[]byte(`{"cwd":`+string(hashJSON)+"}\n"),
			0o600,
		))

		cases := []struct {
			name          string
			id            string
			setup         func(*db.Session)
			sourceMissing bool
			want          string
		}{
			{
				name:          "source_missing",
				id:            "dir-cached-source-missing",
				sourceMissing: true,
				setup: func(s *db.Session) {
					s.Cwd = cachedDir
					s.FilePath = &missingSource
				},
				want: cachedDir,
			},
			{
				name: "missing_source",
				id:   "dir-cached-missing-source",
				setup: func(s *db.Session) {
					s.Cwd = cachedDir
					s.FilePath = &missingSource
				},
				want: cachedDir,
			},
			{
				name: "virtual_source",
				id:   "dir-cached-virtual-source",
				setup: func(s *db.Session) {
					s.Cwd = cachedDir
					s.FilePath = &virtualPath
				},
				want: cachedDir,
			},
			{
				name: "real_hash_filename",
				id:   "dir-hash-source",
				setup: func(s *db.Session) {
					s.Cwd = t.TempDir()
					s.FilePath = &hashSource
				},
				want: hashCwd,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				te.seedSession(t, tc.id, t.TempDir(), 1, tc.setup)
				if tc.sourceMissing {
					require.NoError(t, te.db.BaselineActiveSessionSourcePaths(
						t.Context(), "test", []db.SessionSourcePath{{
							Agent: "claude", FilePath: missingSource,
						}},
					))
					changed, err := te.db.MarkSessionSourceMissing(
						t.Context(), "test", "claude", tc.id, missingSource,
					)
					require.NoError(t, err)
					assert.True(t, changed)
				}
				before, err := te.db.GetSessionFull(t.Context(), tc.id)
				require.NoError(t, err)
				require.NotNil(t, before)

				w := te.get(t, "/api/v1/sessions/"+tc.id+"/directory")
				assertStatus(t, w, http.StatusOK)
				var resp struct {
					Path string `json:"path"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				t.Logf("status=%d path=%q want=%q", w.Code, resp.Path, tc.want)
				assert.Equal(t, tc.want, resp.Path)

				after, err := te.db.GetSessionFull(t.Context(), tc.id)
				require.NoError(t, err)
				require.NotNil(t, after)
				assert.Equal(t, before.Cwd, after.Cwd)
				assert.Equal(t, before.Project, after.Project)
				assert.Equal(t, before.FilePath, after.FilePath)
				assert.Equal(t, before.SourceMissingAt, after.SourceMissingAt)
				assert.Equal(t, before.DeletedAt, after.DeletedAt)
			})
		}
	})

	t.Run("not found", func(t *testing.T) {
		w := te.get(t, "/api/v1/sessions/nonexistent/directory")
		assertStatus(t, w, http.StatusNotFound)
	})

	t.Run("cursor directory returns workspace root", func(t *testing.T) {
		projectDir := t.TempDir()
		runDir := filepath.Join(projectDir, "frontend")
		require.NoError(t, os.MkdirAll(runDir, 0o755))
		runDirJSON, _ := json.Marshal(runDir)
		sessionFile := filepath.Join(t.TempDir(), "cursor.jsonl")
		content := `{"role":"assistant","message":{"content":[{"type":"tool_use","name":"Shell","input":{"command":"pwd","working_directory":` +
			string(runDirJSON) + `}}]}}` + "\n"
		require.NoError(t, os.WriteFile(sessionFile, []byte(content), 0o644))
		te.seedSession(t, "dir-cursor", projectDir, 3, func(s *db.Session) {
			s.Agent = "cursor"
			s.FilePath = &sessionFile
		})

		w := te.get(t, "/api/v1/sessions/dir-cursor/directory")
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Path string `json:"path"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assertSamePath(t, "path", resp.Path, projectDir)
	})

	t.Run("lookup_boundaries", func(t *testing.T) {
		te.seedSession(t, "dir-trashed", t.TempDir(), 1)
		require.NoError(t, te.db.SoftDeleteSession(t.Context(), "dir-trashed"))
		w := te.get(t, "/api/v1/sessions/dir-trashed/directory")
		t.Logf("trashed status=%d body=%s", w.Code, w.Body.String())
		assertStatus(t, w, http.StatusNotFound)
		assert.Contains(t, w.Body.String(), "session not found")

		readOnly := setupPGMode(t)
		readOnly.seedSession(t, "dir-read-only", t.TempDir(), 1)
		w = readOnly.get(t, "/api/v1/sessions/dir-read-only/directory")
		t.Logf("read-only status=%d body=%s", w.Code, w.Body.String())
		assertStatus(t, w, http.StatusNotImplemented)
		assert.Contains(t, w.Body.String(), "not available in remote mode")
	})
}

func TestListOpeners(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/openers")
	assertStatus(t, w, http.StatusOK)

	var resp struct {
		Openers []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Kind string `json:"kind"`
			Bin  string `json:"bin"`
		} `json:"openers"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	// The response should always be an array (possibly empty),
	// never null.
	assert.NotNil(t, resp.Openers, "openers should be [] not null")
}

func TestGetTerminalConfig(t *testing.T) {
	te := setup(t)

	t.Run("default config", func(t *testing.T) {
		w := te.get(t, "/api/v1/config/terminal")
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Mode string `json:"mode"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "auto", resp.Mode)
	})

	t.Run("set and get", func(t *testing.T) {
		w := te.post(t,
			"/api/v1/config/terminal",
			`{"mode":"clipboard"}`,
		)
		assertStatus(t, w, http.StatusOK)

		w = te.get(t, "/api/v1/config/terminal")
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Mode string `json:"mode"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "clipboard", resp.Mode)
	})

	t.Run("invalid mode", func(t *testing.T) {
		w := te.post(t,
			"/api/v1/config/terminal",
			`{"mode":"invalid"}`,
		)
		assertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("custom requires bin", func(t *testing.T) {
		w := te.post(t,
			"/api/v1/config/terminal",
			`{"mode":"custom","custom_bin":""}`,
		)
		assertStatus(t, w, http.StatusBadRequest)
	})
}
