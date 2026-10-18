# External insight executable

The `external` insight agent invokes a configured binary with the prompt on
stdin. It uses no vendor flags or HTTP model client. Select External in the
existing insight selector, or set it as the default:

```toml
[insights]
default_agent = "external"

[insights.external]
id = "groundloop-ai"
binary = "/absolute/path/to/ai"
args = ["--pipe", "-m", "codex/gpt-6-luna"]
format = "text"
requested_model = "codex/gpt-6-luna"
# Set these to your own non-secret deployment identifiers when known.
# provider_id = "internal-provider"
# backend_id = "enterprise-gateway"
# deployment_id = "deployment-a"
# harness_version = "build-id"
```

`text` explicitly accepts unverified completion. It cannot supply reliable token
counts, reported model, or cost. Omit `format` or use `json` once the executable
emits the [versioned envelope](external-harness-result.example.json). The client
must populate metadata from provider responses, never from generated prose.
JSON mode requires `completion.reason = "stop"`. Nonzero exit, invalid metadata,
truncation, empty answers, and cancellation fail generation. Insight-specific
schema and citation validation still run afterward.

The executable inherits the server environment for credentials. Configure it as
a trusted program. Arguments are passed directly without a shell. Stderr is
discarded; stdout is limited to 8 MiB. Each invocation has a three-minute limit,
or the caller deadline if earlier. The adapter does not sandbox the executable
or its descendants. Do not place secrets in configuration arguments.

## Execution accounting

`harness-executions.db` in the configured data directory is persistent SQLite,
separate from the conversation archive. Back it up alongside the archive. Each
invocation, including correction retries, gets an independent random ID and a
record in `executions`. `record_json` preserves configured identity separately
from observations, nullable usage counters, decimal reported cost, prompt SHA256,
UTC timestamps, monotonic duration, exit code, and saved insight ID. It does not
store the prompt, answer, stderr, credentials, or environment. The existing
insight archive stores accepted answers as before.

Read or export records locally:

```sh
sqlite3 /path/to/data/harness-executions.db \
  'SELECT record_json FROM executions ORDER BY rowid;'
```

`running` after a crash is indeterminate. `response_received` means the process
returned a complete envelope, but insight validation/save is pending.
`unverified_response` is the text equivalent. Final states are `saved`,
`saved_unverified`, `not_saved`, or `failed`. Saving the archive and ledger is
not one transaction; a ledger write failure is logged and may leave a pending
record even when the answer was saved. Do not interpret pending or failed runs
as free. Usage returned in valid JSON on a nonzero exit remains in the ledger.
Malformed JSON cannot provide trustworthy usage.

Counters retain their inclusion semantics; this implementation does not add
categories or calculate prices. Original session usage rows are unchanged.
Canned-insight cache keys include the external configuration so different
backends/deployments do not share cached answers.

## Remaining work

The ledger currently records a prompt digest and saved insight link. Explicit
source revision sets, template/validator versions, observed/configured identity
conflict flags, grouped retry identities, rate-table calculations, and accounting
UI/export integration from the broader contract remain to be implemented.
The client must expose structured metadata before its usage can be recorded.

Existing vendor runners and embedded insight, recall, and embedding HTTP clients
still exist. This change does not establish network isolation. Their removal is
separate pending work; do not load private data on the assumption it is complete.

## Verification

The production adapter returned `SYNTHETIC_OK` from a synthetic prompt through
the Groundloop client in text mode. The record correctly remained
`unverified_response`, with no invented token counts or reported model. Tests
cover strict JSON, truncated answers, nonzero exits with usage, missing counters,
separate backends, cancellation, output limits, and reopening saved records.
