package main

import (
	"go.kenn.io/agentsview/internal/duckdb"
	"go.kenn.io/agentsview/internal/server"
	"go.kenn.io/agentsview/internal/storage"
)

var mirrorBackend storage.Mirror = duckdb.Mirror{}

func pushBackendOptions() []server.Option { return []server.Option{server.WithMirror(mirrorBackend)} }
