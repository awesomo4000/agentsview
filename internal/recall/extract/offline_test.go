package extract

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProductionClientNeverSendsTranscript(t *testing.T) {
	var calls atomic.Int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(204) }))
	defer receiver.Close()
	client := &Client{BaseURL: receiver.URL, HTTPClient: receiver.Client(), Model: "test", Request: RequestShape{MaxTokens: 100}}
	entries, usage, err := client.DistillWithRecovery(t.Context(), "synthetic system", "synthetic private transcript", 1)
	require.Error(t, err)
	require.Empty(t, entries)
	require.Equal(t, Usage{}, usage)
	require.Zero(t, calls.Load())
}
