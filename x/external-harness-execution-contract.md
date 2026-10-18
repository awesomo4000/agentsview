# External harness execution and accounting

Status: implementation contract, partially implemented. See
[adapter setup and current limits](external-harness-setup.md).
Updated: 10/18/2026.
Scope: preserve deterministic session accounting, add an external-process
interface for insight generation, and distinguish model deployments.
The generic adapter and separate invocation ledger are implemented. Remaining
provenance and accounting work is listed in the setup guide.

## Two accounting domains

Original conversations retain their own usage facts, derived from source logs.
Generating a summary creates a separate execution and separate usage facts.
Never attach summarization charges to a source session's existing usage rows.
An insight may reference many source sessions without duplicating their usage.

An LLM supplies answer content only. Parsers normalize logged usage; a runner
measures process timing; pricing code calculates costs from recorded counters
and versioned internal rates. Model-generated claims about tokens, elapsed time,
provider identity, or cost are not accounting evidence.

## Existing implementation audit

| Capability | Evidence in current code | Gap or consequence |
| --- | --- | --- |
| Original message identity and token data | `internal/db/schema.sql`, `messages`: session/ordinal, model, provider_id, token_usage, context/output counters and presence flags | Preserve these. A context-window count must not be treated as billed input tokens. |
| Aggregate usage events | Same schema, `usage_events`: model/provider, input/output/cache/reasoning counts, nullable cost, cost status/source, timestamp, dedup key | Preserve deduplication and cumulative-accounting rules. These complement message facts; do not blindly add both. |
| Provider attribution | `internal/parser/types.go`, `ParsedMessage.ProviderID` and `ParsedUsageEvent.ProviderID`; populated in paths such as Cline, Crush, and Posit Assistant | Coverage varies by parser/source. A provider column does not mean every source identifies its backend. |
| Normalization | `internal/usagefacts/fact.go` | Continue deterministic normalization; preserve missingness and original counter semantics. |
| Provider-aware pricing | `internal/db/usage_rollup_pricing.go` fingerprints provider, reported model, and canonical model; `internal/db/usage.go` distinguishes reported/computed charges | Existing provider policies are not a general internal deployment price book. Add deployment-specific rate identity without collapsing same-named models. |
| Source provenance | Session/message source fields and source IDs in the archive; Claude request/message IDs support deduplication | Preserve existing provenance. Requested model, reported model, harness version, and deployment need a coverage audit per parser. Do not claim universal support. |
| Insight output | `internal/insight/generate.go`, `Result`: Content, Agent, Model | No execution usage, completion reason, backend identity, or process timing contract. The probe borrowed the Claude label; production must not. |
| Insight persistence | `internal/db/insights.go`: agent/model, content, template/schema versions, aggregate hash, provenance/structured JSON | Useful answer provenance exists, but there is no dedicated attempt ledger with the proposed accounting fields. |

The current paths inspected do not provide separate, consistently populated
requested-model and reported-model identities plus a deployment identifier.
Their absence must not be filled by guessing from `agent` or a model alias.

## Process interface

Configure a stable harness ID, absolute executable, and argument array. Do not
execute a shell command string. For the demonstrated client, arguments are
`["--pipe", "-m", "codex/gpt-6-luna"]`. The model selection is configuration,
not something derived by executing transcript text.

Write the complete prompt to stdin and close stdin. Read stdout and stderr
concurrently. Enforce output bounds and a deadline; record exit status and
cancellation. Do not persist stdout/stderr indiscriminately, because diagnostics
may contain transcript content or secrets. Stderr is not a usage protocol.

The production adapter identity should be `groundloop-ai` or another configured
harness ID, independently of model and provider. Keep executable/version identity
in local provenance; exclude credentials, raw environment, and authenticated URLs.
The precise credential environment policy remains a separate implementation
choice. Existing inherited credentials must not be mistaken for endpoint approval.

## Versioned result envelope

Use one JSON object on stdout, encoded by code in the external client. The
`result` string contains the model answer. Structured review JSON is encoded as
text within that string and validated by AgentsView after transport decoding.

See [the illustrative envelope](external-harness-result.example.json). Its IDs,
model names, counters, and backend are synthetic, not observed deployment facts.

Required fields for the new protocol:

- `schema_version`: `agentsview.execution-result.v1`.
- `result`: answer text, nonblank on a successful answer.
- `completion`: normalized reason and raw provider reason, if supplied.

Optional observation groups:

- `identity`: provider, backend, deployment, requested model, reported model,
  model revision, harness version, and provider request ID. Include evidence
  origin for each identity value: `configured`, `provider_response`, or `unknown`.
- `usage`: token counters, counter semantics, evidence origin, coverage, and
  whether counters cover the whole invocation or one provider request.
- `reported_cost`: currency, decimal amount string, scope, and evidence origin.
  Preserve null when the provider/client does not report a charge.

The wrapper must not manufacture reported identity by copying the requested
alias. A client returning only the answer leaves reported model/backend unknown.
Our earlier probe's model label came from its command arguments, so it proves
requested-model round-trip only, not provider-reported identity.

The runner owns execution/attempt IDs, timestamps, monotonic elapsed duration,
exit status, configured harness/deployment selection, prompt template/version
and hash, source references/revisions, and parser/validator version. The child
cannot overwrite runner-owned fields. Retain configured and observed identity
separately; flag disagreements instead of silently choosing one.

## Identity and aggregation

Keep these independent dimensions:

- Harness ID and version: the executable producing the request.
- Provider ID: the service provider, when known.
- Backend ID: the selected organizational service or gateway.
- Deployment ID: a stable, non-secret identifier for a specific deployment.
- Requested model: exact alias sent to the client/provider.
- Reported model and revision: exact identity supplied by the provider.
- Optional canonical family: a separate, versioned normalization for grouping.

For example, the same Opus family through Copilot and an enterprise backend
must remain separate billing identities. A family-level comparison may group
them explicitly; storage and pricing must retain backend/deployment distinctions.
Do not use endpoint hostnames as the sole deployment identity. Never infer
provider from a model prefix or harness name. Preserve unknowns in exports.

Original session logs need equivalent attribution where the source actually
records it. Any operator-supplied mapping must be scoped to a source and time
range, versioned, and labeled configured. It must not rewrite raw observations
or retroactively assign a backend to every historical use of a model alias.

## Token semantics and costs

Token fields are nullable nonnegative integers. Null means unknown; zero means
observed zero. Input, output, cache-read, cache-write, and reasoning counters
must carry inclusion semantics, because providers count them differently.
Reasoning is often included in output, and cache reads may be included in input.
Never sum all fields into a total without knowing these relationships.

A caller-side tokenizer count, if retained, is an estimate in a separate field,
not provider-reported usage. Capture failed attempts and retries if the client
exposes their usage. If it exposes only the final request, mark coverage partial
rather than claiming invocation totals. Missing usage does not mean free.

Preserve reported charges independently from calculated costs. Calculations
must record currency, rate-table ID/version, effective date, provider/backend/
deployment/model lookup keys, and which usage counters were priced. Use exact
decimal or fixed-point money, not binary floating point. Unknown rates yield
unknown calculated cost. Subscription charges and request-level costs are not
interchangeable; retain the pricing basis.

Do not recompute historical reported charges from today's prices. Recalculated
estimates need their own version/provenance. Do not sum reported and calculated
costs for the same attempt as two separate charges.

## Timing and completion

The runner records UTC start/end timestamps and monotonic elapsed milliseconds.
These measure subprocess execution, including startup and waiting. Provider
latency, generation duration, and time-to-first-token are distinct optional
observations. Do not call time-to-first-stdout-byte time-to-first-token when the
client buffers a complete envelope. Session elapsed time and inferred active
work intervals remain separately defined original-session metrics.

Normalize completion as `stop`, `length`, `cancelled`, `error`, `content_filter`,
or `unknown`; retain the raw provider reason. Exit zero alone does not prove a
complete model answer. Nonzero exit, truncation, invalid structured output, or
cancellation must not be stored as a successful complete insight. Preserve the
attempt and any known usage even when answer validation fails.

Unknown completion remains explicit. Legacy plain-text clients may be supported
as a compatibility mode with unverified completion, but must not silently become
verified complete. A strict mode should require positive completion evidence.

## Persistence and implementation sequence

1. Add a generic executable adapter and strict envelope parser. No HTTP model
   client or vendor CLI flags in this path. Keep prompt/schema validation local.
2. Add execution and attempt records separate from archive usage. An attempt
   links to its answer and source-reference set, not to source-session charges.
   Preserve raw observed counters alongside normalized facts and pricing results.
3. Have the external client expose structured metadata from its provider
   response, including finish reason. This requires work in Groundloop; plain
   answer output cannot supply these fields reliably.
4. Add backend/deployment dimensions to accounting, export, cache identities,
   and relevant UI groupings. Preserve unknown historical attribution.
5. Remove embedded insight/recall/embedding HTTP clients after the replacement
   paths and preserved prompt/schema contracts are tested. Recall and embeddings
   are separate integrations, not automatically covered by the summary probe.

Acceptance checks use synthetic data and disposable binaries: two backends with
one model alias remain distinct; missing usage stays null; overlapping token
categories are not double-counted; retries and failed attempts retain charges;
truncated answers cannot become complete insights; duplicate ingestion does not
add usage; summarizing a source session leaves its original accounting unchanged;
provider identity cannot be overwritten by requested aliases; saved records can
be reopened and exported with their provenance intact.

This contract does not claim that the app is network-isolated. Other transports
and existing generation paths remain pending the previously requested removal.
