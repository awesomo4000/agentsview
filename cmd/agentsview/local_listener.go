package main

import (
	"fmt"
	"net"
	"strconv"

	"go.kenn.io/agentsview/internal/config"
)

func browserURL(cfg config.Config) string {
	return "http://" + net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
}
func validateServeConfig(cfg config.Config) error {
	if cfg.Host == "" || !isLoopbackHost(cfg.Host) {
		return fmt.Errorf("host %q: this fork only serves the UI on loopback", cfg.Host)
	}
	if cfg.Port < 0 || cfg.Port > 65535 {
		return fmt.Errorf("invalid port %d", cfg.Port)
	}
	if cfg.Proxy.Mode != "" || cfg.PublicURL != "" || len(cfg.PublicOrigins) > 0 {
		return fmt.Errorf("public hosting is not supported in this fork")
	}
	return nil
}
func isLoopbackHost(host string) bool {
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
