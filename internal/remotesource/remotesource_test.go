package remotesource

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/config"
)

func TestRemoteSourcesAreUnavailable(t *testing.T) {
	for _, transport := range []string{"http", "ssh", ""} {
		source, err := For(config.RemoteHost{Transport: config.RemoteTransport(transport), Host: "synthetic", URL: "http://127.0.0.1:47831", Token: "synthetic"}, Options{})
		require.ErrorContains(t, err, "removed")
		require.Nil(t, source)
	}
}
