package main

import "go.kenn.io/agentsview/internal/postgres"

// pgReplica retains the ordinary replica until the remote-backend removal pass.
// Hosted raw uploads and their separate vault are no longer wired into serving.
type pgReplica struct {
	postgres.Backend
}
