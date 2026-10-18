package localnet

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalClientDoesNotForwardToRedirectOrProxy(t *testing.T) {
	var received atomic.Int32
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1); w.WriteHeader(204) }))
	defer trap.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", trap.URL+"/sensitive")
		w.WriteHeader(307)
	}))
	defer source.Close()
	proxy, _ := url.Parse(trap.URL)
	client, err := Client(source.URL, &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy)}})
	require.NoError(t, err)
	defer client.CloseIdleConnections()
	response, err := client.Get(source.URL)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, 307, response.StatusCode)
	require.Zero(t, received.Load())
	for _, destination := range []string{"https://example.invalid/sensitive", "http://localhost:47831/", "http://127.0.0.1.example.invalid/", "http://192.0.2.1/"} {
		_, err = client.Get(destination)
		require.ErrorIs(t, err, ErrRemote)
	}
	require.Zero(t, received.Load())
}
func TestLocalDaemonStillWorks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("local archive")) }))
	defer server.Close()
	client, err := Client(server.URL, nil)
	require.NoError(t, err)
	defer client.CloseIdleConnections()
	response, err := client.Get(server.URL)
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, "local archive", string(body))
}
