# Runtime network audit

The minimization fork keeps archive browsing, local harness discovery, SQLite
storage, source-log accounting, local exports, and the UI. It removes the
bundled paths that send conversations to remote services. This is application
code enforcement, not an operating-system network sandbox.

## Enforced boundaries

- Insight generation only accepts an explicitly configured external executable.
  Built-in vendor CLI fallbacks and embedded model HTTP clients are removed.
- Recall model extraction and embedding HTTP generation are disabled, including
  when an old configuration enables them. Recall background work only reconciles
  eligibility against the local archive. Saved recall data and vector storage
  remain available; new embeddings and semantic queries need a replacement encoder.
- SSH sync is removed. Remote HTTP archive sync and raw uploads refuse execution
  before opening a connection. Archive import and protocol bookkeeping remain.
- GitHub PR aggregation and server token discovery/validation are disabled.
  Session resume and external editor/terminal launch routes return forbidden.
- Browser API calls use the current origin. Saved remote-server settings are
  ignored. CSP restricts connections to that origin and blocks forms and frames.
  External transcript links lose their navigation target, and the application
  blocks external link clicks. Tauri refuses external URL handoff requests.
- CLI daemon clients accept literal loopback IP addresses only. They ignore proxy
  environment variables and refuse redirects, DNS names, and remote addresses.
- DuckDB retains local mirror queries but disables external access and automatic
  extension loading/installation. Quack remote connections are removed.

Old configuration fields and response types may remain for compatibility. They
cannot restore removed senders. HTTP fixtures retained in Go test files are not
compiled into the application.

## Explicit exceptions

An external harness configured by the operator inherits its environment and can
use credentials and networking. Generating an insight invokes that executable
with selected archive content. Configure only an approved private-data client.
There is no default executable or vendor fallback.

Explicit CLI command capture can run a user-supplied program, and an external
MCP client controls its own networking. The desktop documentation menu opens a
fixed public documentation URL without archive content. Build and development
tools can also use the network. These are not guarantees about other software
on the host or a compromised process.

The UI still uses a localhost listener, default port 47831. A high port is not
an access-control mechanism. This audit does not claim OS-enforced egress denial.

## Verification

Tests use synthetic data, disposable executables, and local receivers. Refusal
tests check that receivers see no model, embedding, upload, or sync requests.
Loopback-client tests check proxy and redirect refusal. DuckDB tests verify local
SQL still works while external reads fail. Browser checks cover a stale remote
setting, cross-origin POST, external link click, and local font loading.

Go package tests for server, insights, harness execution, extraction, vectors,
remote sync, raw clients, configuration, request signing, Git metadata, local
network clients, and DuckDB passed. Focused CLI checks, frontend checks and 181
unit tests, 103 desktop tests, both browser privacy/font checks, and repository-wide
Go vet passed. The full CLI suite still encounters
existing daemon-discovery/autostart fixture failures and is not reported green.
No live archive, installed binary, or running service was changed.
