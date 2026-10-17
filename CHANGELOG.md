# Changelog

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
