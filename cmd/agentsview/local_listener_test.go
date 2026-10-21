package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/config"
)

func TestValidateServeConfigLoopbackOnly(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "::1", "localhost"} {
		require.NoError(t, validateServeConfig(config.Config{Host: host, Port: 47831}))
	}
	for _, host := range []string{"", "0.0.0.0", "::", "192.0.2.1", "example.invalid"} {
		require.ErrorContains(t, validateServeConfig(config.Config{Host: host, Port: 47831}), "loopback")
	}
	for _, port := range []int{-1, 65536} {
		require.ErrorContains(t, validateServeConfig(config.Config{Host: "127.0.0.1", Port: port}), "invalid port")
	}
	require.ErrorContains(t, validateServeConfig(config.Config{Host: "127.0.0.1", Port: 47831, PublicURL: "https://example.invalid"}), "public hosting")
}
