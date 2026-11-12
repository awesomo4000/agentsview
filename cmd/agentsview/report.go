package main

import (
	"bufio"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/cobra"
	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/stringutil"
)

type reportFlags struct {
	database, month, from, to, timezone, query, include, exclude, member string
	models, agents                                                       []string
}

func newReportCommand() *cobra.Command {
	var f reportFlags
	root := &cobra.Command{Use: "report", Short: "Curate local sessions and report monthly tokens as Markdown", GroupID: groupUsage, Args: cobra.NoArgs}
	root.Long = "Read the local SQLite archive without starting a server, syncing, or invoking a model.\nReports contain recorded token counts, not monetary estimates."
	flags := root.PersistentFlags()
	flags.StringVar(&f.database, "db", "", "Existing SQLite archive (defaults to the local sessions.db)")
	flags.StringVar(&f.month, "month", "", "Calendar month, YYYY-MM")
	flags.StringVar(&f.from, "from", "", "First calendar month, YYYY-MM (inclusive)")
	flags.StringVar(&f.to, "to", "", "Last calendar month, YYYY-MM (inclusive)")
	flags.StringVar(&f.timezone, "timezone", "UTC", "IANA timezone for usage timestamps and month boundaries")
	flags.StringVar(&f.query, "query", "", "SQLite FTS query across recorded messages and saved summaries")
	flags.StringArrayVar(&f.models, "model", nil, "Exact recorded model name; repeat to include several")
	flags.StringArrayVar(&f.agents, "agent", nil, "Exact harness name; repeat to include several")
	flags.StringVar(&f.include, "include", "", "File of exact session IDs to include, one per line; empty file selects none")
	flags.StringVar(&f.exclude, "exclude", "", "File of exact session IDs to exclude, one per line")
	flags.StringVar(&f.member, "member", "", "Optional member label supplied by the person preparing the report")
	var idsOnly bool
	sessions := &cobra.Command{Use: "sessions", Short: "List session IDs, recognition hints, models, and matching tokens", Args: cobra.NoArgs}
	sessions.Flags().BoolVar(&idsOnly, "ids-only", false, "Print only session IDs, one per line, for curation files")
	sessions.RunE = func(cmd *cobra.Command, _ []string) error { return runTokenReport(cmd, f, "sessions", "", idsOnly) }
	monthly := &cobra.Command{Use: "monthly", Short: "Group selected usage by month, model, backend, and harness", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return runTokenReport(cmd, f, "monthly", "", false) }}
	show := &cobra.Command{Use: "show <session-id>", Short: "Inspect one session and its monthly token breakdown", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return runTokenReport(cmd, f, "show", args[0], false) }}
	root.AddCommand(sessions, monthly, show)
	return root
}

func reportDateBounds(f reportFlags) (string, string, error) {
	if f.month != "" && (f.from != "" || f.to != "") {
		return "", "", fmt.Errorf("use --month or --from/--to, not both")
	}
	if f.month != "" {
		f.from, f.to = f.month, f.month
	}
	var from, to string
	for _, part := range []struct {
		value string
		end   bool
	}{{f.from, false}, {f.to, true}} {
		if part.value == "" {
			continue
		}
		t, err := time.Parse("2006-01", part.value)
		if err != nil {
			return "", "", fmt.Errorf("invalid month %q: use YYYY-MM", part.value)
		}
		if part.end {
			to = t.AddDate(0, 1, -1).Format("2006-01-02")
		} else {
			from = t.Format("2006-01-02")
		}
	}
	if from != "" && to != "" && from > to {
		return "", "", fmt.Errorf("--from must not follow --to")
	}
	return from, to, nil
}

func readReportIDs(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	ids := []string{}
	seen := map[string]bool{}
	scan := bufio.NewScanner(file)
	for scan.Scan() {
		id := strings.TrimSpace(scan.Text())
		if id == "" || strings.HasPrefix(id, "#") {
			continue
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if err := scan.Err(); err != nil {
		return nil, fmt.Errorf("reading session ID file: %w", err)
	}
	return ids, nil
}

func runTokenReport(cmd *cobra.Command, f reportFlags, kind, id string, idsOnly bool) error {
	from, to, err := reportDateBounds(f)
	if err != nil {
		return err
	}
	if _, err := time.LoadLocation(f.timezone); err != nil {
		return fmt.Errorf("invalid timezone: %w", err)
	}
	include, err := readReportIDs(f.include)
	if err != nil {
		return err
	}
	exclude, err := readReportIDs(f.exclude)
	if err != nil {
		return err
	}
	if id != "" {
		if f.include != "" {
			return fmt.Errorf("report show selects its argument; do not also use --include")
		}
		include = []string{id}
	}
	path := f.database
	if path == "" {
		dir, err := config.ResolveDataDir()
		if err != nil {
			return err
		}
		path = filepath.Join(dir, "sessions.db")
	}
	archive, err := db.OpenReadOnly(cmd.Context(), path)
	if err != nil {
		return fmt.Errorf("open report archive (collect with sync first): %w", err)
	}
	defer archive.Close()
	snapshot, err := archive.TokenReport(cmd.Context(), db.TokenReportOptions{From: from, To: to, Timezone: f.timezone, Query: f.query, IncludeIDs: include, ExcludeIDs: exclude, Models: f.models, Agents: f.agents})
	if err != nil {
		return err
	}
	if kind == "show" && len(snapshot.Sessions) == 0 {
		return fmt.Errorf("session has no match under these filters: %s", id)
	}
	if idsOnly {
		for _, s := range snapshot.Sessions {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), s.ID); err != nil {
				return err
			}
		}
		return nil
	}
	return renderTokenReport(cmd.OutOrStdout(), snapshot, f, kind, from, to)
}

// Escape table delimiters, Markdown syntax, HTML, and terminal control bytes.
func reportCell(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	value = html.EscapeString(strings.Join(strings.Fields(value), " "))
	value = strings.NewReplacer("\\", "\\\\", "|", "\\|", "`", "\\`", "[", "\\[", "]", "\\]", "*", "\\*", "_", "\\_").Replace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

func renderTokenReport(out io.Writer, snapshot db.TokenReportSnapshot, f reportFlags, kind, from, to string) error {
	var b strings.Builder
	heading := map[string]string{"sessions": "Session review", "monthly": "Monthly token report", "show": "Session detail"}[kind]
	fmt.Fprintf(&b, "# %s\n\n", heading)
	fmt.Fprintf(&b, "Source archive: %s.\n\n", reportCell(snapshot.ArchiveID))
	if f.member != "" {
		fmt.Fprintf(&b, "Member: %s\n\n", reportCell(f.member))
	}
	start, end := from, to
	if start == "" {
		start = "beginning of archive"
	}
	if end == "" {
		end = "end of archive"
	}
	fmt.Fprintf(&b, "Period: %s through %s. Timezone: %s.\n\n", reportCell(start), reportCell(end), reportCell(f.timezone))
	if f.query != "" {
		fmt.Fprintf(&b, "Keyword query: %s.\n\n", reportCell(f.query))
	}
	if len(f.models) > 0 {
		fmt.Fprintf(&b, "Model filter: %s.\n\n", reportCell(strings.Join(f.models, ", ")))
	}
	if len(f.agents) > 0 {
		fmt.Fprintf(&b, "Harness filter: %s.\n\n", reportCell(strings.Join(f.agents, ", ")))
	}
	fmt.Fprintf(&b, "Selected sessions: %d. Include list: %t. Exclude list: %t.\n\n", len(snapshot.Sessions), f.include != "" || kind == "show", f.exclude != "")
	byID := map[string]db.TokenReportSession{}
	bySession := map[string][]db.TokenReportUsage{}
	for _, s := range snapshot.Sessions {
		byID[s.ID] = s
	}
	for _, u := range snapshot.Usage {
		bySession[u.SessionID] = append(bySession[u.SessionID], u)
	}
	if kind == "sessions" {
		b.WriteString("| Session ID | Harness | Project hint | Title hint | Title source | Models | Recorded tokens |\n| --- | --- | --- | --- | --- | --- | ---: |\n")
		for _, s := range snapshot.Sessions {
			var total int64
			models := map[string]bool{}
			for _, u := range bySession[s.ID] {
				total += u.Input + u.Output + u.CacheRead + u.CacheWrite
				models[u.Model] = true
			}
			names := []string{}
			for name := range models {
				names = append(names, name)
			}
			slices.Sort(names)
			amount := "no matching usage"
			if len(bySession[s.ID]) > 0 {
				amount = fmt.Sprint(total)
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n", reportCell(s.ID), reportCell(s.Agent), reportCell(s.Project), reportCell(stringutil.TruncateRunes(s.Title, 100, "…")), reportCell(s.TitleSource), reportCell(strings.Join(names, ", ")), amount)
		}
		b.WriteString("\nTitles and projects are recognition hints, not verified workstream tags. First-prompt titles may contain private text.\n\n")
	} else {
		if kind == "show" {
			s := snapshot.Sessions[0]
			fmt.Fprintf(&b, "Session ID: %s\n\nHarness: %s. Project hint: %s.\n\nTitle (%s): %s\n\nParent: %s. Relationship: %s.\n\nRecorded start: %s. Recorded end: %s.\n\nSaved summary: %s\n\n", reportCell(s.ID), reportCell(s.Agent), reportCell(s.Project), reportCell(s.TitleSource), reportCell(s.Title), reportCell(s.ParentID), reportCell(s.Relationship), reportCell(s.StartedAt), reportCell(s.EndedAt), reportCell(s.Summary))
		}
		type key struct{ month, model, provider, agent string }
		type group struct {
			totals db.TokenReportUsage
			ids    map[string]bool
		}
		groups := map[key]*group{}
		for _, u := range snapshot.Usage {
			k := key{u.Month, u.Model, u.Provider, byID[u.SessionID].Agent}
			g := groups[k]
			if g == nil {
				g = &group{ids: map[string]bool{}}
				groups[k] = g
			}
			g.ids[u.SessionID] = true
			g.totals.Input += u.Input
			g.totals.Output += u.Output
			g.totals.CacheRead += u.CacheRead
			g.totals.CacheWrite += u.CacheWrite
			g.totals.Reasoning += u.Reasoning
		}
		keys := []key{}
		for k := range groups {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, func(a, c key) int {
			return strings.Compare(a.month+"\x00"+a.model+"\x00"+a.provider+"\x00"+a.agent, c.month+"\x00"+c.model+"\x00"+c.provider+"\x00"+c.agent)
		})
		b.WriteString("| Month | Model | Backend | Harness | Sessions | Input | Output | Cache read | Cache write | Total | Reasoning* |\n| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
		for _, k := range keys {
			g := groups[k]
			t := g.totals
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %d | %d | %d | %d | %d | %d | %d |\n", reportCell(k.month), reportCell(k.model), reportCell(k.provider), reportCell(k.agent), len(g.ids), t.Input, t.Output, t.CacheRead, t.CacheWrite, t.Input+t.Output+t.CacheRead+t.CacheWrite, t.Reasoning)
		}
		b.WriteString("\n*Reasoning is shown separately and is not added to the total, because providers may include it in output tokens.\n\n")
	}
	if kind == "monthly" {
		b.WriteString("## Selected sessions\n\n| Session ID | Harness | Matching usage |\n| --- | --- | --- |\n")
		for _, session := range snapshot.Sessions {
			status := "no matching usage records"
			if len(bySession[session.ID]) > 0 {
				status = "counted"
			}
			fmt.Fprintf(&b, "| %s | %s | %s |\n", reportCell(session.ID), reportCell(session.Agent), status)
		}
		b.WriteString("\n")
	}
	missing := 0
	for _, s := range snapshot.Sessions {
		if len(bySession[s.ID]) == 0 {
			missing++
		}
	}
	fmt.Fprintf(&b, "Coverage: %d selected sessions have no matching usage records. Missing usage is not zero spend. Unknown backend means the recording did not supply a normalized backend identity.\n\n", missing)
	if snapshot.UndatedRecords > 0 || snapshot.UnattributedRecords > 0 {
		fmt.Fprintf(&b, "Archive usage omitted: %d undated records; %d records without a session ID.\n\n", snapshot.UndatedRecords, snapshot.UnattributedRecords)
	}
	if snapshot.FallbackTimestampRecords > 0 {
		fmt.Fprintf(&b, "Timestamp coverage: %d selected records use the session start because their usage timestamp is missing.\n\n", snapshot.FallbackTimestampRecords)
	}
	b.WriteString("Totals sum available counters; zero does not certify complete telemetry. Counts use archived usage timestamps and existing deduplication rules. Session selection is exact: parent and child sessions must each be included. No cost estimate, model call, sync, or upload was performed.\n")
	_, err := io.WriteString(out, b.String())
	return err
}
