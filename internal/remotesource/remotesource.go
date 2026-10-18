// Package remotesource picks the transport for a configured remote host.
package remotesource

import (
	"errors"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/remotesync"
)

// Options describe the local collector.
type Options struct {
	// Version is the collector's build version.
	Version string
	// DataDir holds the downloaded release cache for SSH hosts.
	DataDir string
}

// For returns a new Source for one sync of rh. Callers name the host when
// they report an error, so the messages here leave it out.
func For(rh config.RemoteHost, opts Options) (remotesync.Source, error) {
	return nil, errors.New("remote HTTP/SSH sync has been removed from this fork")
}
