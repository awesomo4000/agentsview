package vector

import (
	"errors"

	"go.kenn.io/kit/embedconfig"
	kitvec "go.kenn.io/kit/vector"
)

type EncoderConfig struct {
	Model      embedconfig.Model
	Roles      embedconfig.Roles
	Deployment embedconfig.Deployment
	Transport  embedconfig.Transport
	// APIKey is sent as a Bearer token when non-empty.
	APIKey string
	// OllamaMetalRecovery enables kit's Ollama Metal recovery for an Ollama
	// endpoint ending in /v1.
	OllamaMetalRecovery bool
	// MaxRetries is the maximum total attempts on retryable errors; values
	// <= 1 mean one attempt.
	MaxRetries int
	// RetryRateLimits keeps retrying HTTP 429 responses after MaxRetries is
	// spent, until the request succeeds or ctx is canceled. Long-running
	// document builds enable this; latency-sensitive query encoders do not.
	RetryRateLimits bool
}

// NewEncoder refuses remote inference. Local vector storage remains available.
func NewEncoder(cfg EncoderConfig, role embedconfig.Role) (kitvec.EncodeFunc, error) {
	return nil, errors.New("embedded embedding transport has been removed; semantic encoding is unavailable")
}
