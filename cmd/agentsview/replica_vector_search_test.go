package main

import (
	"go.kenn.io/agentsview/internal/clickhouse"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/storage"
)

func TestReplicaVectorReasons(t *testing.T) {
	backend := clickhouse.Backend{}
	assert.Equal(t,
		"semantic search: ClickHouse has no embedding generation matching fingerprint "+
			"abc123 (present: def456, ghi789); run 'agentsview clickhouse push' from a machine "+
			"with a matching [vector.embeddings] config",
		replicaVectorUnavailableReason(backend,
			"ClickHouse has no embedding generation matching fingerprint abc123",
			[]storage.VectorGenerationInfo{{Fingerprint: "def456"}, {Fingerprint: "ghi789"}}))
	assert.Equal(t,
		"semantic search: no match (present: ); run 'agentsview clickhouse push' from a "+
			"machine with a matching [vector.embeddings] config",
		replicaVectorUnavailableReason(backend, "no match", nil))
}

func TestWireReplicaVectorSearchRecordsVectorDisabledReason(t *testing.T) {
	store := &clickhouse.Store{}
	require.NoError(t, wireReplicaVectorSearch(
		t.Context(), config.Config{}, clickhouse.Backend{}, store, "clickhouse serve"))

	_, err := store.SearchContent(t.Context(), db.ContentSearchFilter{
		Pattern: "hello", Mode: "semantic",
	})
	require.ErrorIs(t, err, db.ErrSemanticUnavailable)
	assert.Contains(t, err.Error(), "ClickHouse requires [vector] enabled")
	assert.Contains(t, err.Error(), "agentsview clickhouse push")
	assert.NotContains(t, err.Error(), "agentsview embeddings build")
}

// nonVectorReplica is a replica that does not implement
// storage.VectorSearchProvider; only DisplayName is reachable from the gate.
type nonVectorReplica struct{ storage.Replica }

func (nonVectorReplica) DisplayName() string { return "Other" }

// TestWireReplicaVectorSearchRecordsUnsupportedBackend pins that a replica
// without a vector search provider leaves its store explaining the modes are
// unsupported, even when [vector] is enabled locally.
func TestWireReplicaVectorSearchRecordsUnsupportedBackend(t *testing.T) {
	store := &clickhouse.Store{}
	cfg := config.Config{}
	cfg.Vector.Enabled = true
	require.NoError(t, wireReplicaVectorSearch(
		t.Context(), cfg, nonVectorReplica{}, store, "other serve"))

	_, err := store.SearchContent(t.Context(), db.ContentSearchFilter{
		Pattern: "hello", Mode: "hybrid",
	})
	require.ErrorIs(t, err, db.ErrSemanticUnavailable)
	assert.Contains(t, err.Error(), "not supported by the Other backend")
}
