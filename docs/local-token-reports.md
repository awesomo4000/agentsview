# Local token reports

Use `agentsview report` to review sessions and produce a monthly Markdown report
from the existing SQLite archive. It opens the database read-only. It does not
start a daemon, collect new recordings, invoke a model, or upload anything.

Reports count tokens, not money. Missing backend identity stays `unknown`.
The same model on two recorded backends appears on separate rows. Some parsers,
including the current Pi/OMP parser, do not populate backend identity; the report
cannot recover it from the model name alone.

## Review and curate

Collect first, using the normal local collection workflow. For a one-shot run
without a server, stop an existing daemon and run:

```sh
./agentsview daemon stop
AGENTSVIEW_NO_DAEMON=1 ./agentsview sync
```

Inspect the month, find a topic, and inspect an exact session ID from the output:

```sh
./agentsview report sessions --month 2026-11
./agentsview report sessions --month 2026-11 --query 'deployment OR authentication'
./agentsview report show SESSION_ID
```

The session list shows the harness, model names, project hint, and title source.
A title can be a saved name, a harness title, or the first prompt. Project labels
usually come from recorded working directories, but may have been assigned by a
user. These hints help identify sessions without requiring tags; they do not
prove membership in a team workstream. `show` includes the saved summary and
parent relationship, not the full transcript. Existing `session messages` can
retrieve the transcript when needed.

Keyword queries use SQLite FTS syntax across messages and saved summaries.
They select whole sessions. The report then counts all matching usage for those
sessions in the requested period, not only the messages containing the keyword.

Create a text file containing session IDs to omit:

```sh
./agentsview report sessions --month 2026-11 --query 'hobby OR personal' --ids-only > excluded.txt
```

Review that file before using it. IDs are exact, one per line. Blank lines and
lines beginning with `#` are ignored. Unknown IDs are errors. Exclusions win
when an ID is in both lists. To start with an explicit shortlist instead, use
`--include included.txt`. An empty include file selects nothing; omitting
`--include` selects all sessions matching the other filters.

You can edit or filter these files with ordinary shell tools. This does not
retag, hide, delete, or otherwise change the stored conversations.

## Produce the monthly report

```sh
./agentsview report monthly --month 2026-11 --exclude excluded.txt --member member-a > november.md
```

Compare several months or restrict the report to recorded model/harness names:

```sh
./agentsview report monthly --from 2026-09 --to 2026-11 --agent claude --agent codex
./agentsview report monthly --month 2026-11 --model MODEL_NAME --include included.txt
```

`--model` and `--agent` accept exact names and may be repeated. Models and
harnesses within their respective lists are ORed; different filters intersect.
With no month bounds the report includes all archived months. `--month` cannot
be combined with `--from` or `--to`. The default timezone is UTC; use the same
`--timezone` across team members if reporting local calendar months.

Monthly output contains the supplied member label, source archive ID, filters,
model/backend/harness totals, selected session IDs, and coverage notes. It omits
session titles, project names, summaries, and conversation text. The member label
is optional and is never inferred from the host or login. Session-review and
show output can contain private prompt text in their title hints; keep those
local unless you deliberately choose to share them.

Each member can prepare and review their own Markdown report. No central
collector or automatic team-report merger is included. Do not blindly add
reports with overlapping session IDs, copied archives, or overlapping periods.

## Counting rules and limits

- Month boundaries follow usage timestamps, not just session start dates.
  Where the existing usage reader falls back to session start, the report
  identifies those selected records. Invalid dates are not invented.
- Input, output, cache-read, and cache-write tokens are separate. Total is their
  sum. Reasoning tokens appear separately and are not added again, because
  providers can include them in output.
- Existing normalized usage-source precedence and Claude snapshot deduplication
  are reused. These reports inherit the accounting limitations of this fork;
  they are not independently reconciled provider billing statements.
- Session selection is exact. Including a parent does not implicitly include
  subagents. Inspect the relationships and select each desired session. Usage
  attributed to an excluded owner is not reassigned to a copied transcript.
- Sessions with no matching usage remain visible without a model filter.
  Missing counters or recordings do not mean zero consumption. A model filter
  requires matching usage evidence, so sessions with unknown models disappear.
- Account-wide usage without a session ID cannot be curated by session and is
  omitted. Omitted-record counts describe the query candidates, not proof of
  completeness across every source recording.
- Archive reads use a single SQLite read transaction. Reporting does not apply
  migrations; if the archive needs an upgrade, run the normal collector first.

Use `--db /path/to/sessions.db` for another local archive. The default is the
archive under `AGENTSVIEW_DATA_DIR`, or `~/.agentsview` when unset.
