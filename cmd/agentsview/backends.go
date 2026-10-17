package main

import (
	"fmt"

	"go.kenn.io/agentsview/internal/duckdb"
	"go.kenn.io/agentsview/internal/server"
	"go.kenn.io/agentsview/internal/storage"
)

// No remote SQL replica is compiled into this fork. The shared registry remains
// for mirror orchestration; it registers no replica commands or HTTP routes.
var replicaBackends = []storage.Replica{}

// replicaBackendNamed returns the registered replica with the given CLI name.
func replicaBackendNamed(name string) (storage.Replica, error) {
	for _, backend := range replicaBackends {
		if backend.Name() == name {
			return backend, nil
		}
	}
	return nil, fmt.Errorf("no replica backend named %q is registered", name)
}

// mirrorBackend is the derived local mirror the daemon can rebuild.
var mirrorBackend storage.Mirror = duckdb.Mirror{}

// pushBackendOptions registers every push backend with a server so the
// daemon and the committed OpenAPI document expose the same push routes.
func pushBackendOptions() []server.Option {
	return []server.Option{
		server.WithReplicas(replicaBackends...),
		server.WithMirror(mirrorBackend),
	}
}
