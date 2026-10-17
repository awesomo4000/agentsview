# Minimization fork of AgentsView

A stripped-down fork of [AgentsView](https://github.com/kenn-io/agentsview)
for private, local work. Keep the useful core, remove external services and
features we do not need.

The [original upstream README](README.upstream.md) is preserved from our pinned
baseline. It describes upstream, including features this fork is removing.

## What stays

- Discovery of coding-agent harnesses and their logs.
- Parsers that normalize different agent formats into a local SQLite archive.
- Conversation content, stable identities, source provenance, and usage data.
- The existing UI for browsing and searching that archive.
- Local conversation exports for a future internal summarizer approved for
  private data, with summaries used to map conversations to projects or
  workstreams.

## What goes

The [removal checklist](REMOVALS.md) tracks completed work and upcoming cuts.
Targets include telemetry, update services, online pricing, external UI assets,
S3, PostgreSQL, ClickHouse, web imports, and publishing. DuckDB, vectors, MCP,
SSH/HTTP sync, and internal-model integration remain under review.
Local log-to-SQLite ingestion stays.

Removed so far: application telemetry, UI tracking, the artifact vault and
folder exchange, raw archive backup/recovery, and the hosted raw PostgreSQL
service. Removing docbank also removes the transitive PostHog dependency.
S3 discovery and ingestion are also removed, including the MinIO client.
PostgreSQL replication, serving, direct reads, and the pgx driver are removed.
Automatic updates, self-update installers, and public pricing refreshes are
also removed. Bundled rates and local custom pricing remain.
ClickHouse replication, serving, service installation, and its client dependencies
are removed. SQLite analytics and the optional DuckDB mirror remain.
Live Claude.ai account sync, browser relay/authentication, Cursor Admin API,
GitHub Gist publishing, and managed Caddy/public hosting are removed. Local
chat-archive file imports and exports remain. Fonts are bundled locally.
Other external integrations remain. This is a development fork, not yet an
Internet-independent or privacy-approved release.

## Build

The current build uses the inherited toolchain: Go 1.27.2 with CGO, a C compiler,
and Node.js 24.11+ in the 24.x line or Node.js 26+. The frontend declares npm
12.2.0; the initial baseline was also built successfully with npm 11.20.0.

```sh
make build
```

This builds `./agentsview` with the frontend embedded. It currently downloads
Go/npm dependencies. Pricing snapshots are checked in. Build-time and runtime network
removals are tracked separately in the checklist. It does not install the
binary or build the native Tauri bundle.

The planned server default is `127.0.0.1:47831`, with `--port` to override it.
That change is not implemented yet; the inherited default is still `8080`.

## Working on this fork

- Commit and push fork work to
  [awesomo4000/agentsview](https://github.com/awesomo4000/agentsview).
- `origin` is the fork. `upstream` is the original repository, used only to
  fetch fixes. Do not open upstream PRs for the minimization work.
- Keep the parser, SQLite ingestion, and UI working after each removal.
- Preserve the MIT license and upstream copyright notices.

See the [requirements](x/agentsview-local-fork-requirements.md),
[build and validation record](MINIMIZATION.md), and [changelog](CHANGELOG.md).
The pinned upstream commit is recorded in [UPSTREAM](UPSTREAM).
