package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"
)

var ErrSessionSummaryInvalid = errors.New("summary must be valid UTF-8 and at most 16000 bytes")

type SessionSummary struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
	UpdatedAt string `json:"updated_at"`
}

func (db *DB) GetSessionSummary(ctx context.Context, id string) (SessionSummary, error) {
	result := SessionSummary{SessionID: id}
	err := db.getReader().QueryRowContext(ctx, "SELECT content, updated_at FROM session_summaries WHERE session_id = ?", id).Scan(&result.Content, &result.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return result, err
}

// SetSessionSummary stores an operator-supplied annotation, never transcript or usage data.
func (db *DB) SetSessionSummary(ctx context.Context, id, content string) (SessionSummary, error) {
	if err := db.requireWritable(); err != nil {
		return SessionSummary{}, err
	}
	normalized, err := normalizeLabelSessionID(id)
	if err != nil {
		return SessionSummary{}, err
	}
	id = normalized
	content = strings.TrimSpace(content)
	if !utf8.ValidString(content) || len(content) > 16000 {
		return SessionSummary{}, ErrSessionSummaryInvalid
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if content == "" {
		_, err := db.getWriter().ExecContext(ctx, "DELETE FROM session_summaries WHERE session_id = ?", id)
		return SessionSummary{SessionID: id}, err
	}
	result := SessionSummary{SessionID: id, Content: content}
	err = db.getWriter().QueryRowContext(ctx, `INSERT INTO session_summaries(session_id, content) VALUES (?, ?)
 ON CONFLICT(session_id) DO UPDATE SET content=excluded.content, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
 RETURNING updated_at`, id, content).Scan(&result.UpdatedAt)
	return result, err
}
