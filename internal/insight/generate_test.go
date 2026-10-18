package insight

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCodexStream(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantError string
	}{
		{
			name: "agent messages",
			input: `{"type":"thread.started","thread_id":"abc"}
{"type":"turn.started"}
{"type":"item.started","item":{"id":"m1","type":"agent_message"}}
{"type":"item.updated","item":{"id":"m1","type":"agent_message","text":"partial"}}
{"type":"item.completed","item":{"id":"m1","type":"agent_message","text":"# Summary\nDone."}}
{"type":"turn.completed"}
`,
			want: `# Summary
Done.`,
		},
		{
			name: "multiple messages",
			input: `{"type":"item.completed","item":{"id":"m1","type":"agent_message","text":"First"}}
{"type":"item.completed","item":{"id":"m2","type":"agent_message","text":"Second"}}
`,
			want: `First
Second`,
		},
		{
			name: "turn failed",
			input: `{"type":"turn.started"}
{"type":"turn.failed","error":{"message":"rate limit"}}
`,
			wantError: "rate limit",
		},
		{
			name: "deduplicates by id",
			input: `{"type":"item.updated","item":{"id":"m1","type":"agent_message","text":"v1"}}
{"type":"item.updated","item":{"id":"m1","type":"agent_message","text":"v2"}}
{"type":"item.completed","item":{"id":"m1","type":"agent_message","text":"v3"}}
`,
			want: "v3",
		},
		{
			name: "skips malformed json",
			input: `not valid json
{"type":"item.completed","item":{"id":"m1","type":"agent_message","text":"OK"}}
`,
			want: "OK",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseCodexStream(strings.NewReader(tt.input), nil)
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestParseStreamJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantError string
	}{
		{
			name: "result event",
			input: `{"type":"system","subtype":"init"}
{"type":"assistant","message":{"content":"Working..."}}
{"type":"result","result":"# Final Summary"}
`,
			want: "# Final Summary",
		},
		{
			name: "falls back to assistant messages",
			input: `{"type":"assistant","message":{"content":"Part 1"}}
{"type":"assistant","message":{"content":"Part 2"}}
`,
			want: `Part 1
Part 2`,
		},
		{
			name: "gemini format",
			input: `{"type":"system","subtype":"init"}
{"type":"message","role":"assistant","content":"Analysis done.","delta":true}
{"type":"result","result":"# Full Result"}
`,
			want: "# Full Result",
		},
		{
			name: "error event",
			input: `{"type":"system","subtype":"init"}
{"type":"error","error":{"message":"rate limited"}}
`,
			wantError: "rate limited",
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseStreamJSON(strings.NewReader(tt.input), nil)
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestCollectStreamLines_LargeLine(t *testing.T) {
	longLine := strings.Repeat("x", 3*1024*1024)
	input := longLine + "\nsmall-line\n"
	var got []LogEvent

	done := collectStreamLines(
		strings.NewReader(input), "stderr",
		func(ev LogEvent) {
			got = append(got, ev)
		},
	)
	text := <-done

	require.Len(t, got, 2)
	assert.Equal(t, "stderr", got[0].Stream)
	assert.Len(t, got[0].Line, len(longLine))
	assert.Equal(t, "small-line", got[1].Line)
	assert.Contains(t, text, "small-line")
}

func TestValidAgents(t *testing.T) {
	for _, agent := range []string{
		"claude", "codex", "copilot", "gemini", "kiro",
	} {
		assert.True(t, ValidAgents[agent], "%s should be valid", agent)
	}
	assert.False(t, ValidAgents["gpt"], "gpt should not be valid")
}

// fakeClaudeBin writes a script that prints the given stdout
// and exits with the given code, ignoring all flags. Uses a
// .cmd batch file on Windows and a shell script elsewhere.

// fakeGeminiBin writes a script that records its argv to an
// args file, then prints stream-json output. This lets tests
// verify both the CLI flags and the parsed result.

func TestParseCLIResult(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantResult string
		wantModel  string
	}{
		{
			name:       "array format with result event",
			input:      `[{"type":"result","result":"# Summary","modelUsage":{"claude-3":{}}}]`,
			wantResult: "# Summary",
			wantModel:  "claude-3",
		},
		{
			name:       "legacy single-object format",
			input:      `{"result":"legacy result","model":"old-model"}`,
			wantResult: "legacy result",
			wantModel:  "old-model",
		},
		{
			name:       "array with no result event",
			input:      `[{"type":"system_prompt","content":"hello"},{"type":"turn","result":""}]`,
			wantResult: "",
			wantModel:  "",
		},
		{
			name:       "empty input",
			input:      ``,
			wantResult: "",
			wantModel:  "",
		},
		{
			name:       "garbage input",
			input:      `not json at all`,
			wantResult: "",
			wantModel:  "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, model := parseCLIResult([]byte(tc.input))
			assert.Equal(t, tc.wantResult, result)
			assert.Equal(t, tc.wantModel, model)
		})
	}
}

func TestPromptArgLimit_SkipsEndpointMode(t *testing.T) {
	assert.Positive(t, PromptArgLimit("copilot", GenerateOptions{}))
	assert.Zero(t, PromptArgLimit("copilot", GenerateOptions{Endpoint: &EndpointConfig{}}))
	assert.Zero(t, PromptArgLimit("claude", GenerateOptions{}))
}

func TestWindowsArgLength(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"", 2},
		{"plain", 5},
		{"two words", 11},
		{`say "hi"`, 12},
		{`x"y`, 4},
		{`a\"b`, 6},
		{`dir\ `, 7},
		{`dir\`, 4},
		{"日本 語", 6},
		{"😀", 2},
	} {
		assert.Equal(t, tc.want, windowsArgLength(tc.in), tc.in)
	}
}
