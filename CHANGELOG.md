# Changelog

## 10/21/2026

- Add and remove session tags and edit a local summary in the Analysis panel.
  Find saved summaries through SQLite FTS. Preserve annotations through
  transcript reimports and archive rebuilds, separately from usage accounting.

- Use SQLite FTS for session search, including browsers with old semantic-search
  preferences. Remove DuckDB, embedding engines, their commands and settings,
  and their native dependencies. Preserve the SQLite archive, user labels, and
  existing mirror/vector files.

## 10/20/2026

- Set the search direction to SQLite FTS5, optional approved session summaries,
  and user tags. Queue DuckDB and embedding machinery for removal; record
  searchable summaries and convenient tag editing as unfinished work.

## 10/18/2026

- Keep archive browsing local: remove built-in model senders, vendor CLI
  generation, remote sync/uploads, GitHub lookups, and session launch actions.
  Restrict browser API calls and navigation, and confine daemon HTTP to literal
  loopback without proxies or redirects. Disable remote DuckDB Quack and external
  extension access. Explicitly configured external insight executables remain
  available and retain their own network permissions.

- Run insights through a configured external executable with a strict JSON
  result envelope or explicit plain-text compatibility mode. Record each
  invocation in separate SQLite storage, preserving usage and deployment
  identity without changing source-session charges. Keep existing UI selectors
  and partition cached insights by external configuration.

- Audit existing usage and provider fields and define the external-harness
  execution contract. Keep source-session accounting separate from summary
  attempts, with explicit deployment identity and unknown-value semantics.
  This documents planned behavior; runtime and storage are unchanged.

## 10/17/2026

- Add a standalone Go probe showing Groundloop `ai` working through the existing
  harness adapter with a synthetic prompt. Document the compatible JSON envelope
  and the remaining need for a generic executable adapter.

- Preserve built-in summarization, recall, and review prompts under `x/prompts`,
  with source snapshots, schemas, and an external-harness execution guide.
  Verify the adapter with a synthetic local executable; no model is invoked.

- Default CLI and desktop UI serving to loopback port 47831. Preserve `--port`
  and saved port overrides; reject occupied ports instead of silently moving.
  Explicit `--port 0` remains available for temporary listeners.

- Remove live Claude.ai sync, Chrome relay/extension, and desktop web-account auth.
  Keep local chat-archive imports, harness discovery, and log ingestion.
- Remove Cursor Admin API, GitHub Gist publishing, managed Caddy/public hosting,
  and unused remote SQL replica/vector-push plumbing. Keep local exports, DuckDB,
  SQLite vectors, MCP, and SSH/HTTP sync. Require loopback for the UI listener.
- Bundle Inter and JetBrains Mono with their licenses for local UI rendering.

- Remove ClickHouse replication, serving, and background service installation.
  Reject obsolete ClickHouse settings; retain SQLite analytics, DuckDB, and the UI.

- Remove automatic update checks, the self-update command, web update prompts,
  and the Tauri updater. Install future fork builds through internal distribution.
- Remove public pricing downloads and scheduled refreshes. Bundle the existing
  pinned pricing snapshot for builds; preserve stored rates, usage calculations,
  and local custom overrides while replacement internal rates remain pending.

## 10/16/2026

- Start the local fork from AgentsView commit
  `a22d9d1e33006b157746043e84ca5da0b75bcb48`, retaining the UI,
  harness/log discovery, ingestion, and SQLite conversation archive.
- Remove the application telemetry sender, daily reporting, UI tracking calls,
  and telemetry HTTP endpoint. Preserve source-machine identities used by the
  archive.
- Remove the artifact vault, folder exchange, raw archive backup/recovery, and
  hosted raw PostgreSQL service, eliminating docbank and its transitive PostHog
  dependency. Preserve SQLite archive recovery, conversation exports, and local
  capture bundle validation.
- Add a fork-specific README and removal checklist. Preserve the original
  upstream README as `README.upstream.md`.
- Remove S3 discovery, downloads, sidecar hydration, periodic polling, and the
  MinIO client. Reject legacy S3 configuration while retaining archived SQLite
  conversations and local harness ingestion.
- Remove PostgreSQL replication, serving, direct reads, background upload
  services, remote embedding-recipe adoption, and the pgx driver. Reject
  obsolete PostgreSQL settings. Retain SQLite storage and vector search, the
  UI, MCP over local/HTTP reads, ClickHouse/DuckDB, and SSH/HTTP sync.
