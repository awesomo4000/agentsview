package sync

import (
	"context"
	"go.kenn.io/agentsview/internal/db"
	"os"
)

func (e *Engine) shouldSkipFileWithPrefix(ctx context.Context,
	prefix, sessionID string, info os.FileInfo, sourceFingerprint ...string,
) bool {
	if e.forceParse {
		return false
	}
	fullID := applyIDPrefixToID(prefix, sessionID)
	storedSize, storedMtime, ok := e.db.GetSessionFileInfo(ctx,
		fullID,
	)
	if !ok {
		return false
	}
	if storedSize != info.Size() ||
		storedMtime != info.ModTime().UnixNano() {
		return false
	}
	if len(sourceFingerprint) > 0 && sourceFingerprint[0] != "" {
		storedHash, ok := e.db.GetSessionFileHash(ctx, fullID)
		if !ok || storedHash != sourceFingerprint[0] {
			return false
		}
	}
	if e.db.GetSessionDataVersion(ctx, fullID) <
		db.CurrentDataVersion() {
		return false
	}
	return true
}
