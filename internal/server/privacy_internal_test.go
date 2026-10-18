package server

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/config"
)

func TestRemovedActionsRefuseBeforeLaunching(t *testing.T) {
	srv := &Server{}
	_, err := srv.humaResumeSession(t.Context(), &resumeSessionInput{})
	require.Error(t, err)
	// Refusal happens before archive lookup or terminal discovery, even with no DB.
	require.Contains(t, err.Error(), "disabled")
	var calls atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(204) }))
	defer endpoint.Close()
	srv.cfg.Insights = config.InsightsConfig{Endpoint: endpoint.URL, Model: "test"}
	result, err := srv.defaultInsightGenerateStream(t.Context(), "claude", "synthetic sensitive prompt", nil)
	require.Error(t, err)
	require.Empty(t, result.Content)
	require.Zero(t, calls.Load())
	_, err = validateGithubTokenWithURL(t.Context(), endpoint.URL, "synthetic token")
	require.Error(t, err)
	require.Zero(t, calls.Load())
}
