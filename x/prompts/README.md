# Extracted prompts

Snapshot from commit `a1d005bc0e3389c763a361f8a2ce644860a95d7c`, extracted on
10/17/2026. These are review copies. The running app still uses its original Go
prompt builders; it does not load this directory. No generation paths or model
clients were removed by this extraction.

## Prompt inventory

| Files | Purpose and representation |
| --- | --- |
| [daily-activity-summary.txt](daily-activity-summary.txt) | Exact instruction to summarize accomplishments, decisions, and patterns by project. |
| [agent-analysis.txt](agent-analysis.txt) | Exact instruction for session effectiveness and workflow improvements. |
| [recall-intent.txt](recall-intent.txt) | Exact user-intent extraction instruction: tasks, constraints, preferences, corrections, and open questions. |
| [recall-action.txt](recall-action.txt) | Exact action/finding extraction instruction: procedures, changes, findings, decisions, warnings, and outcomes. |
| [recall-generic.txt](recall-generic.txt) | Exact generic-role instruction, currently identical to the action prompt. |
| [recall-unexecuted-action.txt](recall-unexecuted-action.txt) | Exact preamble for segments with no executed tool calls. Prevents recording planned work as completed. |
| [tool-effectiveness.txt](tool-effectiveness.txt) | Exact instruction and JSON output shape for evidence-cited tool assessments. |
| [tool-effectiveness-correction.txt](tool-effectiveness-correction.txt) | Exact correction assembly with explicit placeholders for the original prompt and validation error. |
| `review-*.txt` | All six review templates rendered by the real builder using empty synthetic aggregates. Include common instructions, template focus, output schema, evidence rules, and the empty payload. |

Review templates:

- [Prompt maturity](review-prompt_maturity_review.txt)
- [Context setup](review-context_setup_review.txt)
- [Workflow hygiene](review-workflow_hygiene_review.txt)
- [Tool reliability](review-tool_reliability_review.txt)
- [Model and cost](review-model_cost_review.txt)
- [Instruction opportunities](review-instruction_opportunity_review.txt)

The review examples use `SYNTHETIC-EMPTY-AGGREGATE` as a marker, not a real
aggregate hash. Their empty values are fixture data, not findings. The context
setup example exercises the no-context-data branch. Both branches remain in the
source snapshot and literal-fragment inventory.

## Assembly, schemas, and provenance

These instructions are not always complete requests. The builders append
selected archive evidence, metadata, user focus, and output constraints:

- Daily/range requests include at most 50 sessions, session metadata, and each
  first message truncated to 200 runes. They do not contain complete transcripts.
- Single-session analysis includes non-system messages, each preview limited to
  800 runes, plus timing and usage metadata.
- Tool-effectiveness requests include budgeted tool evidence, message/call
  references, and omission markers. The reply is validated against that evidence.
- Review templates include computed aggregates and, for some templates,
  prompt samples and workflow examples. They are not necessarily content-free.
- Recall separates intent and action units. The instruction is the system
  message and the transcript unit is the user message. The HTTP implementation
  currently supplies a separate JSON schema. A binary adapter must preserve
  that output contract rather than sending the instruction alone.

[recall-response-schema.go.txt](recall-response-schema.go.txt) preserves the
schema builder and its bounds/types. Its prompt asks for titles up to 120
characters, while the schema accepts up to 500. That existing distinction is
preserved, not silently corrected.

[source/](source/) contains exact source snapshots of the four prompt-builder
files, including dynamic assembly, conditional instructions, template metadata,
profiles, and validators that live in those files. They have `.go.txt` suffixes
so they do not become compiled packages. [manifest.json](manifest.json) records
source hashes and the source commit. [literal-fragments.json](literal-fragments.json)
records string constants and statically evaluable `WriteString` arguments with
source lines. It is an audit aid, not an executable template: dynamic arguments,
format strings, and conditions require the source snapshots.

The recall profile registry currently has `base` and `qwen` entries. Qwen uses
the same prompt wording with different request settings. Local override files
`intent.txt`, `action.txt`, and `generic.txt` can replace the corresponding
built-in prompts; no user override files were read for this extraction.

The prompt builders, evidence IDs, schemas, provenance, and validators are useful
for an internal summarizer independent of how the model is called. Project or
workstream classification still needs an additional prompt or adaptation.

## External harness execution

See [HARNESS.md](HARNESS.md) for the current adapter behavior and a synthetic
round-trip demonstration.

## Reproducing the text extraction

`extract.go.txt` uses the Go AST to recover exact string wording and calls the
real review/recall builders with synthetic inputs. Run from the repository root:

```sh
scratch_dir="$(mktemp -d "$PWD/.prompt-extract.XXXXXX")"
cp x/prompts/extract.go.txt "$scratch_dir/main.go"
go run -tags fts5 "$scratch_dir/main.go"
rm "$scratch_dir/main.go"
rmdir "$scratch_dir"
```

This regenerates the `.txt` prompts and `literal-fragments.json`; it does not
refresh source snapshots or their manifest. Refresh those together when taking a
new snapshot. Review diffs before replacing this historical copy. The command
invokes no agent or model, though Go may fetch missing build dependencies.
