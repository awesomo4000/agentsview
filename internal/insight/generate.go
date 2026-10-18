package insight

import (
	"bufio"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"unicode/utf16"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/harness"
	"go.kenn.io/agentsview/internal/stringutil"
)

// geminiInsightModel is the model passed to the gemini CLI
// for insight generation.
const geminiInsightModel = "gemini-3.1-pro-preview"

// claudeStdoutLogMaxBytes bounds Claude stdout log payloads sent to
// SSE/UI so a single large JSON response does not overwhelm the log
// stream. Full stdout is still parsed for result extraction.
const claudeStdoutLogMaxBytes = 16 * 1024

// Result holds the output from an AI agent invocation.
type Result struct {
	ExecutionID string
	Finish      func(int64) error
	Content     string
	Agent       string
	Model       string
}

// ValidTypes lists the insight types the generate route accepts.
var ValidTypes = map[string]bool{
	"daily_activity":      true,
	"agent_analysis":      true,
	CannedType:            true,
	ToolEffectivenessType: true,
}

// ValidAgentNames lists the supported insight agent names in display
// order. ValidAgents is a lookup set derived from it. The names live in
// internal/config so configuration validation accepts exactly this set.
var ValidAgentNames = config.InsightAgentNames()

// ValidAgents is the set of supported insight agent names.
var ValidAgents = func() map[string]bool {
	m := make(map[string]bool, len(ValidAgentNames))
	for _, name := range ValidAgentNames {
		m[name] = true
	}
	return m
}()

// GenerateFunc is the signature for insight generation,
// allowing tests to substitute a stub.
type GenerateFunc func(
	ctx context.Context, agent, prompt string,
) (Result, error)

// LogEvent represents one log line emitted by an insight
// agent process.
type LogEvent struct {
	Stream string `json:"stream"` // stdout|stderr
	Line   string `json:"line"`
}

// LogFunc receives real-time log events from the agent
// subprocess.
type LogFunc func(LogEvent)

// GenerateStreamFunc is like GenerateFunc but includes a
// log callback for streaming stdout/stderr events.
type GenerateStreamFunc func(
	ctx context.Context, agent, prompt string, onLog LogFunc,
) (Result, error)

// AgentConfig holds insight generation overrides for one agent.
type AgentConfig struct {
	Binary      string
	Sandbox     string
	AllowUnsafe bool
}

// GenerateOptions holds optional insight generation overrides.
type GenerateOptions struct {
	External   *harness.Config
	LedgerPath string
	Agents     map[string]AgentConfig
	Endpoint   *EndpointConfig
}

// GenerateStreamWithOptions invokes an AI agent CLI to generate an
// insight, using configured binary paths before falling back to PATH.
func GenerateStreamWithOptions(
	ctx context.Context, agent, prompt string, onLog LogFunc,
	opts GenerateOptions,
) (Result, error) {

	if !ValidAgents[agent] {
		return Result{}, fmt.Errorf(
			"unsupported agent: %s", agent,
		)
	}
	if agent == "external" {
		if opts.External == nil {
			return Result{}, errors.New("external harness is not configured")
		}
		record, content, err := harness.Run(ctx, *opts.External, prompt, opts.LedgerPath)
		result := Result{Content: content, Agent: opts.External.ID, ExecutionID: record.ID}
		if record.ID != "" {
			result.Finish = func(id int64) error { return harness.Finish(opts.LedgerPath, record.ID, id) }
		}
		if record.Observation != nil {
			if model, ok := record.Observation.Identity["reported_model"]; ok && model.Source == "provider_response" && model.Value != nil {
				result.Model = *model.Value
			}
		}
		return result, err
	}
	return Result{}, errors.New("built-in AI runners are removed; configure the external harness")

}

// agentEnv returns the current environment with
// CLAUDE_NO_SOUND=1 appended.
//
// The full environment is passed through intentionally.
// Agent CLIs need provider auth (API keys, tokens, config
// paths) that vary across providers, users, and deployment
// methods (env vars, desktop.env, persisted login). An
// allowlist is brittle here: every new provider or config
// var requires a code change, and missing one breaks auth.
// Sandboxing is handled by CLI flags (--tools, --sandbox,
// --config-dir, etc.), not by env filtering.

func emitLog(onLog LogFunc, stream, line string) {
	if onLog == nil {
		return
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	onLog(LogEvent{
		Stream: stream,
		Line:   line,
	})
}

func truncateLogLine(line string, maxBytes int) string {
	if maxBytes <= 0 || len(line) <= maxBytes {
		return line
	}
	prefix := stringutil.SafeTruncate(line, maxBytes)
	omitted := len(line) - len(prefix)
	return fmt.Sprintf(
		"%s... [truncated %d bytes]",
		prefix, omitted,
	)
}

func collectStreamLines(
	r io.Reader, stream string, onLog LogFunc,
) <-chan string {
	ch := make(chan string, 1)
	go func() {
		defer close(ch)
		br := bufio.NewReader(r)
		var lines []string
		for {
			line, err := br.ReadString('\n')
			trimmed := strings.TrimRight(line, "\r\n")
			if trimmed != "" {
				lines = append(lines, trimmed)
				emitLog(onLog, stream, trimmed)
			}
			if err == nil {
				continue
			}
			if err == io.EOF {
				break
			}
			emitLog(
				onLog, "stderr",
				fmt.Sprintf("read %s: %v", stream, err),
			)
			_, _ = io.Copy(io.Discard, br)
			break
		}
		ch <- strings.Join(lines, "\n")
	}()
	return ch
}

// generateClaude invokes `claude -p --output-format json`.

// parseCLIResult extracts the result text and model from
// claude CLI output. Claude Code (v2+) outputs a JSON array
// of events; we find type="result" and read its result field.
// Falls back to the legacy single-object format for older
// versions: {"result":"...","model":"..."}.
func parseCLIResult(data []byte) (result, model string) {
	// Try JSON array format (Claude Code v2+).
	var events []jsontext.Value
	if json.Unmarshal(data, &events) == nil {
		for _, raw := range events {
			var ev struct {
				Type       string                    `json:"type"`
				Result     string                    `json:"result"`
				ModelUsage map[string]jsontext.Value `json:"modelUsage"`
			}
			if json.Unmarshal(raw, &ev) != nil {
				continue
			}
			if ev.Type == "result" &&
				strings.TrimSpace(ev.Result) != "" {
				if len(ev.ModelUsage) > 0 {
					keys := make([]string, 0, len(ev.ModelUsage))
					for k := range ev.ModelUsage {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					model = keys[0]
				}
				return ev.Result, model
			}
		}
	}
	// Fall back to legacy single-object format.
	var resp struct {
		Result string `json:"result"`
		Model  string `json:"model"`
	}
	if json.Unmarshal(data, &resp) == nil {
		return resp.Result, resp.Model
	}
	return "", ""
}

// generateCodex invokes `codex exec` in read-only sandbox
// and parses the JSONL stream for agent_message items.

// codexEvent represents a JSONL event from codex --json.
type codexEvent struct {
	Type  string `json:"type"`
	Error struct {
		Message string `json:"message,omitempty"`
	} `json:"error,omitempty"`
	Item struct {
		ID   string `json:"id,omitempty"`
		Type string `json:"type,omitempty"`
		Text string `json:"text,omitempty"`
	} `json:"item,omitempty"`
}

// parseCodexStream reads codex JSONL and extracts
// agent_message text from item.completed/item.updated events.
func parseCodexStream(
	r io.Reader, onLog LogFunc,
) (string, error) {
	br := bufio.NewReader(r)
	messages := make([]string, 0)
	indexByID := make(map[string]int)

	for {
		line, err := br.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("read stream: %w", err)
		}

		trimmed := strings.TrimSpace(line)
		emitLog(onLog, "stdout", trimmed)
		if trimmed != "" {
			var ev codexEvent
			if json.Unmarshal(
				[]byte(trimmed), &ev,
			) == nil {
				if ev.Type == "turn.failed" ||
					ev.Type == "error" {
					msg := ev.Error.Message
					if msg == "" {
						msg = "codex stream error"
					}
					return "", fmt.Errorf(
						"codex: %s", msg,
					)
				}

				isMsg := (ev.Type == "item.completed" ||
					ev.Type == "item.updated") &&
					ev.Item.Type == "agent_message" &&
					ev.Item.Text != ""
				if isMsg {
					if ev.Item.ID == "" {
						messages = append(
							messages, ev.Item.Text,
						)
					} else if idx, ok := indexByID[ev.Item.ID]; ok {
						messages[idx] = ev.Item.Text
					} else {
						indexByID[ev.Item.ID] = len(messages)
						messages = append(
							messages, ev.Item.Text,
						)
					}
				}
			}
		}

		if err == io.EOF {
			break
		}
	}

	return strings.Join(messages, "\n"), nil
}

// PromptArgLimit returns the largest prompt, as measured by promptArgSize,
// that agent can receive, or zero when the prompt goes through stdin or an
// HTTP body. Copilot's CLI takes the prompt as one argument, which Linux caps
// at 128 KiB and Windows caps, for the whole command line, at 32,767
// characters. macOS has no per-argument cap, only 1 MiB for all arguments and
// the environment together, so its limit leaves 128 KiB for the rest.
func PromptArgLimit(agent string, opts GenerateOptions) int {
	if agent != "copilot" || opts.Endpoint != nil {
		return 0
	}
	switch runtime.GOOS {
	case "windows":
		return 30 << 10
	case "darwin":
		return 896 << 10
	default:
		return 120 << 10
	}
}

// promptArgSize measures a prompt the way the OS limits it as an argument.
var promptArgSize = func(prompt string) int {
	if runtime.GOOS == "windows" {
		return windowsArgLength(prompt)
	}
	return len(prompt)
}

// windowsArgLength returns the UTF-16 length of s once quoted for a Windows
// command line, following syscall.EscapeArg.
func windowsArgLength(s string) int {
	if s == "" {
		return 2
	}
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	hasSpace := strings.ContainsAny(s, " \t")
	if hasSpace {
		n += 2
	}
	if !strings.ContainsAny(s, `"\`) {
		return n
	}
	slashes := 0
	for i := range len(s) {
		switch s[i] {
		case '\\':
			slashes++
			continue
		case '"':
			n += slashes + 1
		}
		slashes = 0
	}
	if hasSpace {
		n += slashes
	}
	return n
}

// generateCopilot invokes `copilot -p <prompt> --silent`.
// The prompt is passed as the -p argument (copilot does not
// read prompts from stdin). Output is plain text on stdout.

// generateGemini invokes `gemini --output-format stream-json`
// and parses the JSONL stream for result/assistant messages.

// streamMessage represents a JSONL event from stream-json
// output (shared format between Claude and Gemini CLIs).
type streamMessage struct {
	Type    string `json:"type"`
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
	Message struct {
		Content string `json:"content,omitempty"`
	} `json:"message,omitempty"`
	Result string `json:"result,omitempty"`
	Error  struct {
		Message string `json:"message,omitempty"`
	} `json:"error,omitempty"`
}

// parseStreamJSON reads stream-json JSONL and extracts the
// result text. Prefers type=result, falls back to collecting
// assistant messages.
func parseStreamJSON(
	r io.Reader, onLog LogFunc,
) (string, error) {
	br := bufio.NewReader(r)
	var lastResult string
	var assistantMsgs []string

	for {
		line, err := br.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("read stream: %w", err)
		}

		trimmed := strings.TrimSpace(line)
		emitLog(onLog, "stdout", trimmed)
		if trimmed != "" {
			var msg streamMessage
			if json.Unmarshal(
				[]byte(trimmed), &msg,
			) != nil {
				continue
			}
			if msg.Type == "error" {
				m := msg.Error.Message
				if m == "" {
					m = "stream error"
				}
				return "", fmt.Errorf(
					"stream: %s", m,
				)
			}
			if msg.Type == "message" &&
				msg.Role == "assistant" &&
				msg.Content != "" {
				assistantMsgs = append(
					assistantMsgs, msg.Content,
				)
			}
			if msg.Type == "assistant" &&
				msg.Message.Content != "" {
				assistantMsgs = append(
					assistantMsgs,
					msg.Message.Content,
				)
			}
			if msg.Type == "result" &&
				msg.Result != "" {
				lastResult = msg.Result
			}
		}

		if err == io.EOF {
			break
		}
	}

	if lastResult != "" {
		return lastResult, nil
	}
	if len(assistantMsgs) > 0 {
		return strings.Join(assistantMsgs, "\n"), nil
	}
	return "", nil
}

// agentBinary maps an agent name to its CLI binary name.
func agentBinary(agent string) string {
	if agent == "kiro" {
		return "kiro-cli"
	}
	return agent
}

// ansiRE matches ANSI escape sequences.
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

// generateKiro invokes `kiro-cli chat --no-interactive` with
// the prompt on stdin and strips ANSI escape codes from output.
//
// Note: kiro-cli does not have a hard no-tools/read-only mode
// like Claude's --tools "" or Codex's --sandbox read-only.
// --trust-tools= disables tool trust prompts but does not
// prevent tool execution. Use only with trusted session data.
// The working directory is set to os.TempDir() to limit
// exposure if tools are triggered.
