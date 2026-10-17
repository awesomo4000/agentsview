# Removal checklist

Make the fork smaller while preserving harness discovery, log parsing, SQLite
storage, and the UI. Checked items are implemented. Unchecked items remain in
the code or still need verification.

## Telemetry

- [x] Remove AgentsView's PostHog reporter and embedded application key.
- [x] Remove daemon activity pings and screen-view claim bookkeeping.
- [x] Remove frontend event reporting, visit tracking, and tracking listeners.
- [x] Remove the telemetry capture HTTP endpoint.
- [x] Remove the artifact/docbank dependency path that brought in PostHog.
- [x] Verify PostHog is absent from the resolved dependencies and linked binary.

Keep the installation identity for now. Despite its historical
`telemetry-install-id` filename, the archive also uses it for source-machine
identity. Any later migration must preserve existing attribution.

## Automatic external requests

- [x] Remove update checking and self-update commands.
- [x] Remove Tauri auto-update and its remote configuration.
- [x] Remove scheduled pricing refresh and online catalog fetchers.
- [ ] Replace the embedded third-party pricing catalog with supplied local rates.
- [x] Remove the build's pricing-snapshot download dependency.
- [x] Bundle fonts locally and remove Google Fonts requests.
- [ ] Audit other UI resources and webview navigation for external requests.

## Remote services and transports

- [x] Remove S3 discovery, ingestion, polling, and MinIO client dependencies.
      Reject configured S3 roots; preserve previously archived conversations.
- [ ] Review SSH/HTTP sync for a possible future internal upload path. Hold removal.
- [ ] Review remaining raw sync and remote connectors before further removal.
- [x] Remove PostgreSQL, its driver, direct read flags, API routes, and CI service.
      Reject legacy PostgreSQL configuration; preserve the SQLite archive.
- [x] Remove ClickHouse replication, serving, service installation, configuration,
      client dependencies, and integration-test service. Preserve SQLite analytics.
- [ ] Retain DuckDB for local analytics while evaluating an optional build or
      separate analysis tool.
- [x] Remove artifact-folder exchange, raw archive vault, and docbank services.
      Preserve the SQLite conversation-export interface.
- [x] Remove hosted raw PostgreSQL provisioning, serving, and reparse entry points.
      Raw protocol/client packages remain pending the remote-sync removal.
- [x] Remove reverse-proxy/Caddy management and public-listener configuration.

Local discovery, file watching, incremental import, reconciliation, and SQLite
writes are retained. Upstream calls much of this "sync"; do not delete it along
with remote sync.

## Imports, publishing, and model clients

- [x] Remove live Claude.ai account sync and browser relay/authentication plumbing.
      Retain local Claude.ai, ChatGPT, and Gemini archive file imports.
- [x] Remove unused remote SQL replica orchestration and vector-push adapters.
      Keep DuckDB push/watch and local vector build/search.
- [x] Remove Cursor Admin API integration.
- [x] Remove GitHub Gist publishing and related UI actions.
- [ ] Review model clients for compatibility with approved internal models;
      retain pending that decision.
- [ ] Evaluate internal embedding support; retain SQLite vectors and semantic
      search for now.
- [ ] Evaluate MCP for assistant access; retain it for now.
- [ ] Remove configuration, UI controls, generated API clients, and tests owned
      exclusively by each deleted feature.

The future internal summarizer is separate work. Preserve normalized
conversation content, stable IDs, revisions, project evidence, and explicit
exports. Do not invoke a summarizer or choose a service as part of these cuts.

## Local server and execution

- [ ] Default to `127.0.0.1:47831` across server and desktop launch paths.
- [ ] Keep `--port`, reject invalid ports, and fail on a collision instead of
      silently selecting another port.
- [ ] Remove wildcard/LAN serving and arbitrary remote connection settings.
- [ ] Implement and test local API authentication, Host/Origin validation, and
      CSRF protection according to the requirements.
- [ ] Remove parser mount-command execution and unrelated subprocess launchers.
- [ ] Audit and confine scratch files and persistent state to app-owned paths.

The UI listener now rejects non-loopback hosts. Remote connection settings
remain for the deferred SSH/HTTP review. Legacy Caddy process metadata and
identity-checked stop cleanup remain for compatibility; Caddy cannot be launched.

Keep the localhost listener and the desktop backend launch. A blanket ban on
networking packages or subprocess symbols does not fit the retained UI.

## Repository and dependencies

- [ ] Remove deployment files, plugins, docs, scripts, and workflows used only
      by deleted features. Keep build, test, and upstream-sync documentation.
- [ ] Run dependency cleanup after each cut and record remaining dependency paths.
- [ ] Record binary size and dependency counts at meaningful checkpoints.
- [ ] Pin and vendor release dependencies where supported.
- [ ] Establish a repeatable process for applying upstream parser/ingestion fixes.

## Acceptance before calling it minimized

- [ ] Verify discovery and ingestion against a synthetic multi-agent corpus.
- [ ] Verify SQLite reopen, stable identities, revisions, and archive preservation.
- [ ] Verify conversation export can support the approved internal summarizer.
- [ ] Verify usage deduplication and accounting parity.
- [ ] Exercise the retained UI with external networking denied.
- [ ] Trace backend and webview traffic and confirm no external requests or DNS.
- [ ] Verify read-only agent sources and writes confined to app-owned locations.

See [MINIMIZATION.md](MINIMIZATION.md) for results already recorded. This list
is the work queue; it is not a claim that the unchecked features are gone.
