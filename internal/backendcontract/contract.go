// Package backendcontract centralizes the compile-time checks that every
// storage backend implements its role contract. A backend that omits a method
// fails `go build` here rather than at runtime.
//
// Roles are defined in internal/storage: the SQLite archive, remote replicas,
// and the derived DuckDB mirror. Every backend also implements db.Store, the
// server-facing read-and-curation surface.
package backendcontract

import (
	"go.kenn.io/agentsview/internal/db"
	duckdbstore "go.kenn.io/agentsview/internal/duckdb"
	"go.kenn.io/agentsview/internal/storage"
)

// Every backend serves the HTTP read-and-curation surface.
var (
	_ db.Store = (*db.DB)(nil)
	_ db.Store = (*duckdbstore.Store)(nil)
)

// Remote replicas: push target and read-only serve.

// Replicas that serve semantic search over pushed embeddings.

// Derived mirror: local rebuild and Quack serve.
var _ storage.Mirror = duckdbstore.Mirror{}

// The archive supplies push watermarks to replicas.
var _ storage.SyncStateStore = (*db.DB)(nil)
