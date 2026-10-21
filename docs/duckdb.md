# DuckDB removal in the minimization fork

This fork uses SQLite for archive storage and analytics. DuckDB mirroring,
push/serve commands, and Quack connections are removed. Existing DuckDB files
are left untouched. The SQLite archive remains the source of conversation data.

A separate analysis tool can be considered if measured workloads need it.
