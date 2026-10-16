# Superseded proposal

Historical reference only. The current requirements are in [agentsview-local-fork-requirements.md](agentsview-local-fork-requirements.md). The parser-only, stateless design and its deletion list are no longer the project scope.

# Local-Only Agent Usage Reporter — Software Requirements Specification

## Scope revision, 10/16/2026

**Keep the existing AgentsView UI and its localhost HTTP listener.** This decision supersedes the CLI-only architecture and conflicting removal rules below. The original extraction proposal remains below as a reference for the parser and accounting work, not as authorization to delete the UI.

- Preserve the frontend, desktop wrapper, and local server code needed to support the UI. CLI reporting remains in scope. Review UI dependencies before deleting packages; the parser-only package list in §5.1 is no longer the complete retention boundary.
- Bind the server exclusively to `127.0.0.1:47831` by default. The command that launches the server accepts `--port <port>` to override the default, with valid values from 1 through 65535. Keep the selected port fixed across launches with the same arguments. If it is occupied, report the conflict instead of silently choosing another port. The UI connects to the selected port. Never bind to a wildcard or LAN address. A Unix socket bridge is deferred.
- An unusual port is not access control. Ordinary browsers can reach loopback ports. Require an unpredictable per-launch authentication token for data APIs and event streams, provisioned to the app through a private bootstrap mechanism. Do not expose the token through an unauthenticated endpoint or log it.
- Validate the exact expected Host and permitted UI origins. Reject untrusted cross-origin requests, do not enable wildcard CORS, and protect state-changing requests against CSRF. Missing or null Origin headers do not establish trust. Authentication remains necessary regardless of headers.
- Keep all UI assets local, including fonts. Remove telemetry, online pricing, update checks, auto-update, remote imports, publishing, sync, and other external integrations. The app must work without Internet access.
- The security objective is no external network traffic, with local UI traffic explicitly permitted. The Go backend may compile `net` and `net/http` for serving the UI. Their presence alone must not fail acceptance. A system webview also has network capability, so the entire desktop app cannot be described as having no network capability compiled in.
- Local desktop-to-backend communication and the specific backend launch needed by the desktop wrapper are permitted. Arbitrary subprocess execution remains out of scope; remove parser mount-command execution as planned.
- Preserve read-only access to agent data and the report privacy rules. UI persistence and supporting local storage must be inventoried before deciding which archive/database packages can be removed; keeping the UI does not automatically authorize retaining every upstream service.

### Consequences for the original requirements

| Original section | Revised interpretation |
|---|---|
| §1 purpose, G4, §1.2 non-goals, §4 architecture | Local desktop/web UI plus reporting, with loopback HTTP permitted. The UI and its required local API/events are in scope. |
| §5.1–5.2 extraction boundary | Retain `frontend/`, `desktop/`, and the local server dependency subset needed by the UI. Remove external integrations after tracing those dependencies. |
| PRV-1, PRV-3, PRV-5, PRV-8 | Permit the local HTTP listener, desktop IPC, and narrowly scoped backend launch. Vendor UI assets and restrict webview requests/navigation to local app resources. |
| §7.1 V-1, V-2, V-4 | Replace blanket bans on networking symbols with checks for forbidden external integrations and a reviewed inventory of network/process call sites. Account for standard-library dependencies of the HTTP server. |
| §7.2 V-6 | Exercise the UI and report corpus with external networking denied and loopback allowed. Verify loopback-only binds, no external connections or DNS lookups, and only the approved backend launch. Test rejection of unauthenticated, invalid-Host, and untrusted-origin requests. Keep write-confinement verification, with any UI storage exceptions specified before implementation. |
| §9 milestones | Retain the UI throughout extraction and include local UI operation and access-control checks in acceptance. Parser/accounting/pricing milestones otherwise remain applicable. |

The exact retained UI dependency set, storage needs, and token bootstrap mechanism still require source inspection. The parser-only proof of concept does not establish acceptance for the revised application.

---

| | |
|---|---|
| **Status** | Draft for development team |
| **Date** | 2026-10-16 |
| **Upstream baseline** | `kenn-io/agentsview` commit `a22d9d1e33006b157746043e84ca5da0b75bcb48` (2026-10-16, v0.45.0 line) |
| **Upstream license** | MIT (Kenn Software LLC). The fork must retain the upstream `LICENSE` text and copyright notice. |
| **Working name** | `avlocal` (module path to be assigned internally) |

---

## 1. Purpose

We need per-session and per-model accounting of AI coding-agent token usage for internal projects whose data must never leave the user's machine. The outputs are summaries and spreadsheets for accounting.

Upstream AgentsView already parses 70+ agent log formats correctly. Its parser layer, including the handling of streaming snapshots, resumed sessions and subagent copies, is the only part we need.

The goal of this project is to **extract the minimum code from AgentsView needed to parse agent logs and produce usage reports**, in a **standalone** module with **no network capability compiled in**. Removing code is the preferred mechanism throughout. A feature that is disabled by configuration still exists, so it does not satisfy a requirement in this document.

### 1.1 Goals

- G1. Parse local agent session logs, at least as completely as upstream, into normalized usage records: agent, session, project, model, timestamp, and token counts by class.
- G2. Produce deterministic CSV (comma-separated values), XLSX (Excel workbook) and JSON (JavaScript Object Notation) reports that accounting can consume.
- G3. Price usage only from an internally supplied rate table.
- G4. Make it provable from the built binary and from source that the tool cannot open network connections.
- G5. Keep the codebase small enough to audit, and keep it re-syncable with upstream parser fixes.

### 1.2 Non-goals

These are explicitly out of scope and must not be ported:

- Web user interface (UI), desktop app, REST (Representational State Transfer) API (application programming interface), SSE (server-sent events), and MCP (Model Context Protocol) server.
- Persistent archive, full-text or semantic search, quality signals, Recall, Generated Insights, health scores.
- Any sync or backend: PostgreSQL, ClickHouse, DuckDB, S3 (Amazon Simple Storage Service), hosted raw sync, filesystem sync, artifact folders, SSH (Secure Shell) remote.
- Publishing (GitHub Gist), Claude.ai or Cursor web imports, Cursor Admin API.
- Telemetry, update checks, self-update, online pricing catalogs.
- Any LLM (large language model) invocation. The internal summarizer is a separate program (§5.6).

---

## 2. Background: upstream audit findings

These findings motivate the removal-first approach. They were verified by source review and by running upstream behind a recording, refusing proxy with `strace` on `connect()`.

| Upstream behavior | Default | Upstream control | Evidence |
|---|---|---|---|
| PostHog telemetry to `us.i.posthog.com` (installation ID, version, OS (operating system)/architecture, daily liveness ping, UI events) | **On** | Environment variable only (`AGENTSVIEW_TELEMETRY_ENABLED=0`); no config key | `internal/telemetry/telemetry.go`, `go.kenn.io/kit/telemetry/posthog`; 4 connections observed |
| Pricing catalog fetch from `raw.githubusercontent.com` (LiteLLM, Pydantic genai-prices) and `openrouter.ai` at start and every 24 h | **On** | **None for the daemon**; `--offline` only on some CLI (command-line interface) commands | `cmd/agentsview/pricing_schedule.go`; still observed with telemetry and update checks disabled |
| Update check to `github.com` | On | `disable_update_check` / env | `internal/update` |
| Desktop auto-updater to `github.com/wesm/agentsview/...` (different owner than the repo) | On | `AGENTSVIEW_DESKTOP_AUTOUPDATE=0` | `desktop/src-tauri/tauri.conf.json` |
| Google Fonts loaded by web UI | On | None | `frontend/index.html` |
| Gist publishing, Insights, embeddings, PG/ClickHouse/DuckDB, hosted raw sync, Cursor Admin API, Claude.ai import, SSH remote binary download | Opt-in | Config | Various |
| Archive stores full transcripts, tool inputs and outputs (source code, secrets) | `archive_content = "full"` | Config | `~/.agentsview/sessions.db` |

Upstream is a 215 MB binary with about 2,700 Go files. A configuration-hardened deployment still leaves reachable egress code paths, so it is rejected for this use.

---

## 3. Definitions

- **Provider:** upstream's per-agent-format parser implementation (`parser.Provider`), which handles discovery, fingerprinting and parsing.
- **Fact:** one normalized, unpriced usage row produced by `usagefacts.FromMessage` / `usagefacts.FromEvent`.
- **Snapshot group:** Claude streaming rows that share `(message.id, requestId)`. Each row is a cumulative snapshot of one API request.
- **General identity:** a row identity that recurs across sessions: a source UUID (universally unique identifier) or a usage-event dedup key. Resumed and forked sessions and subagent copies replay rows with the same identity.
- **Egress:** any attempt to open a non-loopback network connection, resolve DNS (Domain Name System) names, or invoke a process that could do so.

---

## 4. Architecture

```
 ┌────────────────────────────────────────────────────────────┐
 │ avlocal (single static-ish Go binary, CGO for SQLite only)  │
 │                                                            │
 │  discover ──► parse (extracted providers) ──► usagefacts   │
 │                                   │                        │
 │                                   ▼                        │
 │                     dedup + attribution (new)              │
 │                                   │                        │
 │                     price (internal rate table, new)       │
 │                                   │                        │
 │                 report writers: CSV / XLSX / JSON (new)     │
 └───────────────────────────────────┬────────────────────────┘
                                     │ files only
                                     ▼
              internal summarizer (separate program, separate audit)
```

- **Stateless.** Each run re-parses its sources. There is no database, cache, daemon or watcher.
- **Read-only** with respect to agent data. It writes only to the output path given on the command line.
- **No network packages linked** (§6).

---

## 5. Extraction boundary

### 5.1 Packages to keep

These were verified to compile as a closed set after the modifications in §5.3.

| Upstream package | Files | Lines | Role |
|---|---|---|---|
| `internal/parser` | 226 | ~115,000 | All agent providers, discovery, parsing |
| `internal/usagefacts` | 1 | 591 | Token-usage normalization (standard library only) |
| `internal/export` → **`projectid`** (subset) | 2 | 1,478 | Project identity: `project_identity.go`, `types.go` only |
| `internal/money` | 4 | 496 | Microdollar type used by parsed usage events |
| `internal/pathutil` | 2 | 129 | Path helpers |
| `internal/rawpath` | 1 | 68 | Path helpers |
| `internal/stringutil` | 1 | 33 | String helpers |
| `internal/ctxio` | 2 | 38 | Context-aware I/O (input/output) |

The resulting third-party module set is `mattn/go-sqlite3`, `tidwall/gjson` (+`match`, `pretty`), `klauspost/compress`, `ccoveille/go-safecast`, `pmezard/go-difflib`, `golang.org/x/exp`, `golang.org/x/net` (**`html` and `html/atom` packages only**), and `golang.org/x/sync`. The dependency graph contains **no** `net/http` or `crypto/tls`.

### 5.2 Everything else is deleted

This includes, but is not limited to: `cmd/agentsview`, `internal/{server,telemetry,update,pricing,pricingrefresh,db,sync,remotesync,rawarchive,postgres,clickhouse,duckdb,mcp,insight,recall,vector,cursorusage,sshremote,artifact,ingest,activity,service,config,apiclient}`, `frontend/`, `desktop/`, `plugins/`, `docs/`, `support/`, the Dockerfiles and compose files, `scripts/install.*`, `openapi.yaml`, `renovate.json`, and the PostHog `go.kenn.io/kit` dependency.

`go.mod` must be regenerated with `go mod tidy` after the deletion so that no removed module remains listed.

### 5.3 Required code modifications inside kept packages

Each item is a removal. A stub is acceptable only where marked, and only as an interim step.

| ID | Change | Reason |
|---|---|---|
| EXT-1 | Delete `internal/parser/s3source.go`, `s3source_test.go`, `s3_provider.go`, `claude_s3.go`, `codex_s3.go`, `cursor_s3.go`, `icodemate_s3.go`, and every S3 field, branch and URI check in the providers (`claude_provider.go`, `codex_provider.go`, `cursor_provider.go`, `discovery.go`, …). *Interim:* replacing the three minio-backed functions with error stubs is acceptable for the first milestone. | Removes the minio S3 client (HTTP, TLS) |
| EXT-2 | Create package `projectid` from `internal/export/project_identity.go` and `types.go`. Do **not** copy `pricing.go`, `canonical_json.go`, `reporting.go`, `reporting_joint.go` or `websearch.go`. Repoint `internal/parser/project.go`. | Breaks the parser → export → pricing → catalog → `net/http` chain |
| EXT-3 | Remove `runMountCommand` / `autofsMountSource` (`internal/parser/project.go`), which executes `/sbin/mount`. Replace it with a pure-Go read of the mount table (`/proc/self/mountinfo` on Linux; `getfsstat(2)` via `golang.org/x/sys/unix` on macOS), or drop automount classification. | No subprocess execution (PRV-5) |
| EXT-4 | Delete `WriteSanitizedWindsurfStateDB` (`windsurf_provider.go`). | Remote-sync export path; it writes files |
| EXT-5 | Change `discovery_disk_map.go`, which spills discovered paths into an SQLite temp file via `os.CreateTemp("")`, to an in-memory map. Alternatively, create the temp file only under a run-scoped directory that the tool removes on exit. | Writes outside the output path (PRV-6) |
| EXT-6 | Rename the module path, move the kept packages under the new module's `internal/`, and strip upstream branding from user-visible strings. | Standalone module |
| EXT-7 | Remove the remote-machine and sync-only surface from `ProviderConfig` and the providers where it is dead after §5.2: `SourceMachines`, `PathRewriter`, `RawCaptureProjectDirs`, `StoredPathResolver`, reconciliation and watch plans. Do this only once the golden tests (§7.3) are in place. | Reduce audit surface |

Agent SQLite databases are already opened read-only by upstream's `sqlite_dsn.go` (`mode=ro`, optional `immutable=1`; Goose also `mode=ro`). This must be preserved, and the database-opening call sites must be confined to that one helper (PRV-4).

### 5.4 New code

| Component | Notes |
|---|---|
| `cmd/avlocal` | CLI (command-line interface) entry point. Flags only; no environment variables that enable features. |
| Discovery root resolution | From each provider's `AgentDef.DefaultDirs` under `--home` (default `$HOME`), plus explicit `--root agent=path`. Replace upstream's config-file resolution, which is not ported. |
| Dedup and attribution | §5.5.3 |
| Pricing | §5.5.4 |
| Report writers | §5.5.5 |

### 5.5 Functional requirements

#### 5.5.1 Discovery

- FR-1. Scan the default roots of every retained provider, and accept explicit roots per agent.
- FR-2. Accept `--agents` include and exclude lists.
- FR-3. Accept a time window (`--since`, `--until`, inclusive, IANA (Internet Assigned Numbers Authority) time zone `--tz`, default UTC).
- FR-4. Never follow symlinks out of a configured root. Preserve upstream's `open_nofollow` behavior.

#### 5.5.2 Parsing and normalization

- FR-5. Use upstream providers unchanged except for the edits in §5.3. Parser fixes must be cherry-pickable from upstream (§8).
- FR-6. Normalize every message and usage event through `usagefacts.FromMessage` / `FromEvent`. Keep only `TokenEligible` facts.
- FR-7. Carry these fields per fact: agent, session ID, project (from `projectid`), model as reported, provider ID, source (`message`, `session`, …), timestamp (UTC), input, output, cache-write (5-minute and 1-hour), cache-read, reasoning tokens, web-search request count, and any provider-reported cost with its source.
- FR-8. Parsing errors are reported per source on stderr and in the report's `diagnostics` section. A broken file must not abort the run.

#### 5.5.3 Deduplication and attribution

Upstream implements these rules in SQL in `internal/db/usage_rollup_classify.go` and `usage_rollup*.go`, which are not ported. They must be re-implemented in Go and covered by tests derived from those files.

- FR-9. **Snapshot groups** (same `ClaudeMessageID` + `ClaudeRequestID`, across all sessions):
  - one contribution per group;
  - output tokens = maximum within the group (greatest-output winner);
  - web-search count = maximum within the group;
  - attribution goes to the earliest row.
- FR-10. **General identities** (source UUID, else usage-event dedup key, across all sessions): count once, attributed to the earliest row.
- FR-11. Attribution must be independent of discovery order: sort by timestamp, then by a stable key, before deduplicating.
- FR-12. **Session-level usage events** must not double-count message-level facts for the same session. Port upstream's request-scoped and session-scoped precedence (`usagefacts.SourceIsRequestScoped`, `RequestScoped`).
- FR-13. **Copilot** authoritative reported cost: follow upstream's selection rule, or exclude it and report the count of excluded rows. Decide this at milestone M2.
- FR-14. Every run emits counters: raw facts, facts dropped per rule, and final rows.

#### 5.5.4 Pricing

- FR-15. Rates come only from a local file passed with `--rates rates.toml`. It contains per-model integer microdollars per million tokens for input, output, cache-write 5-minute, cache-write 1-hour and cache-read. Matching is on the exact model string, with an optional explicit alias table, for example `gateway-sonnet → claude-sonnet-x`.
- FR-16. No embedded third-party catalog. The upstream LiteLLM and genai-prices snapshots are removed with `internal/pricing`.
- FR-17. **An unpriced model is an error, not $0.** Upstream silently prices unmatched models at zero. The tool must list unpriced models and exit non-zero unless `--allow-unpriced` is given. With the flag, the report flags those rows.
- FR-18. All money arithmetic is integer microdollars. There is no floating point before formatting.
- FR-19. The report records a SHA-256 digest of the rate file used.

#### 5.5.5 Outputs

- FR-20. `requests.csv`: one row per deduplicated, priced request. Columns are the FR-7 fields plus `cost_microdollars` and `priced_model`.
- FR-21. Rollups (CSV and one XLSX workbook with a sheet each):
  - by day × agent × model;
  - by project × model;
  - by session (start, end, models used, totals);
  - by user (OS username or `--user` label, for merging per-user reports centrally).
- FR-22. `report.json`: rollups plus metadata. The metadata covers tool version, upstream baseline commit, run window, time zone, rate-file digest, dedup counters, diagnostics, and a SHA-256 digest of the canonical rows.
- FR-23. Output is deterministic: identical inputs, rates and flags produce byte-identical files, apart from an optional `generated_at` field that is off with `--reproducible`.
- FR-24. Outputs contain no transcript text, tool inputs or outputs, file contents, or prompts.
- FR-25. Project identity handling is selectable with `--project-names {full|basename|hash}`. The default is `basename`, because full working-directory paths can themselves be sensitive.

#### 5.6 Summarizer interface

- FR-26. The tool never invokes the summarizer and has no code that could. The summarizer consumes `report.json` and the CSVs as files.
- FR-27. The `report.json` schema is versioned and documented (JSON Schema checked into the repo).

---

## 6. Privacy and security requirements

These are enforced by removing code and verified mechanically (§7). Configuration-based controls do not satisfy them.

- PRV-1. **No network capability compiled in.** The binary must not link `net/http`, `crypto/tls`, `net.(*Dialer)`, `net.Dial*`, `net.Listen*`, `net.(*Resolver)`, or any package from `os/exec`, `net/rpc`, `net/smtp` or `plugin`. Only the pure string helpers `net.ParseIP`, `net.SplitHostPort` and `net.JoinHostPort` are permitted, and only from `projectid`. Removing them is preferred.
- PRV-2. **No telemetry, analytics, crash reporting or update mechanism**, including in dependencies. The PostHog key and `go.kenn.io/kit` must be absent from the source tree and from `go.sum`.
- PRV-3. **No listening sockets** and no IPC (inter-process communication) endpoints.
- PRV-4. **Read-only access to agent data.** Session files are opened read-only and no-follow; SQLite databases use `mode=ro` (+`immutable=1` where upstream sets it) through a single helper. Source files are never written, renamed, locked for write, or deleted.
- PRV-5. **No subprocess execution** (EXT-3).
- PRV-6. **Writes are confined** to `--out <dir>`. No temp files elsewhere (EXT-5), no home-directory state files, no installation IDs, no logs on disk unless `--log-file` is given.
- PRV-7. **No environment-variable feature switches.** Behavior is controlled by flags only. `$HOME` and `$TMPDIR` are the only environment variables read; the latter is unnecessary once EXT-5 is done.
- PRV-8. **No embedded remote resources.** The tool has no UI. If one is added later, every asset is vendored and the CSP (Content Security Policy) is `default-src 'self'`.
- PRV-9. **Minimal outputs** (FR-24, FR-25).
- PRV-10. **Supply chain.**
  - The build uses vendored modules (`go mod vendor`), committed and reviewed.
  - CI (continuous integration) runs with `GOFLAGS=-mod=vendor` and `GOPROXY=off`.
  - Releases are built reproducibly (`-trimpath`, pinned toolchain) and signed internally.
  - There is no `curl | bash` installer.

---

## 7. Verification and acceptance

### 7.1 Static gates (CI, required on every change)

```bash
# V-1: forbidden packages absent from the dependency graph
go list -deps ./... | grep -E '^(net/http|crypto/tls|os/exec|net/rpc|net/smtp|plugin)$' && exit 1

# V-2: forbidden symbols absent from the linked binary
go tool nm ./avlocal | grep -E ' T (net/http|crypto/tls)\.| net\.\(\*Dialer\)| net\.Dial| net\.Listen| os/exec\.' && exit 1

# V-3: no forbidden modules
grep -E 'posthog|go\.kenn\.io/kit|minio|clickhouse|pgx|duckdb|aws-sdk' go.mod go.sum && exit 1

# V-4: allowlisted net usage only
grep -rnE '\bnet\.[A-Z]' --include='*.go' . | grep -vE 'net\.(ParseIP|SplitHostPort|JoinHostPort)\b' && exit 1

# V-5: no hardcoded external URLs in non-test code (allowlist documentation links explicitly)
```

### 7.2 Runtime gate

- V-6. Run the full golden corpus inside a network-less namespace (`unshare -rn`, or `sandbox-exec` with `(deny network*)` on macOS), under `strace -f -e trace=network,process`. Pass criteria:
  - zero `socket(AF_INET*)` and zero `connect()` other than `AF_UNIX`;
  - zero `execve` after start;
  - zero file writes outside `--out`.

### 7.3 Correctness gates

- V-7. The parser test suite passes: `CGO_ENABLED=1 go test -tags fts5 ./internal/...`. *Known baseline:* `TestParseCortexSession_SplitHistoryReadError` fails on pristine upstream `a22d9d1` in our environment, likely when run as root. Track it rather than block on it.
- V-8. **Golden parity with upstream.** For a corpus of real session logs collected on an isolated machine, totals must match upstream `agentsview usage daily --offline --all --breakdown --json --timezone UTC` exactly. The corpus covers at least Claude Code, Codex, Gemini, Copilot, Cursor and every agent actually used internally. Totals are per day × model, for input, output, cache-write, cache-read and web-search requests. Upstream must run with telemetry disabled and its network blocked. Record any intentional divergence (for example FR-13 or FR-17) in `PARITY.md`.
- V-9. **Dedup fixtures.** Include at minimum:
  - Claude streaming snapshots (same id and request, growing output);
  - a resumed session replaying prior turns;
  - a forked session;
  - subagent transcripts;
  - out-of-order discovery;
  - a session-level event overlapping message facts.

  *Reference result from the proof of concept:* on the streaming-plus-resume fixture, upstream and the extracted parser both produce 1,500 / 650 / 8,000 / 43,000 (input / output / cache-write / cache-read). On the same fixture, summing every usage line gives 4,700 input tokens (3.1×). Summing parsed messages without cross-session dedup gives 2,700 (+80%).
- V-10. **Determinism:** two runs with `--reproducible` produce byte-identical outputs.
- V-11. **Unpriced models:** a fixture with an unknown model must exit non-zero without `--allow-unpriced`.

---

## 8. Upstream synchronization

- SYNC-1. Record the upstream commit in `UPSTREAM` at the repo root.
- SYNC-2. Maintain the extraction as a scripted, idempotent transform plus a small patch queue, not as hand edits on a diverged tree. The script copies the §5.1 package set from an upstream checkout, applies §5.3, and rewrites import paths.
- SYNC-3. For each upstream update:
  - re-run the transform;
  - diff `go list -deps` against the previous baseline; any new import in the kept set needs explicit review;
  - run V-1 to V-11;
  - review upstream's changelog for parser and usage-accounting changes, and port any changes to the SQL dedup rules (§5.5.3) by hand.
- SYNC-4. Target cadence: monthly, or on demand when an agent format the company uses changes.

---

## 9. Deliverables and milestones

| Milestone | Scope | Exit criteria |
|---|---|---|
| **M1: Extraction** | §5.1 to §5.3 (EXT-1 stub permitted), new module, CLI emitting `requests.csv` without pricing | V-1 to V-4, V-6, V-7 |
| **M2: Accounting correctness** | FR-9 to FR-14 full dedup port, golden corpus | V-8, V-9 |
| **M3: Pricing and reports** | FR-15 to FR-25, `report.json` schema | V-10, V-11 |
| **M4: Hardening** | EXT-1 full removal, EXT-3, EXT-5, EXT-7; vendoring and signed reproducible builds; SYNC tooling | All gates; PRV-10 |

The proof of concept against `a22d9d1` already demonstrates the M1 boundary. It is attached separately as `0001-local-only-extraction-PoC-strip-S3-pricing-fetch-add.patch`. Its results:

- an 18.5 MB `usagecsv` binary;
- zero `net/http`, `crypto/tls` or dialer symbols;
- parser tests pass apart from the known baseline failure;
- totals on the V-9 fixture match upstream.

The proof of concept still uses the interim S3 stub and does not yet apply EXT-2 to EXT-5. It should be treated as a starting reference, not a deliverable.

---

## 10. Risks and open questions

| ID | Item | Mitigation / decision owner |
|---|---|---|
| R-1 | Upstream dedup lives in SQL, so a Go port can drift from it | V-8 golden parity on every sync; port tests from upstream `usage_rollup*_test.go` |
| R-2 | Some agents record no per-message tokens (`UsageCapabilities.NoPerMessageTokenData`) or bill in credits (`AICreditsDenominated`, for example Copilot) | Report these per agent in `diagnostics`; accounting decides treatment |
| R-3 | Cursor token usage upstream partly comes from the Cursor Admin API, which is removed | Measure local-only Cursor coverage on the corpus; accept the gap or obtain usage from Cursor billing separately |
| R-4 | Token counts are client-logged and can differ from provider invoices | Monthly reconciliation against provider billing; document the expected variance |
| R-5 | `mattn/go-sqlite3` requires CGO (cgo, Go's C interop) for database-backed agents (Cursor, Windsurf, Zed, Goose, OpenCode, …) | Accept, vendor and pin; optionally evaluate `modernc.org/sqlite` (pure Go) later |
| R-6 | Parser package size (~115k lines) limits full manual audit | Rely on mechanical gates (§7.1, §7.2) for the egress guarantee; audit diffs per sync |
| R-7 | Upstream churn (frequent releases, large parser changes) | Scripted transform (SYNC-2); pin to known-good baselines |
| R-8 | Project basenames may still reveal sensitive project names | `--project-names hash` with a locally kept mapping file if required |

---

## Appendix A: Source references (upstream `a22d9d1`)

- Telemetry: `internal/telemetry/telemetry.go`; endpoint `DefaultEndpoint = "https://us.i.posthog.com"` in `go.kenn.io/kit@v0.34.1/telemetry/posthog/posthog.go`.
- Pricing refresh job: `cmd/agentsview/pricing_schedule.go` (24 h, `RunAtStart: true`); fetchers in `internal/pricing/catalog/{litellm,genai,openrouter}.go`.
- S3 client: `internal/parser/s3source.go` (minio).
- Parser → pricing chain: `internal/parser/project.go` → `internal/export` → `internal/export/pricing.go` → `internal/pricing` → `internal/pricing/catalog` (`net/http`).
- Subprocess: `internal/parser/project.go` `runMountCommand` (`/sbin/mount`).
- Temp-file writes: `internal/parser/discovery_disk_map.go` (`os.CreateTemp("", "agentsview-discovery-map-*.db")`).
- Read-only SQLite helper: `internal/parser/sqlite_dsn.go`.
- Dedup rules: `internal/db/usage_rollup_classify.go`, `internal/db/usage_rollup*.go`, `internal/db/usage.go`.
- Provider interface: `internal/parser/provider.go` (`ProviderFactories`, `Provider.Discover`, `Provider.Parse`); agent definitions in `internal/parser/types.go`.
- Normalization: `internal/usagefacts/fact.go`.
