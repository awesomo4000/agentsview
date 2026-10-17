package pricing

import (
	"go.kenn.io/agentsview/internal/pricing/catalog"
)

// ModelPricing holds per-model token pricing in cost per
// million tokens. Separate from db.ModelPricing — the CLI
// command converts between the two.
type ModelPricing = catalog.ModelPricing

// PricingBand is a complete rate tuple applied above an exclusive input-token
// threshold.
type PricingBand = catalog.PricingBand

// ParseLiteLLMPricing parses the LiteLLM JSON map into
// ModelPricing entries. Per-token costs are converted to
// per-million-token costs. Entries missing both input and
// output cost are skipped.
func ParseLiteLLMPricing(data []byte) ([]ModelPricing, error) {
	return catalog.ParseLiteLLMPricing(data)
}
