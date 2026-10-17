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

PostgreSQL was the next cut, recorded below. ClickHouse/DuckDB analytics, SQLite vectors,
MCP, SSH/HTTP sync, and internal-model integration remain pending separate
scope decisions, as recorded in the requirements and checklist.

## PostgreSQL removal

Removed the PostgreSQL implementation and pgx driver, `pg` commands, direct
`--pg` reads, PostgreSQL push API routes, and the PostgreSQL CI database.
The PostgreSQL-only upload embedding builder, remote recipe adoption, raw
upload cleanup command, and hosted-contributor memory mode are also removed.
Shared watch and OS-service helpers now live under `replica_*` filenames for
ClickHouse. API clients were regenerated.

Legacy `[pg]`, named `[pg.*]`, `default_pg`, and `AGENTSVIEW_PG_*` settings
fail explicitly. `pg-serve` deployment mode is rejected. Existing SQLite
archives are not migrated or deleted. The UI, discovery, parsers, conversation
exports, local/HTTP MCP and memory access, SQLite vectors, ClickHouse/DuckDB,
and SSH/HTTP sync remain.

Declared module requirements fell from 136 to 131. The rebuilt embedded-UI
executable is 159,026,802 bytes, or 151.66 MiB, down 8.53 MiB from the S3 cut
and 56.58 MiB from the upstream baseline. Its import closure and linked module
inventory contain no pgx, jackc, or PostgreSQL implementation packages.

Validation:

- `make build`: passed, including the existing embedded UI.
- Regenerated API clients and `npm run check`: passed with zero errors or warnings.
- `go fmt ./...` and `go vet -tags fts5 ./...`: passed.
- All ordinary Go test packages and the retained `benchdb` benchmark package
  compile. No external database was started.
- Focused CLI, config, server, local/HTTP MCP, conversation-export, stable
  installation-identity, and ClickHouse service tests passed. SQLite semantic
  search and vector export regressions passed.
- Three daemon-discovery tests fail in both this checkout and an isolated
  checkout of the preceding commit: `TestSessionGetVariants`,
  `TestSessionUsage_NoSyncDiscoveredDaemon`, and
  `TestSessionUsage_UsesDiscoveredDaemon`. Each attempts forbidden test-binary
  daemon autostart after failing to discover its fixture. These pre-existing
  failures were excluded from the passing focused suite.
- Native Tauri packaging, browser smoke, and runtime traffic tracing were not
  performed. No private logs were used.

## Updates and pricing downloads removed

Removed the CLI self-updater, update-check HTTP route, web update prompts,
Tauri updater plugin, update menus, and updater signing/archive steps.
Daemon version comparisons remain in `internal/versionutil` because they
protect compatibility when a client connects to an existing daemon.

Removed LiteLLM, OpenRouter, and GenAI Prices network fetchers, the scheduled
pricing job, its unused poller package, and the unknown-model fetch fallback.
Startup and usage paths seed bundled rates locally. Existing stored prices,
normalization, cost calculations, and custom overrides remain. Replacing the
bundled third-party catalog with approved internal rates is still pending.

The existing immutable LiteLLM snapshot is now checked in, alongside the
GenAI snapshot. Builds, development scripts, API generation, CI, and container
builds no longer download pricing. `make clean` preserves both snapshots.
The LiteLLM snapshot source reference is
`7d50a31eb5b080c29438f97be7701e117938ce88`; its SHA-256 is
`a9b59c9780c31c299a7049d5cc15d6dec6b4418cf6f5363f85c35e85c965d3ac`.

The embedded-UI executable is 158,815,730 bytes, or 151.46 MiB, down 0.20 MiB
from the PostgreSQL cut. Go module requirements remain at 131. The desktop Rust
lockfile shrank from 556 to 526 package entries after removing the updater.
No native release-bundle size was measured.

Validation:

- `make build`: passed with the embedded UI and checked-in pricing snapshot.
- Pricing parsing and calculations, local catalog seeding, version comparison,
  CLI custom pricing, shared replica, server, and export regressions passed.
  Local seeding tests forbid HTTP requests and check stored rates survive.
- Build-script tests passed and assert no pricing generator is invoked.
  AppImage icon-repair tests and shell syntax checks passed.
- API clients regenerated; `npm run check` passed with zero errors or warnings.
  Frontend startup/sync tests passed, 95 tests. Frontend format/lint/type checks
  passed after removing an unused import.
- Tauri `cargo check` and all 107 Rust library tests passed. No desktop app was
  launched, installed, signed, or packaged.
- `go fmt ./...`, `go vet -tags fts5 ./...`, and all ordinary Go test-package
  compilation passed. One initial linker run exhausted disk space; after
  clearing this task's desktop build artifacts, that package passed with
  `-p 1`.
- Full runtime network tracing and browser smoke remain pending. Other external
  integrations and fonts remain, so this is not an offline-approved release.

## Next removals

[REMOVALS.md](REMOVALS.md) owns the removal checklist. Continue with automatic
external requests and remote services. Preserve the local discovery-to-SQLite-to-UI path throughout.

Use the Git diff against `UPSTREAM` as the initial patch history. A repeatable
upstream-update workflow and release vendoring remain pending.
