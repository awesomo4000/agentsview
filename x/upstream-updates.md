# Updating the minimization fork

Keep the original full-tree base in `UPSTREAM`. A selected patch does not mean
that the fork contains every intervening upstream change. Record each selected
commit below; never advance the base marker for a partial backport.

## Repeatable procedure

1. Start with a clean tree. Fetch `upstream`; inspect commits affecting the
   desired parsers, ingestion, SQLite, and their tests. Inspect each complete
   commit for schema, parser-version, fixture, and shared-code dependencies.
2. Apply selected diffs to the existing fork. Keep the full upstream commit ID
   in the ledger. Adapt shared-code changes deliberately; do not resolve a
   conflict by wholesale replacement with an upstream file.
3. Preserve the local-only transports, explicit-only external harness, fixed
   loopback port, SQLite FTS, and local annotations. Keep removed service
   packages out of the linked application. `x/fork-policy.json` owns the
   dependency denylist and runtime regression selection.
4. Give parser changes fork-local scoped data versions. Include all harnesses
   sharing changed logic. Prove unchanged session/source identities and archive
   preservation with synthetic upgrade tests. Do not copy upstream version
   numbers: identical integers can mean different migrations in this fork.
5. Update format provenance, the ledger, and the changelog. Run relevant parser,
   ingestion, SQLite preservation tests, `go fmt ./...`, and
   `go vet -p 1 -tags fts5 ./...`.
6. Run `python3 scripts/check-fork-policy.py`. It checks the actual linked Go
   dependency graph and runs refusal, loopback-client, port, and unconfigured
   insight tests. It fails when a selected test pattern matches no tests.
7. For UI or network-boundary changes, also run the synthetic local-privacy and
   local-FTS browser checks. Review new HTTP clients, subprocesses, scheduler
   jobs, dependencies, and frontend URLs; the gate is not an exhaustive security
   audit. Build the executable, scrub added content for private data, commit,
   and push only to the user fork.

The policy check is a rejection gate, not an automatic code-removal tool. New
upstream senders can use allowed standard-library packages. Semantic review and
runtime tests remain necessary. Do not merge all upstream code and assume a
package denylist removes every unwanted feature.

The `Fork policy` workflow runs this gate and collector/upgrade tests on pushes
and pull requests targeting `minimization`. It does not publish or auto-merge.

## Selected update on 11/12/2026

Reviewed upstream main `8264e3075117d094384185a5f533ce1653c5b123` against fork `2472c3608`. A simulated full
merge reported 562 conflicted paths, including 121 content conflicts. This update
instead backports the following selected changes and their parser regressions.

| Upstream commit | Retained behavior | Fork upgrade |
| --- | --- | --- |
| `e447c92167a4469cbfc8f0bc49621afce2dfb337` | Pi-family reasoning effort | 135 |
| `2f6b3b45c5fe3f0c01cf1fdb4b10483b1c57e110` | Pi header-parented subagents and self-parent prevention | 135 |
| `48659d435f8c3135d68c4aca64099963d4b1e419` | OMP path-valued parents and nested subagents | 136 |
| `a159d61e420a0a3dbb375214b3a3adcd26e98711` | Codex code-mode wait results | 137 |
| `472db3d9767f5be6f7c79098fe72b908f08c8779` | Codex incremental cell waits | No additional reparse |
| `9a5beba6a8b75de8c86f2c3f2e00f13f3b963403` | Claude command arguments containing less-than characters | 138 |

Pi-family changes also affect OMO, Prime Agent, and StepCode. Codex changes cover
TraeX and Augure Code. Claude command extraction covers Cowork, Qoder, ICodeMate,
Grok, and iFlow. Scoped upgrades leave unavailable recordings archived.

The backports retain the older fork's context plumbing. SQLite upgrade tests use
fork version intervals rather than upstream intervals. No remote backend, model
client, dependency update, or frontend change is imported.

## Pending review

The broader Claude deduplication change `907494117`, recorded-cost changes
`024ef2d37` and `8264e3075117d094384185a5f533ce1653c5b123`, and Codex reasoning/compaction change `b110db9b7`
remain pending. They cross accounting/schema boundaries and need their own
coherent dependency review; they are not included by the six backports above.

Do not open a fork database in upstream merely because its numeric parser
version is accepted. Fork and upstream version histories now differ.

## Validation for this batch

The full parser and ingestion suites passed. Focused SQLite upgrade tests and
sync preservation tests passed, including Pi effort persistence, OMP lineage,
session/source identity, missing-source retention, and preservation of local
tags, summaries, stars, and pins. Repository-wide Go formatting and vet passed.
The CLI was rebuilt using the retained embedded UI assets.

The fork policy command passed on the real dependency graph and all seven
runtime test selections. Disposable fake-toolchain checks also confirmed that
it rejects a forbidden dependency and fails when a test selection matches no
tests. All runtime fixtures were synthetic; no live archive was opened.

Frontend/browser tests were not rerun because this batch changes no frontend
or network-boundary implementation. The new CI workflow has not been observed
on GitHub; local checks are the validation reported here. This is a selected
backport batch, not a claim that the fork matches current upstream.
