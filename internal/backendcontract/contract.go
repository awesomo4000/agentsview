// Package backendcontract checks the SQLite archive against its service contracts.
package backendcontract

import (
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/storage"
)

var _ db.Store = (*db.DB)(nil)
var _ storage.SyncStateStore = (*db.DB)(nil)
