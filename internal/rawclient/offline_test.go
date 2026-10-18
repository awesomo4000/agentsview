package rawclient

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/parser"
)

func TestUploadCannotSendCredentialsOrObjects(t *testing.T) {
	var calls atomic.Int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(204) }))
	defer receiver.Close()
	client, err := NewClient(Config{BaseURL: receiver.URL, DeviceID: "synthetic-device", Credential: "synthetic-secret", HTTPClient: receiver.Client()})
	require.NoError(t, err)
	_, err = client.MissingObjects(t.Context(), parser.AgentClaude, nil)
	require.ErrorContains(t, err, "removed")
	require.Zero(t, calls.Load())
}
