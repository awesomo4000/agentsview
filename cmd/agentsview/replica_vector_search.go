package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	kitvec "go.kenn.io/kit/vector"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/storage"
)

// replicaVectorUnavailableReason explains why the replica could not serve the
// local embeddings config: unavailable is the backend's own description of
// the miss (no matching generation, or one that is not fully written) and
// present lists the generations the replica does have, so an operator can
// tell a "wrong config" miss from a "never pushed" one.
func replicaVectorUnavailableReason(
	backend storage.Replica, unavailable string, present []storage.VectorGenerationInfo,
) string {
	fps := make([]string, 0, len(present))
	for _, g := range present {
		fps = append(fps, g.Fingerprint)
	}
	return fmt.Sprintf(
		"semantic search: %s (present: %s); run 'agentsview %s push' from a "+
			"machine with a matching [vector.embeddings] config",
		unavailable, strings.Join(fps, ", "), backend.Name())
}

// wireReplicaVectorSearch attaches semantic search to store when the local
// [vector.embeddings] config's fingerprint matches a generation already
// pushed to the replica. It is the shared startup gate for every replica
// server. `<backend> serve` treats a returned error as fatal.
// label prefixes the log lines with the calling server.
//
// A miss is never an error: a replica that cannot serve vectors, a
// fingerprint mismatch, or a generation that is not fully written each leave
// semantic search unavailable (a recorded reason surfaced through
// db.ErrSemanticUnavailable) rather than failing construction. It returns an
// error only for a genuinely unexpected query failure or an embeddings
// config the query encoder cannot be built from; the caller decides whether
// that is fatal.
//
// No per-query staleness gate is needed: a pushed generation is keyed by its
// immutable fingerprint, so a startup match cannot go stale while the process
// runs. Changing the local embeddings config changes the fingerprint, which
// requires restarting the serve (or re-running the CLI command), and that
// restart re-runs this gate.
func wireReplicaVectorSearch(
	ctx context.Context, appCfg config.Config, backend storage.Replica,
	store storage.ReplicaStore, label string,
) error {
	vectorStore, ok := store.(storage.VectorSearchStore)
	if !ok {
		return nil
	}
	provider, ok := backend.(storage.VectorSearchProvider)
	if !ok {
		vectorStore.SetSemanticUnavailableReason(fmt.Sprintf(
			"semantic search is not supported by the %s backend", backend.DisplayName()))
		return nil
	}
	if appCfg.ArchiveContent.UsageOnly() {
		vectorStore.SetSemanticUnavailableReason(
			"vector search is unavailable for usage-only archives")
		return nil
	}

	if !appCfg.Vector.Enabled {
		vectorStore.SetSemanticUnavailableReason(fmt.Sprintf(
			"semantic search: %s requires [vector] enabled with a matching "+
				"[vector.embeddings] config and a generation pushed by "+
				"'agentsview %s push'", backend.DisplayName(), backend.Name()))
		return nil
	}
	gen := vectorGeneration(appCfg.Vector.Embeddings)
	enc, err := newVectorQueryEncoder(appCfg.Vector.Embeddings, "")
	if err != nil {
		return fmt.Errorf("building query encoder: %w", err)
	}
	encodeQuery := func(ctx context.Context, text string) ([]float32, error) {
		return kitvec.EncodeOne(ctx, enc, text)
	}
	info := storage.VectorGenerationInfo{Fingerprint: gen.Fingerprint(), Model: gen.Model}
	searcher, unavailable, err := provider.OpenVectorSearcher(
		ctx, store, info, appCfg.Vector.Embeddings.MaxInputChars, encodeQuery)
	if err != nil {
		return err
	}
	if searcher == nil {
		present, err := provider.VectorGenerations(ctx, store)
		if err != nil {
			log.Printf("%s: listing vector generations: %v", label, err)
		}
		reason := replicaVectorUnavailableReason(backend, unavailable, present)
		vectorStore.SetSemanticUnavailableReason(reason)
		log.Printf("%s: %s", label, reason)
		return nil
	}
	vectorStore.SetVectorSearcher(searcher)
	log.Printf("%s: semantic search enabled (fingerprint %s, model %s)",
		label, info.Fingerprint, gen.Model)
	return nil
}
