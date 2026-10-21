# Local AgentsView fork requirements

Status: approved direction, implementation in progress.
Updated: 10/21/2026.
Working name: `avlocal`.
Upstream: `kenn-io/agentsview`, commit
`a22d9d1e33006b157746043e84ca5da0b75bcb48`.
Retain the upstream MIT license and copyright notice.

## Purpose

The main value of AgentsView is locating coding-agent harnesses and their logs,
parsing the different formats, and storing their conversations and metadata in a
standard SQLite database. Preserve that pipeline and the existing UI.

We expect to use an internal summarizer approved for private data to summarize
conversations and map them to projects or workstreams. This is a future consumer
of the local archive. Accounting and usage reports remain useful outputs, but
are not a reason to discard conversation content or the SQLite ingestion layer.

The earlier parser-only proposal is
[archived separately](agentsview-parser-only-proposal.superseded.md).
It is historical reference, not a deletion checklist or acceptance contract.
This document replaces it.

## What we keep

- Harness and log discovery, including default locations and explicit roots.
- Upstream parsers and normalization across supported agent formats.
- Local ingestion, incremental updates, reconciliation, and the SQLite archive.
- Stable conversation, message, source, agent, and project identities, timestamps,
  model/usage metadata, and conversation content needed for local viewing and
  approved summarization.
- SQLite migrations and preservation of archived sessions whose source logs
  have disappeared. Never recreate the archive as a shortcut for an upgrade.
- The existing frontend, local HTTP API and event streams, and desktop wrapper.
- Local search and database/query support required by the retained UI.
- Usage accounting and local report/export capabilities.

Trace dependencies before removing packages. In particular, `internal/db`,
`internal/ingest`, `internal/sync`, `internal/server`, `frontend`, and `desktop`
are not blanket deletion targets. Local log-to-SQLite sync is core functionality;
outbound SSH/HTTP sync has been removed. Future internal uploads require a separate
explicit design and approval.

## Session discovery and organization

Users need to understand what sessions were about and organize a tractable
personal archive. Use SQLite FTS5 and the existing UI; embeddings and semantic
search are not requirements. This decision supersedes the earlier vector-search
exploration.

- Search conversation text and, when available, approved summaries and titles.
  Keep summaries separate from original transcript and usage facts.
- Show readable result snippets and session context, with filters for project,
  harness, date, and user tags. Users should not need to learn FTS query syntax.
- Let users add and remove session tags directly in the UI. Preserve tags through
  reingestion and keep user choices separate from summarizer suggestions.
- Make approved summaries optional. Keyword search and manual tagging must work
  without running a model or configuring an external executable.
- Keep SQLite as the archive and analytics engine. Consider a separate DuckDB
  analysis tool only if measured queries justify it later.

Current implementation already has conversation FTS indexes, session-label
storage and mutation API, label display, and label filters. Searchable summaries
and convenient in-UI tag editing still need implementation/verification. This
section defines the target, not a claim that those workflows are finished.

## Local server

- Bind exclusively to `127.0.0.1`, default port `47831`.
- The server launcher accepts `--port <port>`, with values 1 through 65535.
  Explicit `--port 0` remains an opt-in temporary-port mode.
- Use the selected port consistently in the UI and desktop launcher. If occupied,
  report the conflict instead of choosing another port silently.
- Do not bind to wildcard or LAN addresses. A Unix socket bridge is deferred.
- A high port does not exclude ordinary browsers. Protect data APIs and event
  streams with a per-launch authentication token delivered through a private
  app bootstrap. Do not log or serve that token through a public endpoint.
- Validate the expected Host and allowed UI origins. Reject untrusted origins,
  avoid wildcard CORS, and protect state-changing requests against CSRF.
  Missing or null Origin headers do not prove trust.

## Privacy and removal boundary

Remove external integrations and their reachable code rather than relying on
configuration switches. Targets include telemetry, external usage reporting, crash reporting,
update checks, self-update, desktop auto-update, online pricing refresh,
cloud/web imports, publishing, S3, and PostgreSQL. Updates and pricing will be
maintained internally.

S3, PostgreSQL, ClickHouse, outbound SSH/HTTP sync, and embedded model senders
are removed. DuckDB and vector/embedding engines, launch paths, and UI controls
are also removed. Retain MCP
for now. Approved summarization uses only an explicitly configured external
executable. Review dependent UI controls as each removal is implemented.

The application must work without Internet access. Bundle fonts and other UI
assets locally. Constrain webview navigation and resource requests to approved
local resources. No automatic external requests or DNS lookups are permitted.

Loopback HTTP, local desktop IPC, and the specific desktop backend launch are
allowed. Therefore, a blanket ban on `net/http`, sockets, or all subprocess
symbols is not a valid acceptance test. Inventory network and process call sites
and remove unrelated execution, including parser mount commands. The system
webview itself has networking capability; do not claim that the whole desktop
application has none compiled in.

Agent source logs and agent-owned databases remain read-only. Preserve no-follow
and root-confinement behavior. The app writes its own SQLite archive and required
local state under an explicit data directory, separate from agent sources.
Exports go to an explicit output directory. Keep scratch files in a controlled
location and remove them after use. Do not install over or test against live
user data. Security fixtures must use disposable paths under `/tmp`.

The archive intentionally contains private conversation content. Accounting
exports exclude prompts, transcripts, and tool payloads. Conversation exports
for approved summarization are a separate, explicit operation and may contain
that content. Never send it to an unapproved service.

## Internal summarizer interface

Do not implement or invoke a summarizer in the initial minimization work.
Preserve the data needed to support one later. No provider, endpoint, or model
has been selected or authorized by this specification.

Prefer the existing normalized conversation export contract if source inspection
confirms that it fits. The summarizer should consume a consistent, read-only
snapshot or explicit export, with stable conversation/message identifiers and
revision/provenance metadata. It must not reconstruct normalized content by
independently reparsing agent logs.

Future results should associate a summary and proposed project/workstream mapping
with the source conversation and revision. Keep inferred assignments distinct
from source-derived project evidence so they can be reviewed and corrected.
The exact schema and any result-import workflow are deferred until we inspect
upstream's existing export and metadata facilities.

## Accounting and reports

Preserve upstream SQLite usage normalization and deduplication first. Do not
replace its SQL accounting with a new Go implementation merely to remove the
database. Check streaming snapshots, resumed/forked sessions, subagent copies,
request/session precedence, and discovery-order independence with fixtures.

Rates must eventually come from a supplied local table, without online catalogs.
Use integer money arithmetic and record the rate-file digest. Unknown models
must be explicit, never silently treated as free. Deterministic CSV, XLSX, and
versioned JSON reports remain planned deliverables, including session, model,
project, day, and user rollups. Keep accounting output separate from private
conversation exports. Preserve the detailed field/pricing requirements from
§5.5 of the historical proposal where consistent with this architecture;
its mandated stateless implementation and Go dedup rewrite do not apply.

## Work sequence and acceptance

1. Obtain the pinned upstream source and record it in `UPSTREAM`. Read its build
   and contributor instructions, preserve its license, and build the baseline
   with its frontend embedded. Record tool versions and build failures.
2. Establish an isolated fixture baseline for discovery, ingestion into SQLite,
   archive reopening, and UI/API reads. Use synthetic logs under `/tmp`, an
   isolated data directory, and blocked external networking. Do not launch
   unmodified upstream against the user's home or private logs.
3. Minimize in small, buildable changes. Start with automatic external behavior:
   telemetry, updates, pricing refresh, and external UI assets. Then remove
   remote backends, imports, publishing, and unused services after tracing their
   dependencies. Retain the local discovery-to-SQLite-to-UI path throughout.
4. After each removal, build and run relevant tests. Verify unchanged fixture
   ingestion, preserved identities/content, reopen behavior, usage totals, and
   retained UI functionality. Track pre-existing failures separately.
5. Implement and verify the fixed port and `--port` override across launch paths.
   Test port conflicts and access controls, including unauthenticated API calls,
   invalid Host headers, and untrusted origins.
6. Verify no external traffic under runtime tracing, allowing loopback and
   approved local IPC. Audit source and dependency changes for remaining external
   clients. Check writes stay within the isolated app data, scratch, and output
   locations and agent inputs remain unchanged.
7. Document the archive/export contract for the future internal summarizer.
   Add accounting exports and local pricing after the retained core is stable.

Keep changes traceable to the upstream commit and maintain a reproducible patch
or transform process for future parser/ingestion fixes. Pin dependencies and
vendor where supported before release. Build dependencies may be downloaded
during development; the shipped app must not require network downloads.

## Current implementation status

The pinned upstream source and Git history are now in this workspace. The
unmodified baseline built successfully with its embedded frontend on macOS ARM64,
using Go 1.27.2, Node 26.12.0, and npm 11.20.0. The baseline executable is
approximately 208 MiB. No installation or launch against private logs occurred.

The first minimization change removes AgentsView's own telemetry reporter,
daily pings and screen-claim bookkeeping, HTTP capture endpoint, and frontend
tracking calls. It preserves discovery, ingestion, SQLite, and conversation
exports. The legacy installation identity filenames remain because those IDs
also identify source machines in the archive.

The second cut removes the artifact vault, folder exchange, raw archive
backup/recovery, and hosted raw PostgreSQL service. Removing docbank eliminates
the transitive PostHog dependency. SQLite archive recovery and export remain.
S3 discovery, downloads, polling, and the MinIO client are now removed. Legacy
S3 configurations are rejected; existing SQLite conversations remain.
PostgreSQL replication, serving, direct reads, and its driver are removed.
The `pg` commands, `--pg` flags, and hosted-contributor memory mode are gone.
Legacy PostgreSQL configuration is rejected. Local/HTTP MCP and memory reads,
SQLite vectors, and DuckDB remain. ClickHouse is removed.
Automatic updates and public pricing refreshes are now removed, including
the Tauri updater and build-time snapshot downloads. Bundled rates, stored
pricing, and local custom overrides remain until internal rates are supplied.
External fonts and other remote features remain.
This is an intermediate development build, not an offline-approved release.
The fixed port and access-control requirements above remain pending.

The build uses checked-in pricing snapshots and installs frontend packages,
including a pinned public GitHub dependency on `kenn-io/kit-ui`. Those build
steps remain intact for baseline comparison. `go mod tidy` completed after the
vault removal; docbank and PostHog are no longer in the manifests.

Validation results and the next removal targets are tracked in
[MINIMIZATION.md](../MINIMIZATION.md).

## Current minimization checkpoint

Live Claude.ai account sync, browser authentication/relay, Cursor Admin API,
Gist publishing, managed Caddy/public hosting, and unused remote SQL replica
adapters are removed. File-based chat archive import, local exports, harness
parsers, SQLite storage, the UI, DuckDB, local vectors, MCP, and SSH/HTTP sync
remain. Fonts are bundled locally. The UI listener requires loopback.

CLI and desktop now default to port 47831 and reject collisions. Explicit
port overrides and saved configuration remain supported. See REMOVALS.md and MINIMIZATION.md in
the repository root for the implementation checklist and measured results.
