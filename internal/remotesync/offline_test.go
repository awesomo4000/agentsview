package remotesync

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProductionRemoteSourceNeverContactsPeer(t *testing.T) {
	var calls atomic.Int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(204) }))
	defer receiver.Close()
	source := HTTPSource{URL: receiver.URL, Token: "synthetic-secret", Client: receiver.Client()}
	_, err := source.Targets(t.Context())
	require.ErrorContains(t, err, "removed")
	_, _, err = source.Manifest(t.Context(), TargetSet{})
	require.ErrorContains(t, err, "removed")
	_, err = source.Archive(t.Context(), ArchiveRequest{})
	require.ErrorContains(t, err, "removed")
	require.Zero(t, calls.Load())
}
