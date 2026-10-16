//go:build pgtest

package postgres

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/parser"
	"go.kenn.io/agentsview/internal/rawsync"
	"testing"
	"time"
)

func rawCustodyObjectRef(t *testing.T, body []byte) rawsync.ObjectRef {
	t.Helper()
	digest := sha256.Sum256(body)
	object, err := rawsync.NewObjectRef(hex.EncodeToString(digest[:]), int64(len(body)))
	require.NoError(t, err)
	return object
}

func rawCustodyManifest(
	captureID string,
	parentReceipt string,
	capturedAt time.Time,
	objects ...rawsync.ObjectRef,
) rawsync.Manifest {
	var length int64
	for _, object := range objects {
		length += object.Length
	}
	return rawsync.Manifest{
		SchemaVersion:         rawsync.ManifestSchemaVersion,
		Provider:              parser.AgentCodex,
		ConfiguredRootID:      "root-a",
		SourceKey:             "sessions/demo.jsonl#main",
		ExpectedParentReceipt: parentReceipt,
		CaptureID:             captureID,
		CapturedAt:            capturedAt,
		Kind:                  rawsync.ManifestSnapshot,
		Entries: []rawsync.Entry{{
			Path: "session.jsonl", Type: "file", Length: length, Objects: objects,
		}},
	}
}
