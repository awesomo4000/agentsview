// Package harness runs explicitly configured executables and records invocation
// accounting independently of source-session accounting.
package harness

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Config struct {
	ID             string   `json:"id" toml:"id"`
	Binary         string   `json:"binary" toml:"binary"`
	Args           []string `json:"args" toml:"args"`
	Format         string   `json:"format" toml:"format"`
	ProviderID     string   `json:"provider_id" toml:"provider_id"`
	BackendID      string   `json:"backend_id" toml:"backend_id"`
	DeploymentID   string   `json:"deployment_id" toml:"deployment_id"`
	RequestedModel string   `json:"requested_model" toml:"requested_model"`
	HarnessVersion string   `json:"harness_version" toml:"harness_version"`
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.ID) == "" || !filepath.IsAbs(c.Binary) {
		return errors.New("external harness requires id and absolute binary path")
	}
	if c.Format != "" && c.Format != "json" && c.Format != "text" {
		return errors.New("external harness format must be json or text")
	}
	return nil
}

type Identity struct {
	Value  *string `json:"value"`
	Source string  `json:"source"`
}
type Usage struct {
	Source     string `json:"source"`
	Scope      string `json:"scope"`
	Coverage   string `json:"coverage"`
	Input      *int64 `json:"input_tokens"`
	Output     *int64 `json:"output_tokens"`
	CacheRead  *int64 `json:"cache_read_input_tokens"`
	CacheWrite *int64 `json:"cache_creation_input_tokens"`
	Reasoning  *int64 `json:"reasoning_tokens"`
	Total      *int64 `json:"total_tokens"`
	Semantics  struct {
		CacheRead  *bool `json:"cache_read_included_in_input"`
		CacheWrite *bool `json:"cache_creation_included_in_input"`
		Reasoning  *bool `json:"reasoning_included_in_output"`
	} `json:"semantics"`
}
type Envelope struct {
	Schema     string `json:"schema_version"`
	Result     string `json:"result"`
	Completion struct {
		Reason         string `json:"reason"`
		ProviderReason string `json:"provider_reason"`
	} `json:"completion"`
	Identity map[string]Identity `json:"identity"`
	Usage    *Usage              `json:"usage"`
	Cost     *struct {
		Amount   string `json:"amount"`
		Currency string `json:"currency"`
		Scope    string `json:"scope"`
		Source   string `json:"source"`
	} `json:"reported_cost"`
}

func decode(data []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(data, &e, json.RejectUnknownMembers(true)); err != nil {
		return Envelope{}, errors.New("invalid external result JSON")
	}
	if e.Schema != "agentsview.execution-result.v1" {
		return e, errors.New("unsupported external result schema")
	}
	for key, v := range e.Identity {
		switch key {
		case "harness_version", "provider_id", "backend_id", "deployment_id", "requested_model", "reported_model", "model_revision", "provider_request_id":
		default:
			return e, errors.New("unknown identity field")
		}
		if v.Source != "configured" && v.Source != "provider_response" && v.Source != "unknown" {
			return e, errors.New("invalid identity source")
		}
		if v.Source == "unknown" && v.Value != nil {
			return e, errors.New("unknown identity must be null")
		}
	}
	if e.Cost != nil {
		if !regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`).MatchString(e.Cost.Amount) || !regexp.MustCompile(`^[A-Z]{3}$`).MatchString(e.Cost.Currency) || e.Cost.Source != "provider_response" || (e.Cost.Scope != "invocation" && e.Cost.Scope != "provider_request") {
			return e, errors.New("invalid reported cost")
		}
	}
	if e.Usage != nil {
		if e.Usage.Source != "provider_response" || (e.Usage.Scope != "invocation" && e.Usage.Scope != "provider_request") || (e.Usage.Coverage != "complete" && e.Usage.Coverage != "partial" && e.Usage.Coverage != "unknown") {
			return e, errors.New("invalid usage provenance")
		}
		for _, v := range []*int64{e.Usage.Input, e.Usage.Output, e.Usage.CacheRead, e.Usage.CacheWrite, e.Usage.Reasoning, e.Usage.Total} {
			if v != nil && *v < 0 {
				return e, errors.New("negative token count")
			}
		}
	}
	if e.Completion.Reason != "stop" {
		return e, errors.New("external answer is not confirmed complete")
	}
	if strings.TrimSpace(e.Result) == "" {
		return e, errors.New("empty external answer")
	}
	return e, nil
}

type Record struct {
	Failure        string     `json:"failure,omitempty"`
	InsightID      *int64     `json:"insight_id"`
	ID             string     `json:"id"`
	HarnessID      string     `json:"harness_id"`
	ProviderID     string     `json:"provider_id"`
	BackendID      string     `json:"backend_id"`
	DeploymentID   string     `json:"deployment_id"`
	RequestedModel string     `json:"requested_model"`
	HarnessVersion string     `json:"harness_version"`
	PromptHash     string     `json:"prompt_sha256"`
	Started        time.Time  `json:"started_at"`
	Ended          *time.Time `json:"ended_at"`
	DurationMS     int64      `json:"duration_ms"`
	ExitCode       *int       `json:"exit_code"`
	Status         string     `json:"status"`
	Observation    *Envelope  `json:"observation"`
}

// Open creates a private ledger. The archive is never attached or mutated.
func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("_busy_timeout", "5000")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite3", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS executions (id TEXT PRIMARY KEY, record_json TEXT NOT NULL)`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
func save(db *sql.DB, r Record) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO executions(id,record_json) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET record_json=excluded.record_json`, r.ID, string(b))
	return err
}

type bounded struct {
	buffer   bytes.Buffer
	cancel   context.CancelFunc
	exceeded bool
}

func (b *bounded) Write(p []byte) (int, error) {
	if b.buffer.Len()+len(p) > 8*1024*1024 {
		b.exceeded = true
		b.cancel()
		return 0, errors.New("external stdout limit exceeded")
	}
	return b.buffer.Write(p)
}

// Run persists a started attempt before launching. Response readiness is not
// insight validation or archive persistence. A crash leaves an indeterminate
// running record. Credentials are inherited but never copied to the ledger.
func Run(ctx context.Context, c Config, prompt, path string) (Record, string, error) {
	var r Record
	if err := c.Validate(); err != nil {
		return r, "", err
	}
	db, err := Open(path)
	if err != nil {
		return r, "", err
	}
	defer db.Close()
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return r, "", err
	}
	hash := sha256.Sum256([]byte(prompt))
	start := time.Now()
	r = Record{ID: hex.EncodeToString(id), HarnessID: c.ID, ProviderID: c.ProviderID, BackendID: c.BackendID, DeploymentID: c.DeploymentID, RequestedModel: c.RequestedModel, HarnessVersion: c.HarnessVersion, PromptHash: hex.EncodeToString(hash[:]), Started: start.UTC(), Status: "running"}
	if err = save(db, r); err != nil {
		return r, "", err
	}
	runCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	out := &bounded{cancel: cancel}
	cmd := exec.CommandContext(runCtx, c.Binary, c.Args...)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	runErr := cmd.Run()
	end := time.Now().UTC()
	r.Ended = &end
	r.DurationMS = time.Since(start).Milliseconds()
	if cmd.ProcessState != nil {
		n := cmd.ProcessState.ExitCode()
		r.ExitCode = &n
	}
	var answer string
	var parseErr error
	if c.Format == "text" {
		answer = out.buffer.String()
		r.Status = "unverified_response"
		if strings.TrimSpace(answer) == "" {
			parseErr = errors.New("empty external answer")
		}
	} else {
		e, eerr := decode(out.buffer.Bytes())
		parseErr = eerr
		answer = e.Result
		e.Result = ""
		r.Observation = &e
		r.Status = "response_received"
	}
	if runErr != nil || parseErr != nil || out.exceeded || runCtx.Err() != nil {
		r.Status = "failed"
		switch {
		case out.exceeded:
			r.Failure = "stdout_limit"
		case runCtx.Err() != nil:
			r.Failure = "cancelled_or_deadline"
		case runErr != nil:
			r.Failure = "process_error"
		case parseErr != nil:
			r.Failure = parseErr.Error()
		}
		answer = ""
	}
	if err = save(db, r); err != nil {
		return r, "", fmt.Errorf("persist execution outcome: %w", err)
	}
	if r.Status == "failed" {
		return r, "", errors.New("external invocation failed or returned an incomplete/invalid answer; see execution ledger")
	}
	return r, answer, nil
}
