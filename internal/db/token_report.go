package db

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/activity"
)

// TokenReportOptions selects archived usage without changing the archive.
// A nil IncludeIDs selects all sessions; a non-nil empty slice selects none.
type TokenReportOptions struct {
	From, To, Timezone, Query              string
	IncludeIDs, ExcludeIDs, Models, Agents []string
}

type TokenReportSession struct {
	ID, Agent, Project, Title, TitleSource, Summary, ParentID, Relationship string
	StartedAt, EndedAt                                                      string
}

type TokenReportUsage struct {
	SessionID, Month, Model, Provider               string
	Input, Output, CacheRead, CacheWrite, Reasoning int64
	Records                                         int
}

// TokenReportSnapshot keeps local recognition hints separate from usage rows.
// Missing provider identity stays empty; callers must not infer it from a model.
type TokenReportSnapshot struct {
	ArchiveID                                                     string
	Sessions                                                      []TokenReportSession
	Usage                                                         []TokenReportUsage
	UndatedRecords, UnattributedRecords, FallbackTimestampRecords int
}

// TokenReport reuses the daily report's normalized sources and Claude snapshot
// arbitration. Filtering session IDs happens after snapshot attribution, so
// excluding an owning session never transfers its usage to a copied transcript.
func (db *DB) TokenReport(ctx context.Context, options TokenReportOptions) (TokenReportSnapshot, error) {
	var result TokenReportSnapshot
	loc, err := time.LoadLocation(options.Timezone)
	if options.Timezone == "" {
		loc, err = time.UTC, nil
	}
	if err != nil {
		return result, err
	}
	for _, bound := range []string{options.From, options.To} {
		if bound != "" {
			if _, err := time.Parse("2006-01-02", bound); err != nil {
				return result, fmt.Errorf("invalid report date: %w", err)
			}
		}
	}
	if options.From != "" && options.To != "" && options.From > options.To {
		return result, fmt.Errorf("report start must not follow end")
	}
	release, err := db.acquireBulkRead(ctx)
	if err != nil {
		return result, err
	}
	defer release()
	tx, err := db.getReader().BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT value FROM archive_metadata WHERE key = ?), '')`, archiveMetadataArchiveIDKey).Scan(&result.ArchiveID); err != nil {
		return result, err
	}
	include, exclude := tokenReportSet(options.IncludeIDs), tokenReportSet(options.ExcludeIDs)
	models, agents := tokenReportSet(options.Models), tokenReportSet(options.Agents)
	sessions := map[string]TokenReportSession{}
	known := map[string]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT s.id, s.agent, s.project,
 COALESCE(s.display_name,''), COALESCE(s.session_name,''), COALESCE(s.first_message,''),
 COALESCE(ss.content,''), COALESCE(s.parent_session_id,''), s.relationship_type,
 COALESCE(s.started_at,''), COALESCE(s.ended_at,''), s.deleted_at IS NOT NULL
 FROM sessions s LEFT JOIN session_summaries ss ON ss.session_id=s.id`)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var s TokenReportSession
		var display, native, first string
		var deleted bool
		if err := rows.Scan(&s.ID, &s.Agent, &s.Project, &display, &native, &first, &s.Summary, &s.ParentID, &s.Relationship, &s.StartedAt, &s.EndedAt, &deleted); err != nil {
			rows.Close()
			return result, err
		}
		known[s.ID] = true
		if deleted || (options.IncludeIDs != nil && !include[s.ID]) || exclude[s.ID] || (len(agents) > 0 && !agents[s.Agent]) {
			continue
		}
		switch {
		case display != "":
			s.Title, s.TitleSource = display, "saved name"
		case native != "":
			s.Title, s.TitleSource = native, "harness title"
		case first != "":
			s.Title, s.TitleSource = first, "first prompt"
		default:
			s.TitleSource = "unavailable"
		}
		sessions[s.ID] = s
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	for _, id := range append(slices.Clone(options.IncludeIDs), options.ExcludeIDs...) {
		if !known[id] {
			return result, fmt.Errorf("unknown session ID in selection: %s", id)
		}
	}
	if strings.TrimSpace(options.Query) != "" {
		matches, err := tx.QueryContext(ctx, `SELECT m.session_id FROM messages_fts JOIN messages m ON m.id=messages_fts.rowid WHERE messages_fts MATCH ?
 UNION SELECT s.session_id FROM session_summaries_fts JOIN session_summaries s ON s.rowid=session_summaries_fts.rowid WHERE session_summaries_fts MATCH ?`, options.Query, options.Query)
		if err != nil {
			return result, fmt.Errorf("keyword query (SQLite FTS syntax): %w", err)
		}
		found := map[string]bool{}
		for matches.Next() {
			var id string
			if err := matches.Scan(&id); err != nil {
				matches.Close()
				return result, err
			}
			found[id] = true
		}
		err = matches.Err()
		matches.Close()
		if err != nil {
			return result, err
		}
		for id := range sessions {
			if !found[id] {
				delete(sessions, id)
			}
		}
	}
	f := UsageFilter{From: options.From, To: options.To, Timezone: loc.String(), AutomatedScope: "all"}
	bounds := usageBoundsForFilter(f)
	// Cursor account-wide rows cannot be curated by session. Include them in the
	// source scan only to expose omitted unattributed records rather than assign
	// account totals to an arbitrary session.
	query, args := dailyUsageRowsSQLForBounds(f, bounds, db.hasCursorUsageTable(ctx))
	query, args = snapshotRankedDailyUsageRowsSQL(query, args, f, bounds)
	query = dailyUsageRowSelectFromSnapshotRowsWithMachine(query, false) + ` ORDER BY u.ts ASC, u.session_id ASC, COALESCE(u.message_ordinal,-1) ASC`
	rows, err = tx.QueryContext(ctx, query, args...)
	if err != nil {
		return result, err
	}
	seen := map[activity.UsageDedupToken]struct{}{}
	type key struct{ session, month, model, provider string }
	buckets := map[key]*TokenReportUsage{}
	used := map[string]bool{}
	for rows.Next() {
		r, err := scanDailyUsageRowWithMachine(rows, false)
		if err != nil {
			rows.Close()
			return result, err
		}
		date := tokenReportDate(r.ts, loc)
		if date == "" {
			result.UndatedRecords++
			continue
		}
		if options.From != "" && date < options.From || options.To != "" && date > options.To {
			continue
		}
		if r.sessionID == "" {
			result.UnattributedRecords++
			continue
		}
		if _, ok := sessions[r.sessionID]; !ok {
			continue
		}
		if len(models) > 0 && !models[r.model] {
			continue
		}
		if !activity.KeepSessionUsageRow(seen, r.UsageIdentity()) {
			continue
		}
		fact, ok := dailyUsageFact(r)
		if !ok || !fact.TokenEligible {
			continue
		}
		if fact.UsesSessionStart {
			result.FallbackTimestampRecords++
		}
		k := key{r.sessionID, date[:7], r.model, r.providerID}
		b := buckets[k]
		if b == nil {
			b = &TokenReportUsage{SessionID: r.sessionID, Month: k.month, Model: r.model, Provider: r.providerID}
			buckets[k] = b
		}
		b.Input += fact.InputTokens
		b.Output += fact.OutputTokens
		b.CacheRead += fact.CacheReadTokens
		b.CacheWrite += fact.CacheCreationTokens
		b.Reasoning += fact.ReasoningTokens
		b.Records++
		used[r.sessionID] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	for _, b := range buckets {
		result.Usage = append(result.Usage, *b)
	}
	slices.SortFunc(result.Usage, func(a, b TokenReportUsage) int {
		return strings.Compare(a.Month+"\x00"+a.SessionID+"\x00"+a.Model+"\x00"+a.Provider, b.Month+"\x00"+b.SessionID+"\x00"+b.Model+"\x00"+b.Provider)
	})
	for id, s := range sessions {
		// Sessions without matching token records remain visible for inspection,
		// unless a model filter requires positive evidence for that model.
		if !used[id] {
			if len(models) > 0 {
				continue
			}
			start, end := tokenReportDate(s.StartedAt, loc), tokenReportDate(s.EndedAt, loc)
			if end == "" {
				end = start
			}
			if options.From != "" && end < options.From || options.To != "" && start > options.To {
				continue
			}
		}
		result.Sessions = append(result.Sessions, s)
	}
	slices.SortFunc(result.Sessions, func(a, b TokenReportSession) int { return strings.Compare(a.ID, b.ID) })
	return result, tx.Commit()
}

func tokenReportSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, v := range values {
		result[v] = true
	}
	return result
}

func tokenReportDate(timestamp string, loc *time.Location) string {
	t, ok := LocalTime(timestamp, loc)
	if !ok {
		return ""
	}
	return t.Format("2006-01-02")
}
