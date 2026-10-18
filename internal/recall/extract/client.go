package extract

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/stringutil"
)

// ErrContextOverflow reports a prompt the server rejected as too large for
// the model context. The unit must be split before retrying; retrying the
// same text can never succeed.
var ErrContextOverflow = errors.New("unit text overflows the model context")

// ErrPersistentTruncation reports output the client cannot recover: the
// token budget truncated the unit's entries. Splitting the unit preserves
// every entry, so the caller splits until SplitFloorChars; a unit at the
// floor that still truncates needs a larger max_tokens, and the error says
// so. There is deliberately no retry that caps the entry count: a capped
// response looks complete while silently dropping entries.
var ErrPersistentTruncation = errors.New(
	"model output truncated at the token budget",
)

// ErrSplitBudgetExceeded reports a unit whose overflow recovery ran past the
// per-unit split budget: one oversized transcript message would otherwise
// drive an unbounded fan-out of model calls and accumulate every leaf's
// entries in memory. It is neither transient nor endpoint-scoped, so the
// session fails closed and waits out its backoff rather than aborting the
// pass or retrying the same oversized message every time.
var ErrSplitBudgetExceeded = errors.New(
	"unit overflow recovery exceeded the split budget",
)

// errTruncated is the per-request truncation signal that
// DistillWithRecovery converts into the typed split signal.
var errTruncated = errors.New("model output truncated")

// errPermanentRequest marks a server rejection that retrying the same
// request can never fix (wrong model name, bad credentials, malformed
// field).
var errPermanentRequest = errors.New("model server rejected the request")

// errProtocolViolation marks a 200 whose content violates the
// constrained-decoding contract: the server was asked to enforce
// json_schema and did not. That is an endpoint property, not a fact about
// the submitted transcript, so it classifies as endpoint-scoped.
var errProtocolViolation = errors.New(
	"distill response violates the extraction protocol",
)

// errClientOnlyResponseLimit marks a resource bound enforced only after the
// response arrives. Unlike a schema violation, it indicts one model output,
// not the endpoint: the manager fails that session behind its backoff and
// continues through the remaining candidates.
var errClientOnlyResponseLimit = errors.New(
	"distill response exceeds a client-only resource limit",
)

// errDisallowedEntryType marks an entry whose type is valid in general but
// was left out of this request's enum. Only some units narrow the enum, so a
// server that ignores it still answers unrestricted units correctly: like
// errClientOnlyResponseLimit, it indicts one model output, not the endpoint.
var errDisallowedEntryType = errors.New(
	"distill response uses an entry type this unit does not allow",
)

// requestStatusError carries the HTTP status of a permanent server
// rejection so callers can tell endpoint-scoped failures from
// input-specific ones.
type requestStatusError struct {
	status int
	err    error
}

func (e *requestStatusError) Error() string { return e.err.Error() }
func (e *requestStatusError) Unwrap() error { return e.err }

// endpointScopedRejection reports whether err is a permanent failure that
// indicts the endpoint or configuration rather than the submitted unit:
// authentication (401), authorization (403), unknown route or model (404),
// wrong method or media type on the route (405, 415), an unimplemented
// route (501), and schema-violating 200s all fail every request equally,
// so visiting further sessions burns one doomed model call and one failure
// backoff apiece. A 400 stays input-scoped — the same endpoint answers
// other units fine when only this request's content is refused.
func endpointScopedRejection(err error) bool {
	if errors.Is(err, errProtocolViolation) ||
		errors.Is(err, errRedirectRefused) {
		return true
	}
	rejection, hasRejection := errors.AsType[*requestStatusError](err)
	if !hasRejection {
		return false
	}
	switch rejection.status {
	case http.StatusUnauthorized, http.StatusForbidden,
		http.StatusNotFound, http.StatusMethodNotAllowed,
		http.StatusUnsupportedMediaType, http.StatusNotImplemented:
		return true
	}
	return false
}

// errRedirectRefused marks a refused redirect. Deterministic: the same
// endpoint redirects every identical request, so it is endpoint-scoped.
var errRedirectRefused = errors.New("extraction requests do not follow redirects")

// RefuseRedirects is the CheckRedirect policy for extraction HTTP clients:
// redirects are never followed. A 307/308 replays the extraction POST —
// transcript content included — to whatever destination the endpoint
// names, letting a compromised endpoint exfiltrate the request or aim it
// at loopback services that trust local callers. A name-based same-origin
// allowance would not close this: the redirect target is re-resolved, so a
// rebinding hostname passes any string comparison while the connection
// lands elsewhere. Endpoints must be configured with their final URL.
func RefuseRedirects(req *http.Request, _ []*http.Request) error {
	// The target is redacted: a redirect can name a URL carrying
	// credentials or signed tokens, and this message reaches stderr and
	// stored failure rows.
	return fmt.Errorf(
		"%w: refusing redirect to %q; configure the endpoint's final URL",
		errRedirectRefused,
		boundedToken(config.RedactedEndpoint(req.URL.String()), 200))
}

// transientError marks a failure worth retrying: network errors, timeouts,
// rate limits, and server errors. RetryAfter carries the server's requested
// delay, zero when it gave none.
type transientError struct {
	err        error
	retryAfter time.Duration
}

func (e *transientError) Error() string { return e.err.Error() }
func (e *transientError) Unwrap() error { return e.err }

// reservedRequestKeys are payload fields the client owns. ExtraBody must
// not shadow them: a profile smuggling max_tokens or response_format
// through the extra body would bypass validation and desynchronize the
// generation fingerprint from the request actually sent.
var reservedRequestKeys = []string{
	"model", "messages", "max_tokens", "temperature", "response_format",
}

// maxRetryDelay caps the wait between transient retries, whether from
// backoff growth or an excessive Retry-After header.
const maxRetryDelay = 30 * time.Second

// extractionProtocolVersion feeds the generation fingerprint. Bump it when
// the response schema or the recovery behavior changes in a way that alters
// extraction output for an identical configuration.
// v2: minLength constraints on entry title and body.
// v3: truncation always splits; the entry-capped compact retry is gone.
// v4: maxItems/maxLength bounds on entries, fields, and entities.
// v5: body maxLength is enforced client-side only for grammar compatibility.
// v6: action units whose messages ran no tool drop 'procedure' and open
// with unexecutedActionPreamble; units carry that tool evidence.
const extractionProtocolVersion = 6

// Local resource bounds on a single distill response. The transport cap
// only bounds bytes; within it a compromised or misconfigured endpoint
// could return tens of thousands of entries or multi-megabyte fields, and
// accepting them would balloon the archive (and its FTS index) and hold
// the write lock through the inserts. entrySchema declares every limit except
// the body maxLength: some JSON-schema grammar compilers expand a 5000-character
// string bound into a grammar too large to parse. The transport and client-side
// checks still bound bodies safely. Lengths count Unicode code points to match
// JSON Schema maxLength semantics where the schema carries the constraint.
const (
	maxResponseBodyBytes = 16 << 20
	maxResponseEntries   = 100
	maxEntryTitleChars   = 500
	maxEntryBodyChars    = 5000
	maxEntryEntities     = 50
	maxEntityChars       = 200
)

// Entry is one distilled memory entry as the model produces it.
type Entry struct {
	Type     string   `json:"type"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Entities []string `json:"entities"`
}

// Usage reports token consumption for one model call.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// entryTypes is the closed set of entry kinds, shared by the request
// schema and the client-side validation of what actually came back.
var entryTypes = []string{
	"fact", "decision", "procedure", "warning", "preference", "open_question",
}

// entrySchema constrains decoding so the model can only produce parseable
// entries of the given types; validation failures become server-side
// sampling constraints instead of client-side parse errors.
func entrySchema(types []string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"entries": map[string]any{
				"type":     "array",
				"maxItems": maxResponseEntries,
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"type": map[string]any{
							"type": "string",
							"enum": types,
						},
						"title": map[string]any{
							"type": "string", "minLength": 1,
							"maxLength": maxEntryTitleChars,
						},
						"body": map[string]any{
							"type": "string", "minLength": 1,
						},
						"entities": map[string]any{
							"type":     "array",
							"maxItems": maxEntryEntities,
							"items": map[string]any{
								"type":      "string",
								"maxLength": maxEntityChars,
							},
						},
					},
					"required":             []string{"type", "title", "body", "entities"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"entries"},
		"additionalProperties": false,
	}
}

// Client retains local extraction validation and bookkeeping. The embedded
// network implementation is removed. Endpoint fields remain for recognizing
// old configuration and for test-only source fixtures.
type Client struct {
	// distiller is an in-process boundary used by archive bookkeeping tests.
	// Production has no implementation until an approved harness is wired.
	distiller func(context.Context, string, string, []string) ([]Entry, Usage, error)
	BaseURL   string
	// APIKey is sent as a bearer token when non-empty. Keep credentials
	// outside BaseURL so redacted endpoint logs stay useful.
	APIKey string
	Model  string
	// RetryBackoff seeds the exponential wait between transient retries;
	// zero means 500ms. It shapes latency, not output, so it stays outside
	// RequestShape and the fingerprint.
	RetryBackoff time.Duration
	HTTPClient   *http.Client
	Request      RequestShape
}

func (c *Client) retryBackoff() time.Duration {
	if c.RetryBackoff > 0 {
		return c.RetryBackoff
	}
	return 500 * time.Millisecond
}

// ValidateRequestShape rejects a request configuration that would fail
// every model call identically. Manager construction calls it so a bad
// profile fails daemon or CLI setup outright, before any progress rows are
// created; DistillWithRecovery re-checks as a backstop for direct callers.
func (c *Client) ValidateRequestShape() error {
	if c.Request.MaxTokens <= 0 {
		return fmt.Errorf(
			"extraction request max_tokens must be positive; the profile or "+
				"configuration must set it (got %d)", c.Request.MaxTokens,
		)
	}
	for _, key := range reservedRequestKeys {
		if _, ok := c.Request.ExtraBody[key]; ok {
			return fmt.Errorf(
				"extra body must not set reserved request field %q; use the "+
					"dedicated configuration for it", key,
			)
		}
	}
	return nil
}

// DistillWithRecovery runs one unit through the model with the recovery
// ladder: transient failures (network errors, timeouts, rate limits,
// server errors) are retried up to maxAttempts with exponential backoff
// honoring Retry-After, while permanent rejections fail fast; truncated
// output surfaces as ErrPersistentTruncation and a context overflow as
// ErrContextOverflow — both mean "split this unit", which the caller owns
// because it also owns unit identity. The returned Usage sums every
// attempt, successful or not, so recovery costs are accounted.
func (c *Client) DistillWithRecovery(
	ctx context.Context, systemPrompt, text string, maxAttempts int,
) ([]Entry, Usage, error) {
	return c.distillWithRecovery(ctx, systemPrompt, text, entryTypes, maxAttempts)
}

// distillWithRecovery is DistillWithRecovery with the response restricted
// to types, a subset of entryTypes.
func (c *Client) distillWithRecovery(
	ctx context.Context, systemPrompt, text string, types []string,
	maxAttempts int,
) ([]Entry, Usage, error) {
	var total Usage
	if err := c.ValidateRequestShape(); err != nil {
		return nil, total, err
	}
	var lastErr error
	for attempt := range maxAttempts {
		entries, usage, err := c.distill(ctx, systemPrompt, text, types)
		total.PromptTokens += usage.PromptTokens
		total.CompletionTokens += usage.CompletionTokens
		if err == nil {
			return entries, total, nil
		}
		if errors.Is(err, errTruncated) {
			// Rune count, not byte length: split budgets count code points.
			return nil, total, fmt.Errorf(
				"unit of %d chars at max_tokens=%d (split the unit, or raise "+
					"max_tokens if it is already at the split floor): %w",
				utf8.RuneCountInString(text), c.Request.MaxTokens,
				ErrPersistentTruncation,
			)
		}
		transient, hasTransient := errors.AsType[*transientError](err)
		if !hasTransient {
			return nil, total, err
		}
		lastErr = err
		if attempt+1 >= maxAttempts {
			break
		}
		delay := transient.retryAfter
		if delay <= 0 {
			delay = c.retryBackoff() << attempt
		}
		if delay > maxRetryDelay {
			delay = maxRetryDelay
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, total, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, total, fmt.Errorf(
		"distilling unit after %d attempts: %w", maxAttempts, lastErr,
	)
}

// credentialedEndpoint reports whether the configured request carries
// credential material: a bearer token, URL userinfo, or any raw query
// segment whose key is not the api-version surface selector (mirroring
// the config redactor's fail-closed allowlist). Raw wire segments, no parser:
// url.ParseQuery would reject exactly the malformed queries that still
// travel verbatim, and a rejection must not fail open. An unparseable URL
// counts as credentialed for the same reason.

// safeEndpointPathSegments are the only path segments that keep
// endpoint-provided diagnostics flowing: names that select the API
// surface, never credentials. The empty string covers leading, trailing,
// and doubled slashes.
var safeEndpointPathSegments = map[string]bool{
	"":       true,
	"v1":     true,
	"v1beta": true,
	"api":    true,
	"openai": true,
}

// detailWithheld replaces every endpoint-derived detail in error text when
// the endpoint URL carries credential material: a server that knows the
// credential can reflect it through any response field — body,
// finish_reason, a schema-violating key — in re-encodings no replacement
// list can enumerate, and these errors reach persisted failure rows,
// scheduler logs, doctor stderr, and CI logs.
const detailWithheld = "(withheld: endpoint URL carries credential material)"

// responseDetail prepares a response-body excerpt for error text. Bodies
// are attacker-influenced; for credentialed endpoints they are withheld
// outright, and otherwise kept as a control-stripped, length-capped
// excerpt, which is where the diagnostic value lives (wrong model name,
// malformed field).

// transportErrorDetail prepares a transport-layer error's text for error
// messages. Context errors pass through: their text is fixed by the
// runtime and says why the request ended (canceled, timed out). Anything
// else can quote bytes the server chose, so it is withheld for
// credentialed endpoints and control-stripped and length-bounded
// otherwise.

// responseToken prepares a short endpoint-supplied token (a finish_reason)
// for error text: withheld for credentialed endpoints, bounded otherwise.

// stripControls replaces C0/C1 control characters and invalid UTF-8 with
// spaces: response bytes reach terminals and logs, where an escape
// sequence or carriage return forges or hides output.
func stripControls(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F || (r >= 0x80 && r <= 0x9F) ||
			r == utf8.RuneError {
			return ' '
		}
		return r
	}, s)
}

// boundedToken caps an endpoint-supplied token for error text: a hostile
// finish_reason or entry field can approach the transport limit, and these
// errors persist into per-session failure rows.
func boundedToken(value string, maxRunes int) string {
	return stringutil.TruncateRunes(stripControls(value), maxRunes, "…(truncated)")
}

// parseEntries decodes and validates distilled content against the same
// constraints entrySchema requests: an entries array must be present,
// keys are matched exactly (Go's struct decoding is case-insensitive, so
// this walks raw messages instead), unknown keys, nulls, and trailing data
// are rejected, and every entry needs a known type, a non-blank title and
// body, and an entities array of strings.
func parseEntries(content string, types []string) ([]Entry, error) {
	top, err := strictObject(jsontext.Value(content), []string{"entries"})
	if err != nil {
		return nil, err
	}
	rawEntries, err := strictArray(top["entries"], "entries")
	if err != nil {
		return nil, err
	}
	if len(rawEntries) > maxResponseEntries {
		return nil, fmt.Errorf(
			"response carries %d entries, limit %d",
			len(rawEntries), maxResponseEntries,
		)
	}
	entryKeys := []string{"type", "title", "body", "entities"}
	entries := make([]Entry, 0, len(rawEntries))
	for i, rawEntry := range rawEntries {
		fields, err := strictObject(rawEntry, entryKeys)
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		var entry Entry
		if entry.Type, err = strictString(fields["type"], "type"); err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		if !slices.Contains(entryTypes, entry.Type) {
			return nil, fmt.Errorf(
				"entry %d: type %q is not one of %s",
				i, boundedToken(entry.Type, 60),
				strings.Join(entryTypes, ", "),
			)
		}
		if !slices.Contains(types, entry.Type) {
			return nil, fmt.Errorf(
				"%w: entry %d has type %q; allowed: %s",
				errDisallowedEntryType, i, entry.Type,
				strings.Join(types, ", "),
			)
		}
		if entry.Title, err = strictString(fields["title"], "title"); err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		if strings.TrimSpace(entry.Title) == "" {
			return nil, fmt.Errorf("entry %d: title is blank", i)
		}
		if n := utf8.RuneCountInString(entry.Title); n > maxEntryTitleChars {
			return nil, fmt.Errorf(
				"entry %d: title is %d characters, limit %d",
				i, n, maxEntryTitleChars,
			)
		}
		if entry.Body, err = strictString(fields["body"], "body"); err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		if strings.TrimSpace(entry.Body) == "" {
			return nil, fmt.Errorf("entry %d: body is blank", i)
		}
		if n := utf8.RuneCountInString(entry.Body); n > maxEntryBodyChars {
			return nil, fmt.Errorf(
				"%w: entry %d body is %d characters, limit %d",
				errClientOnlyResponseLimit,
				i, n, maxEntryBodyChars,
			)
		}
		rawEntities, err := strictArray(fields["entities"], "entities")
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		if len(rawEntities) > maxEntryEntities {
			return nil, fmt.Errorf(
				"entry %d: carries %d entities, limit %d",
				i, len(rawEntities), maxEntryEntities,
			)
		}
		entry.Entities = make([]string, 0, len(rawEntities))
		for j, rawEntity := range rawEntities {
			entity, err := strictString(rawEntity, "entity")
			if err != nil {
				return nil, fmt.Errorf("entry %d, entity %d: %w", i, j, err)
			}
			if n := utf8.RuneCountInString(entity); n > maxEntityChars {
				return nil, fmt.Errorf(
					"entry %d, entity %d: %d characters, limit %d",
					i, j, n, maxEntityChars,
				)
			}
			entry.Entities = append(entry.Entities, entity)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// strictObject unmarshals data as a JSON object holding exactly the given
// keys, matched case-sensitively. json.Unmarshal already rejects trailing
// data after the value.
func strictObject(
	data jsontext.Value, keys []string,
) (map[string]jsontext.Value, error) {
	if isJSONNull(data) {
		return nil, errors.New("expected an object, got null")
	}
	var object map[string]jsontext.Value
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	for _, key := range keys {
		if _, ok := object[key]; !ok {
			return nil, fmt.Errorf("required key %q is missing", key)
		}
	}
	for key := range object {
		if !slices.Contains(keys, key) {
			// The key is endpoint-supplied and can approach the transport
			// limit; bound it before it reaches logs and failure rows.
			return nil, fmt.Errorf("unknown key %q", boundedToken(key, 60))
		}
	}
	return object, nil
}

func strictArray(
	data jsontext.Value, name string,
) ([]jsontext.Value, error) {
	if isJSONNull(data) {
		return nil, fmt.Errorf("%s must be an array, got null", name)
	}
	var list []jsontext.Value
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("%s must be an array: %w", name, err)
	}
	return list, nil
}

func strictString(data jsontext.Value, name string) (string, error) {
	if isJSONNull(data) {
		return "", fmt.Errorf("%s must be a string, got null", name)
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return "", fmt.Errorf("%s must be a string: %w", name, err)
	}
	return value, nil
}

// isJSONNull matters because json.Unmarshal treats null as a no-op for
// maps, slices, and strings instead of reporting a type mismatch.
func isJSONNull(data jsontext.Value) bool {
	return string(bytes.TrimSpace(data)) == "null"
}

// isTransientStatus reports whether an HTTP status is worth retrying:
// timeouts, rate limits, and server-side errors. Other non-200 statuses
// (auth failures, missing routes, validation rejections) are deterministic
// for the same request and fail fast.
func isTransientStatus(status int) bool {
	if status == http.StatusNotImplemented {
		// 501 is deterministic: the route itself is unimplemented, and
		// retrying the identical request cannot implement it.
		return false
	}
	return status == http.StatusRequestTimeout ||
		status == http.StatusTooManyRequests ||
		status >= 500
}

// isContextOverflowDetail reports whether a 400 body identifies an
// input-length error. A structured error code is unambiguous; otherwise
// the message must pair an input-side subject (context, prompt, input)
// with an overflow term (exceed, too long/large, maximum). Bare "token" is
// deliberately not a subject: output-budget rejections like "max_tokens
// exceeds the maximum allowed value" would match it, and splitting the
// input cannot fix an invalid output limit — while every genuine overflow
// phrasing also names the prompt, input, or context. A phrasing this
// misses fails the unit with the server's message intact, which is
// recoverable by configuration; the reverse mistake would send the caller
// splitting units in a useless loop.
func isContextOverflowDetail(body string) bool {
	lower := strings.ToLower(body)
	codes := []string{
		"context_length_exceeded",
		"exceed_context_size_error",
	}
	for _, code := range codes {
		if strings.Contains(lower, code) {
			return true
		}
	}
	containsAny := func(needles []string) bool {
		return slices.ContainsFunc(needles, func(needle string) bool {
			return strings.Contains(lower, needle)
		})
	}
	hasOverflowTerm := containsAny(
		[]string{"exceed", "too long", "too large", "maximum"},
	)
	// Explicit input-side length evidence wins outright — including
	// combined-budget messages like "input tokens plus max_tokens exceed
	// the model maximum", which splitting the input does fix. Bare "input"
	// and bare "prompt" are deliberately not subjects: servers prefix
	// arbitrary validation errors with "Input validation error:", and
	// parameter names like prompt_logprobs would pair with any overflow
	// term. Length-qualified forms are safe — "invalid input length
	// parameter" has no overflow term.
	if containsAny([]string{
		"input is too long", "input too long",
		"input is too large", "input too large",
		"prompt is too long", "prompt too long",
		"prompt is too large", "prompt too large",
	}) {
		return true
	}
	if hasOverflowTerm && containsAny([]string{
		"input length", "input tokens", "prompt length", "prompt tokens",
	}) {
		return true
	}
	// With no input-side evidence, a message naming an output-budget
	// parameter is a configuration error — even "max_tokens exceeds the
	// context window" is not fixed by splitting the input.
	if containsAny([]string{
		"max_tokens", "max_new_tokens", "max_completion_tokens",
	}) {
		return false
	}
	// Bare "context" paired with an overflow term covers the common server
	// phrasings: "maximum context length is ...", "exceeds the available
	// context size".
	return hasOverflowTerm && strings.Contains(lower, "context")
}

// SplitFloorChars is the smallest unit size worth splitting further. A unit
// below this floor that still overflows or truncates is not a size problem,
// so the error should surface instead of recursing forever. The floor
// scales down with the window budget so small-window configurations can
// still split.
func SplitFloorChars(maxWindowChars int) int {
	floor := maxWindowChars / 8
	if floor > 2000 {
		return 2000
	}
	if floor < 1 {
		return 1
	}
	return floor
}

func (c *Client) distill(ctx context.Context, systemPrompt, text string, types []string) ([]Entry, Usage, error) {
	if c.distiller != nil {
		return c.distiller(ctx, systemPrompt, text, types)
	}
	return nil, Usage{}, fmt.Errorf("%w: embedded recall model transport has been removed", errPermanentRequest)
}
