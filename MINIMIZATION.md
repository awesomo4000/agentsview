# Local fork minimization

## Retained core

Keep harness/log discovery, local ingestion, normalized SQLite conversations,
and the UI. The approved direction is in
[x/agentsview-local-fork-requirements.md](x/agentsview-local-fork-requirements.md).
The archive and its normalized conversation export will support a future
internal summarizer approved for private data.

## Baseline

Source: `kenn-io/agentsview` at
`a22d9d1e33006b157746043e84ca5da0b75bcb48`, also recorded in `UPSTREAM`.

`make build` passed on macOS ARM64 with Go 1.27.2, Node 26.12.0, and npm 11.20.0.
The upstream executable with embedded UI is approximately 208 MiB. Its SHA-256
is `cd99a76166cdeda6a8bcd3f0f158122b7e2d78181be7623f86081b982db961bc`.
A comparison copy was kept at `/tmp/avlocal-upstream-a22d9d1e`.
No binary was installed or launched against private logs.

The upstream build downloads a pricing snapshot and npm/Go dependencies.
Frontend installation reported six audit findings, three low and three high.
Those findings have not been triaged; dependencies were kept at the baseline.
The native Tauri application bundle has not been built.

## First removal

Removed AgentsView's telemetry package, its startup/shutdown integration and
daily pings, UI capture API, daily screen-claim state, and all frontend tracking
calls and visit-tracking listeners. Tests for those deleted features were
removed. The server now has a regression test that the capture endpoint is
absent. Export tests check that downloads still work without telemetry.

Preserved the installation identity and its historical filenames because the
archive uses it as the source-machine key. Removing that key would change
session attribution. It is no longer passed to AgentsView's telemetry sender.

At the first checkpoint, `go mod tidy` did not change the manifests. PostHog
remained through this dependency chain, removed in the second cut below:

```text
internal/artifact
  -> go.kenn.io/docbank
  -> go.kenn.io/docbank/internal/api
  -> go.kenn.io/docbank/internal/telemetry
  -> go.kenn.io/kit/telemetry/posthog
  -> github.com/posthog/posthog-go
```

This build has not met the no-external-traffic requirement. Do not mistake the
first removal for completed privacy hardening.

## Validation

- Unmodified upstream `make build`: passed.
- `make build` after removal, including embedded frontend: passed.
- Eight affected frontend suites: 260 tests passed.
- `npm run check`: generated API check passed; Svelte reported zero errors and
  zero warnings.
- Focused Go tests across CLI, server, config, database, and parser: passed.
  These cover local session ingestion with stable machine identity, removed
  telemetry endpoint, installation identity, conversation export/reopen and
  revision behavior, and Claude metadata/identity parsing.
- `go fmt ./...`: passed.
- `CGO_ENABLED=1 go vet -tags fts5 ./...`: passed.
- Full parser suite, full SQLite suite, upstream golden parity, UI browser smoke,
  and external-network runtime tracing: not run in this first pass.

The selected Go tests used the following filter with `-tags fts5` and `-count=1`:

```text
Test(SessionSyncUsesInstallationIdentityAndPreservesHistoricalNames|TelemetryCaptureRouteRemoved|Installation|ConversationExport|ParseClaudeSession_Metadata|ClaudeSessionIdentity)
```

## Artifact vault removal

Removed the docbank artifact ledger and folder exchange, raw archive vault
backup/recovery, and hosted raw PostgreSQL provisioning, serving, and reparse.
The `archive` command, `sync --target`, and artifact-exchange HTTP endpoint are
gone, including their generated clients and feature-specific tests.

SQLite archive-mode detection and interrupted-compaction recovery remain.
Local capture bundles retain the same raw-source wire fields and validation,
now owned by `internal/capture` instead of the deleted artifact package.
Conversation export, parsers, local sync, and the UI remain.

`go mod tidy` removed 29 module requirements, reducing the declared total from
173 to 144. This includes docbank, PostHog, and unused AWS SDK v2, PDF, and MongoDB
dependencies. Other remote transports and backend dependencies remain.

The embedded-UI build is 169,820,402 bytes, or 161.95 MiB, compared with the
218,355,602-byte upstream baseline, or 208.24 MiB. This is a cumulative reduction
of 46.29 MiB, about 22%, including the earlier application telemetry removal.
Native Tauri packaging and runtime network tracing remain untested.

Validation for this cut:

- `make build`: passed, including the embedded frontend.
- Focused Go regressions: passed across CLI, server, config, SQLite, parsers,
  capture, and backfill. Coverage includes source-machine identity, conversation
  exports, interrupted-compaction recovery, archive-only behavior, raw-source
  validation, and synthetic Claude backfill.
- API clients regenerated from the server schema.
- `npm run check`: passed with zero errors and warnings.
- `go fmt ./...`, `go vet -tags fts5 ./...`, and compilation of all ordinary
  Go tests: passed.
- Retained PostgreSQL integration tests compile with `-tags "fts5 pgtest"`.
  No PostgreSQL instance was started or tested.
- `go list -deps ./cmd/agentsview` and linked Go symbols contain no docbank or
  PostHog packages.

## S3 removal

Removed the MinIO transport, S3 discovery interfaces and scanners, downloads,
sidecar hydration, object metadata handling, and periodic object-root polling.
The dedicated Docker S3 test target and S3 operator instructions are removed.
Local session-ID prefixing and freshness helpers formerly stored in S3 files
remain available to local and SSH/HTTP ingestion.

Configured `s3://` roots now fail with an explicit unsupported-source error,
including uppercase scheme spelling. Existing SQLite conversations are not
migrated or deleted. Local roots retain their provider discovery, sidecar,
compressed-rollout, stable-identity, and archive-export paths. The UI is unchanged.

`go mod tidy` removed MinIO and seven supporting modules, reducing declared
requirements from 144 to 136. The rebuilt executable with the existing embedded
frontend is 167,969,074 bytes, or 160.19 MiB, down another 1.77 MiB. Its Go import
closure contains no MinIO, AWS SDK, or Smithy packages.

Validation:

- `CGO_ENABLED=1 go build -tags fts5 -o agentsview ./cmd/agentsview`: passed.
- Focused tests across config, parser, sync, CLI, and database: passed. These
  cover S3 configuration rejection, local harness parsing, compressed Codex
  ingestion and duplicate selection, Cursor ingestion, source-collision handling,
  Claude archived tool-result preservation, stable machine identity, and
  conversation exports.
- `go fmt ./...` and `go vet -tags fts5 ./...`: passed.
- All ordinary Go test packages compile with `-tags fts5`.
- Native Tauri packaging, UI browser smoke, and runtime traffic tracing were not
  repeated in this backend-only cut. No private logs were used.

PostgreSQL remains a later removal. ClickHouse/DuckDB analytics, SQLite vectors,
MCP, SSH/HTTP sync, and internal-model integration remain pending separate
scope decisions, as recorded in the requirements and checklist.

## Next removals

[REMOVALS.md](REMOVALS.md) owns the removal checklist. Continue with automatic
external requests and remote services. Preserve the local discovery-to-SQLite-to-UI path throughout.

Use the Git diff against `UPSTREAM` as the initial patch history. A repeatable
upstream-update workflow and release vendoring remain pending.
