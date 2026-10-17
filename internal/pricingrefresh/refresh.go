// Package pricingrefresh manages the SQLite model-pricing catalog lifecycle.
package pricingrefresh

import (
	"context"
	"slices"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/pricing"
)

const (
	fallbackVersionMetaKey = "_fallback_version"
	pricingStorageMetaKey  = "_pricing_storage_version"
	pricingStorageVersion  = "2"
)

// SeedFallback installs the embedded catalog (snapshot + supplemental
// aliases) when pricing.SeedVersion differs from the stored meta.
// On reseed it also deletes flat-rate rows for date-ambiguous Kimi
// aliases so they cannot shadow the date-based CanonicalModelForDate
// pricing path.
func SeedFallback(database *db.DB) error {
	return SeedFallbackContext(context.Background(), database)
}

// SeedFallbackContext is SeedFallback with cancellation checks between each
// catalog operation. It is intended for bounded, short-lived workflows.
func SeedFallbackContext(ctx context.Context, database *db.DB) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := seedEmbeddedGenAI(ctx, database); err != nil {
		return err
	}
	stored, err := database.GetPricingMetaContext(ctx, fallbackVersionMetaKey)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	storageVersion, err := database.GetPricingMetaContext(ctx, pricingStorageMetaKey)
	if err != nil {
		return err
	}
	if stored == pricing.SeedVersion && storageVersion == pricingStorageVersion {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := storeFallbackContext(ctx, database); err != nil {
		return err
	}
	// Only delete while reseeding (version mismatch). A later LiteLLM
	// refresh that legitimately lists one of these names is not
	// clobbered on every startup.
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := database.DeleteModelPricingContext(ctx,
		pricing.DateAliasedModels(),
	); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := database.SetPricingMetaContext(ctx,
		fallbackVersionMetaKey, pricing.SeedVersion,
	); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return database.SetPricingMetaContext(ctx, pricingStorageMetaKey, pricingStorageVersion)
}

// EnsureCurrent installs bundled rates when their version changes. It never fetches rates.
func EnsureCurrent(ctx context.Context, database *db.DB) error {
	return SeedFallbackContext(ctx, database)
}

func seedEmbeddedGenAI(ctx context.Context, database *db.DB) error {
	embedded := pricing.EmbeddedGenAIDocument()
	return database.InsertMissingGenAIPricing(ctx, db.GenAIPricingDocument{
		Version: embedded.Version, SourceRef: embedded.SourceRef,
		Source: db.GenAIPricingSourceEmbedded, Data: embedded.RawJSON(),
	})
}

// storeFallback writes the embedded catalog over the stored table. Like
// any non-OpenRouter source it outranks OpenRouter, so OpenRouter-owned
// rows it covers under another spelling are retired and rows it lists
// exactly pass to its ownership, in the same transaction, so a reseed
// never leaves an OpenRouter row beside or under an embedded one while
// the sentinel still claims it. Databases that never stored the sentinel
// keep none.
func storeFallback(database *db.DB) error {
	return storeFallbackContext(context.Background(), database)
}

func storeFallbackContext(ctx context.Context, database *db.DB) error {
	value, err := database.GetPricingMetaContext(
		ctx, pricing.OpenRouterModelsMetaKey,
	)
	if err != nil {
		return err
	}
	previous, err := pricing.DecodeOpenRouterModels(value)
	if err != nil {
		return err
	}
	fallback := pricing.FallbackPricing()
	patterns := make([]string, len(fallback))
	for i, p := range fallback {
		patterns[i] = p.ModelPattern
	}
	retired := pricing.ShadowedPatterns(patterns, previous)
	var owned []string
	for _, pattern := range previous {
		if !slices.Contains(patterns, pattern) &&
			!slices.Contains(retired, pattern) {
			owned = append(owned, pattern)
		}
	}
	var meta db.PricingMeta
	if value != "" {
		meta = db.PricingMeta{
			Key:   pricing.OpenRouterModelsMetaKey,
			Value: pricing.EncodeOpenRouterModels(owned),
		}
	}
	return database.ReconcileModelPricingContext(
		ctx, dbModelPricing(fallback), retired, meta,
	)
}

func dbModelPricing(prices []pricing.ModelPricing) []db.ModelPricing {
	dbPrices := make([]db.ModelPricing, len(prices))
	for i, price := range prices {
		bands := make([]db.PricingBand, len(price.Bands))
		for j, band := range price.Bands {
			bands[j] = db.PricingBand{
				AboveInputTokens:       band.AboveInputTokens,
				InputPerMTok:           band.InputPerMTok,
				OutputPerMTok:          band.OutputPerMTok,
				CacheCreationPerMTok:   band.CacheCreationPerMTok,
				CacheCreation1hPerMTok: band.CacheCreation1hPerMTok,
				CacheReadPerMTok:       band.CacheReadPerMTok,
			}
		}
		dbPrices[i] = db.ModelPricing{
			ModelPattern:           price.ModelPattern,
			InputPerMTok:           price.InputPerMTok,
			OutputPerMTok:          price.OutputPerMTok,
			CacheCreationPerMTok:   price.CacheCreationPerMTok,
			CacheCreation1hPerMTok: price.CacheCreation1hPerMTok,
			CacheReadPerMTok:       price.CacheReadPerMTok,
			Bands:                  bands,
		}
	}
	return dbPrices
}
