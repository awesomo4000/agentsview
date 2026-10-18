package vector

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.kenn.io/kit/embedconfig"
)

func TestEncoderCannotSendDocumentsOrQueries(t *testing.T) {
	var calls atomic.Int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(204) }))
	defer receiver.Close()
	for _, role := range []embedconfig.Role{embedconfig.RoleDocument, embedconfig.RoleQuery} {
		encode, err := NewEncoder(EncoderConfig{Deployment: embedconfig.Deployment{BaseURL: receiver.URL}}, role)
		require.ErrorContains(t, err, "removed")
		require.Nil(t, encode)
	}
	require.Zero(t, calls.Load())
}
