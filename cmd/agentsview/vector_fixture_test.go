package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	kitvec "go.kenn.io/kit/vector"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/vector"
)

// enabledVectorConfig returns a minimal Config with [vector] enabled and a
// fresh DataDir, so the push source resolves a vectors.db path under a temp
// dir that no other test touches.
func enabledVectorConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		DataDir: t.TempDir(),
		Vector: config.VectorConfig{
			Enabled: true,
			Embeddings: config.VectorEmbeddingsConfig{
				Model:         "fake-model",
				Dimension:     4,
				MaxInputChars: 8192,
			},
		},
	}
}

// fakePushUnitSource is a vector.UnitSource that replays a fixed slice of
// units, enough to build a tiny active generation for the round-trip test.
type fakePushUnitSource struct {
	units []db.EmbeddableUnit
}

func (f fakePushUnitSource) ScanEmbeddableUnits(
	_ context.Context, _ string, _ bool, fn func(db.EmbeddableUnit) error,
) (string, error) {
	for _, u := range f.units {
		if err := fn(u); err != nil {
			return "", err
		}
	}
	return "2024-01-01T00:00:03Z", nil
}

// fakePushEncoder returns a deterministic 4-dimensional encoder matching the
// gen dimension the round-trip test builds with.
func fakePushEncoder() kitvec.EncodeFunc {
	return func(_ context.Context, texts []string) ([][]float32, error) {
		out := make([][]float32, len(texts))
		for i := range texts {
			out[i] = []float32{1, 0, 0, 0}
		}
		return out, nil
	}
}

// testPushUnitSource returns the fixed unit set the push-source tests build
// their vectors.db from: two sessions, three documents.
func testPushUnitSource() fakePushUnitSource {
	return fakePushUnitSource{units: []db.EmbeddableUnit{
		{SessionID: "session-1", Kind: "user", SourceUUID: "u1", Content: "hello"},
		{
			SessionID: "session-1", Kind: "run", SourceUUID: "a1",
			Ordinal: 1, OrdinalEnd: 2, Content: "first\n\nsecond",
			Offsets: []db.UnitOffset{{Ordinal: 1}, {Ordinal: 2, RuneStart: 7, ByteStart: 7}},
		},
		{SessionID: "session-2", Kind: "user", SourceUUID: "u2", Content: "world"},
	}}
}

// buildTestVectorsDB opens a read-write index at cfg's resolved vectors.db
// path, builds and activates one generation over two sessions, then closes
// it so the push source can reopen the file read-only.
func buildTestVectorsDB(t *testing.T, cfg config.Config) {
	t.Helper()

	ctx := t.Context()
	ix, err := vector.Open(
		ctx, cfg.Vector.ResolvedDBPath(cfg.DataDir), false,
		cfg.Vector.Embeddings.MaxInputChars,
	)
	require.NoError(t, err)
	defer ix.Close()

	gen := kitvec.Generation{Model: "fake-model", Dimensions: 4}
	result, err := ix.Build(
		ctx, testPushUnitSource(), fakePushEncoder(), gen, vector.BuildOptions{},
	)
	require.NoError(t, err)
	require.True(t, result.Activated)
}
