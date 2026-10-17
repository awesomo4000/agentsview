package pricingrefresh

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/dbtest"
	"go.kenn.io/agentsview/internal/money"
	"go.kenn.io/agentsview/internal/pricing"
)

func TestSeedFallbackReseedsBandsWhenStorageVersionIsMissing(t *testing.T) {
	database := testDB(t)
	fallback := pricing.FallbackPricing()
	var gpt pricing.ModelPricing
	for _, model := range fallback {
		if model.ModelPattern == "gpt-5.5" {
			gpt = model
			break
		}
	}
	require.NotEmpty(t, gpt.Bands)
	require.NoError(t, database.UpsertModelPricing([]db.ModelPricing{{
		ModelPattern:         gpt.ModelPattern,
		InputPerMTok:         gpt.InputPerMTok,
		OutputPerMTok:        gpt.OutputPerMTok,
		CacheCreationPerMTok: gpt.CacheCreationPerMTok,
		CacheReadPerMTok:     gpt.CacheReadPerMTok,
	}}))
	require.NoError(t, database.SetPricingMeta(
		fallbackVersionMetaKey,
		pricing.FallbackVersion,
	))

	require.NoError(t, SeedFallback(database))
	got, err := database.GetModelPricing("gpt-5.5")
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.NotEmpty(t, got.Bands)
}
func TestStoreFallbackReconcilesOpenRouterOwnership(t *testing.T) {
	database := testDB(t)
	fallback := pricing.FallbackPricing()
	require.NotEmpty(t, fallback)
	exact := fallback[0].ModelPattern
	var spelled string
	for _, p := range fallback {
		if upper := strings.ToUpper(p.ModelPattern); upper != p.ModelPattern {
			spelled = upper
			break
		}
	}
	require.NotEmpty(t, spelled, "need a fallback pattern with lowercase")
	require.NoError(t, database.UpsertModelPricing([]db.ModelPricing{
		{ModelPattern: exact, InputPerMTok: money.MustParseDollars("99")},
		{ModelPattern: spelled, InputPerMTok: money.MustParseDollars("98")},
		{ModelPattern: "acme/only-openrouter"},
	}))
	require.NoError(t, database.SetPricingMeta(pricing.OpenRouterModelsMetaKey,
		pricing.EncodeOpenRouterModels([]string{exact, spelled, "acme/only-openrouter"})))

	require.NoError(t, SeedFallback(database))

	shadowed, err := database.GetModelPricing(spelled)
	require.NoError(t, err)
	assert.Nil(t, shadowed, "OpenRouter spelling of a seeded model retired")
	seeded, err := database.GetModelPricing(exact)
	require.NoError(t, err)
	require.NotNil(t, seeded)
	assert.Equal(t, fallback[0].InputPerMTok, seeded.InputPerMTok,
		"embedded rate wins on the exact pattern")
	meta, err := database.GetPricingMeta(pricing.OpenRouterModelsMetaKey)
	require.NoError(t, err)
	assert.Equal(t, `["acme/only-openrouter"]`, meta,
		"ownership of seeded and retired patterns transferred")
}
func TestSeedFallbackWithoutSentinelWritesNone(t *testing.T) {
	database := testDB(t)
	require.NoError(t, SeedFallback(database))
	meta, err := database.GetPricingMeta(pricing.OpenRouterModelsMetaKey)
	require.NoError(t, err)
	assert.Empty(t, meta)
}
func testDB(t *testing.T) *db.DB {
	t.Helper()
	return dbtest.OpenTestDBAt(
		t, filepath.Join(t.TempDir(), "sessions.db"),
	)
}

type blockedPricingTransport struct{ calls atomic.Int64 }

func (b *blockedPricingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	b.calls.Add(1)
	return nil, errors.New("network forbidden in local pricing test")
}

func TestEnsureCurrentUsesLocalRatesWithoutNetwork(t *testing.T) {
	database := testDB(t)
	transport := &blockedPricingTransport{}
	original := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = original })
	require.NoError(t, EnsureCurrent(t.Context(), database))
	bundled, err := database.GetModelPricing("gpt-5.5")
	require.NoError(t, err)
	require.NotNil(t, bundled)
	require.NoError(t, database.UpsertModelPricing([]db.ModelPricing{{
		ModelPattern: "internal-test-model", InputPerMTok: money.MustParseDollars("7"),
	}}))
	require.NoError(t, EnsureCurrent(t.Context(), database))
	local, err := database.GetModelPricing("internal-test-model")
	require.NoError(t, err)
	require.NotNil(t, local)
	assert.Equal(t, money.MustParseDollars("7"), local.InputPerMTok)
	assert.Zero(t, transport.calls.Load())
}

func TestEnsureCurrentHonorsCancellation(t *testing.T) {
	database := testDB(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, EnsureCurrent(ctx, database), context.Canceled)
	rows, err := database.ListModelPricing(t.Context())
	require.NoError(t, err)
	assert.Empty(t, rows)
}
