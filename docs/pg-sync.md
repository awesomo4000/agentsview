# PostgreSQL removed

This minimization fork no longer supports PostgreSQL replication, serving,
or direct reads. The `pg` commands and `--pg` flags are removed. Remove
`[pg]`, named `[pg.*]` targets, `default_pg`, and `AGENTSVIEW_PG_*` settings
from existing configurations before starting the fork.

The local SQLite archive and its conversation exports remain.
See [the removal checklist](../REMOVALS.md) for retained and pending features.
