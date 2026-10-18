# How external harness calls work

This describes the fork at the source commit in [README.md](README.md), not a
claim about the latest behavior of independently installed CLIs.

## Selection and execution

1. A generation request chooses an agent, or `[insights].default_agent` supplies
   it. If neither specifies one, the current fallback is `claude`.
2. A configured `[insights]` endpoint and model currently select direct HTTP
   generation before binary resolution. That client remains present for now.
3. Otherwise `[agent.<name>].binary` selects the executable. If absent, the app
   searches `PATH`. Kiro resolves to `kiro-cli`; other names match their agent.
4. The selected Go adapter builds a fixed argument list and calls
   `exec.CommandContext`. It does not construct a shell command.
5. It passes the assembled prompt, reads stdout and stderr, parses the result,
   and returns content plus agent/model metadata. Server generation streams
   status/log events and stores successful insights in SQLite. Structured reviews
   and tool assessments have additional output/evidence validation.

Normal insight requests have a ten-minute server generation timeout; canned
reviews have three minutes. Cancellation reaches the subprocess through the
command context. This is not a general descendant-process containment mechanism.

## Existing adapters

`<binary>` below is the configured executable or PATH resolution. These are
argument descriptions, not commands to run against private data.

| Adapter | Arguments after executable | Prompt input | Result handling |
| --- | --- | --- | --- |
| Claude | `-p --output-format json --no-session-persistence --tools ""` | stdin | JSON event array with a `result` event, or legacy object with `result` and `model`. |
| Codex | `exec --json --sandbox read-only --skip-git-repo-check --ephemeral -` | stdin | JSONL `agent_message` items from completed/updated item events. |
| Copilot | `-p <prompt> --silent --no-custom-instructions --no-ask-user --disable-builtin-mcps` | command-line argument | Plain stdout text. |
| Gemini | `--model gemini-3.1-pro-preview --output-format stream-json` | stdin | JSONL result or assistant messages. |
| Kiro | `chat --no-interactive --trust-tools= --wrap never` | stdin | Plain stdout with ANSI escapes and selected banners stripped. |

The adapter sets `CLAUDE_NO_SOUND=1` for every harness and inherits the rest of
the app environment. The binary can therefore use inherited credentials,
configuration variables, and its own persisted login. Gemini requires a
configured sandbox provider or an explicit unsafe opt-in, and receives
`GEMINI_SANDBOX` when configured. Kiro runs in the system temporary directory;
other adapters inherit the app working directory.

Important limits of the current behavior:

- A binary override is not a generic stdin/stdout interface. It must implement
  the chosen adapter's arguments and output protocol. A wrapper can translate
  those to an internal harness, or we can add a dedicated adapter later.
- Read-only/tool flags are harness-specific. They do not prohibit the binary
  from making network requests. Kiro's tool-trust flag does not prevent tool
  execution. There is no app-wide network sandbox here.
- Copilot receives prompt text in its process arguments, subject to platform
  visibility and argument-length limits. Other adapters use stdin.
- Stdout/stderr may contain content, and log events can be sent to the UI.
  Claude's stdout log is truncated to 16 KiB; this is not content redaction.
- Changing the default agent is not an allowlist: a request can select another
  supported adapter. Requiring explicitly approved binaries remains separate work.

A future explicit configuration could point the Claude-compatible adapter at
an internal wrapper:

```toml
[insights]
default_agent = "claude"

[agent.claude]
binary = "/path/to/approved-claude-compatible-wrapper"
```

That path is illustrative. No configuration was installed or changed. Leave
endpoint/model unset to select the binary path in the current implementation.

## Verified without a model

[harness-demo.go.txt](harness-demo.go.txt) creates a disposable fake executable
under `/tmp`, calls the production `GenerateStreamWithOptions` entry point, and
checks that the prompt arrived unchanged and the synthetic result was parsed.
The fake executable only records stdin/arguments and emits fixed JSON. It has
no model client and performs no network operations. The app adapter still
inherits its normal environment; the fake does not inspect or print it.

Run on macOS/Linux from the repository root:

```sh
scratch_dir="$(mktemp -d "$PWD/.harness-demo.XXXXXX")"
cp x/prompts/harness-demo.go.txt "$scratch_dir/main.go"
go run -tags fts5 "$scratch_dir/main.go"
rm "$scratch_dir/main.go"
rmdir "$scratch_dir"
```

The observed output is in [harness-demo-result.json](harness-demo-result.json).
The demo verifies stdin delivery, exact synthetic content/model recovery, and
records the arguments including the empty `--tools` value. Existing adapter
flag/configuration and prompt-builder tests also passed during this extraction.
No installed agent, live archive, or model service was used.

## Source map

- [Binary selection, environment, and adapters](../../internal/insight/generate.go)
- [Default agent and per-agent configuration](../../internal/config/config.go)
- [Server endpoint-versus-binary options](../../internal/server/server.go)
- [Generation route, streaming, timeout, and persistence](../../internal/server/huma_routes_insights.go)
- [Canned generation and validation](../../internal/server/insights.go)

Recall extraction and embeddings currently use separate HTTP clients. They do
not automatically use this harness adapter. The extracted recall prompts can be
reused, but wiring their structured response contract to a binary is still work.

## Groundloop ai probe

On 10/17/2026, the real `ai --pipe -m codex/gpt-6-luna` command successfully
processed a synthetic three-fact session through the production
`insight.GenerateStreamWithOptions` entry point. No archive or private transcript
was read. Unlike the fake demonstration above, this probe did invoke the model
through the external client, as requested.

[ai-harness-probe.go.txt](ai-harness-probe.go.txt) is the standalone Go program.
It uses itself as a Claude-compatible subprocess wrapper: validates the adapter
arguments, forwards stdin to `ai`, captures its plain-text stdout, and emits a
JSON object with `result` and `model`. The existing adapter parses that object
back into an insight result. [The observed response](ai-harness-probe-result.json)
contains the synthetic summary and model label.

To repeat deliberately, from the repository root:

```sh
scratch_dir="$(mktemp -d "$PWD/.ai-probe.XXXXXX")"
cp x/prompts/ai-harness-probe.go.txt "$scratch_dir/main.go"
go run -tags fts5 "$scratch_dir/main.go" -ai /absolute/path/to/ai -model codex/gpt-6-luna
rm "$scratch_dir/main.go"
rmdir "$scratch_dir"
```

The most compact existing response envelope is:

```json
{"result":"Markdown summary or structured JSON answer as a string","model":"codex/gpt-6-luna"}
```

`result` must contain nonblank text; `model` is optional to the existing parser.
No `type` field is needed for this single-object response. Encode it with a JSON
library so quotes and newlines are escaped. For a structured review, the inner
answer is itself JSON text and receives the review's normal schema validation.
This probe checked ordinary summary generation and adapter parsing, not those
structured review validators or persistence through the full server route.

This is the existing Claude adapter's legacy envelope, not a cross-vendor
standard. The probe reports `adapter: claude` because that is the adapter we
borrowed; the actual producer is `ai` using `codex/gpt-6-luna`. A production
integration should have its own adapter identity to avoid misleading provenance.

Known adapter names remain `claude`, `codex`, `copilot`, `gemini`, and `kiro`.
Configuration can choose a default and override a binary path, but cannot add
an arbitrary name, argument list, or output parser. Therefore JSON output alone
would not make `ai` a drop-in override: it would still receive Claude's flags.
A small generic subprocess adapter with executable, argument array, stdin
prompt, stdout answer, stderr diagnostics, and exit-status checking would let
`ai` remain unchanged. That is the recommended production integration.

This probe changed no runtime configuration or generation behavior. Embedded
HTTP clients still remain until the separately planned removal.

## Accounting and deployment identity

The proposed [execution contract](../external-harness-execution-contract.md)
separates source-session usage from summarization attempts and records requested
versus reported model, backend/deployment, completion, and usage provenance.
It is a design for implementation, not behavior supported by this probe.
