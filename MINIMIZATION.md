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

`go mod tidy` completed without changing the manifests. PostHog is still needed
by this transitive dependency chain:

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

## Next removals

1. Trace artifact/docbank consumers and remove the dependency and its embedded
   telemetry while preserving SQLite conversation export and core UI reads.
2. Remove update checking/self-update, desktop auto-update, pricing refresh,
   and external font loads. Preserve the UI typography by bundling fonts.
3. Remove remote replicas, imports, publishing, and external LLM/embedding
   clients in separately verified changes. Keep local ingestion and SQLite.
4. Apply the fixed `127.0.0.1:47831` default and `--port` override across server
   and desktop launch paths, remove automatic port fallback, and implement the
   access controls specified in the requirements.
5. Exercise the retained UI and synthetic ingestion pipeline under denied
   external networking, then complete the archive/export contract for the
   approved internal summarizer.

Use the Git diff against `UPSTREAM` as the initial patch history. A repeatable
upstream-update workflow and release vendoring remain pending.
