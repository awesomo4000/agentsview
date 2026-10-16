package sync

import (
	"go.kenn.io/agentsview/internal/parser"
	"path/filepath"
	"strings"
)

func applyIDPrefixToID(prefix, id string) string {
	if prefix == "" || id == "" || strings.HasPrefix(id, prefix) {
		return id
	}
	return prefix + id
}

func applyIDPrefixToIDs(prefix string, ids []string) []string {
	if prefix == "" || len(ids) == 0 {
		return ids
	}
	prefixed := make([]string, len(ids))
	for i, id := range ids {
		prefixed[i] = applyIDPrefixToID(prefix, id)
	}
	return prefixed
}

func applyIDPrefixToParsedResult(
	prefix string, result *parser.ParseResult,
) {
	if prefix == "" {
		return
	}
	result.Session.ID = applyIDPrefixToID(prefix, result.Session.ID)
	result.Session.ParentSessionID = applyIDPrefixToID(
		prefix, result.Session.ParentSessionID,
	)
	result.Session.SourceSessionID = applyIDPrefixToID(
		prefix, result.Session.SourceSessionID,
	)
	for i := range result.Messages {
		for j := range result.Messages[i].ToolCalls {
			call := &result.Messages[i].ToolCalls[j]
			call.SubagentSessionID = applyIDPrefixToID(
				prefix, call.SubagentSessionID,
			)
			for k := range call.ResultEvents {
				event := &call.ResultEvents[k]
				event.SubagentSessionID = applyIDPrefixToID(
					prefix, event.SubagentSessionID,
				)
			}
		}
	}
}

func localCodexSessionIndexPath(sessionPath string) string {
	dir := filepath.Dir(sessionPath)
	for dir != "" {
		base := filepath.Base(dir)
		if base == "sessions" || base == "archived_sessions" {
			return filepath.Join(
				filepath.Dir(dir), parser.CodexSessionIndexFilename,
			)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
