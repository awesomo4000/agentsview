package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInsightsConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  InsightsConfig
		wantErr string
	}{
		{name: "absent"},
		{name: "loopback", wantErr: "removed", config: InsightsConfig{Endpoint: "http://127.0.0.1:30000/v1", Model: "local"}},
		{name: "https", wantErr: "removed", config: InsightsConfig{Endpoint: "https://models.example/v1", Model: "remote"}},
		{name: "remote http opt in", wantErr: "removed", config: InsightsConfig{Endpoint: "http://models.example/v1", Model: "remote", AllowHTTP: true}},
		{name: "partial endpoint", config: InsightsConfig{Endpoint: "http://127.0.0.1:30000/v1"}, wantErr: "removed"},
		{name: "partial model", config: InsightsConfig{Model: "local"}, wantErr: "removed"},
		{name: "userinfo", config: InsightsConfig{Endpoint: "https://user:secret@models.example/v1", Model: "remote"}, wantErr: "removed"},
		{name: "remote http", config: InsightsConfig{Endpoint: "http://models.example/v1", Model: "remote"}, wantErr: "removed"},
		{name: "default agent", config: InsightsConfig{DefaultAgent: "codex"}},
		{name: "default agent ignores case", config: InsightsConfig{DefaultAgent: " Codex "}},
		{name: "unknown default agent", config: InsightsConfig{DefaultAgent: "gpt"}, wantErr: "insight agent must be one of"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestInsightsConfigAPIKey(t *testing.T) {
	t.Setenv("AGENTSVIEW_TEST_INSIGHTS_KEY", "secret")
	c := InsightsConfig{APIKeyEnv: " AGENTSVIEW_TEST_INSIGHTS_KEY "}
	assert.Equal(t, "secret", c.APIKey())
	t.Setenv("AGENTSVIEW_TEST_INSIGHTS_KEY", "")
	assert.Empty(t, c.APIKey())
	assert.NotContains(t, os.Getenv("AGENTSVIEW_TEST_INSIGHTS_KEY"), "secret")
}

func TestInsightsConfigTOMLLoadAndFinalize(t *testing.T) {
	err := loadMinimalErrWithConfig(t, map[string]any{"insights": map[string]any{"endpoint": "http://127.0.0.1:11434/v1", "model": "test"}})
	require.ErrorContains(t, err, "removed")
	cfg := loadMinimalWithConfig(t, map[string]any{"insights": map[string]any{"default_agent": "external", "external": map[string]any{"id": "approved", "binary": "/tmp/approved-harness", "format": "text"}}})
	require.Equal(t, "external", cfg.Insights.DefaultAgent)
	require.Equal(t, "approved", cfg.Insights.External.ID)
}
