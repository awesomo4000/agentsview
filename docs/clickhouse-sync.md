# ClickHouse removed from this fork

ClickHouse push, serve, and service commands are no longer available.
Remove `[clickhouse]`, `[clickhouse.NAME]`, `default_clickhouse`, and
`AGENTSVIEW_CLICKHOUSE_*` settings before starting the fork.
Existing SQLite archives are preserved. Use SQLite analytics or the retained
[DuckDB mirror](duckdb.md).

If an older build installed a ClickHouse watch service, stop and uninstall it
with that build before replacing it. This fork does not modify installed services
or delete remote data.
