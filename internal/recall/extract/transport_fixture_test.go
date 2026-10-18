// Test-only model boundary for archive bookkeeping and recovery tests.
package extract

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/httputil"
	"go.kenn.io/agentsview/internal/stringutil"
)

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		if c.HTTPClient.CheckRedirect == nil {
			// The no-redirect policy is the extraction client's own
			// boundary, not the caller's wiring: a redirect replays the
			// extraction POST — transcript included — wherever the
			// endpoint points. A caller-provided client keeps its other
			// settings; only a missing policy is filled in, on a copy.
			enforced := *c.HTTPClient
			enforced.CheckRedirect = RefuseRedirects
			return &enforced
		}
		return c.HTTPClient
	}
	return &http.Client{
		Timeout:       10 * time.Minute,
		CheckRedirect: RefuseRedirects,
	}
}

func (c *Client) fixtureDistill(
	ctx context.Context, systemPrompt, text string, types []string,
) ([]Entry, Usage, error) {
	payload := map[string]any{
		"model":       c.Model,
		"max_tokens":  c.Request.MaxTokens,
		"temperature": c.Request.Temperature,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": text},
		},
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "session_readout",
				"strict": true,
				"schema": entrySchema(types),
			},
		},
	}
	// Extras first, client-owned fields last: even if validation of
	// reserved keys were bypassed, the extra body could not shadow them.
	merged := make(map[string]any, len(c.Request.ExtraBody)+len(payload))
	maps.Copy(merged, c.Request.ExtraBody)
	maps.Copy(merged, payload)
	body, err := json.Marshal(merged)
	if err != nil {
		return nil, Usage{}, fmt.Errorf("encoding distill request: %w", err)
	}
	// The route joins the parsed endpoint path so query parameters
	// (Azure-style ?api-version=...) survive; string concatenation would
	// land the route inside the query value.
	endpoint, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, Usage{}, fmt.Errorf(
			"invalid endpoint %s", config.RedactedEndpoint(c.BaseURL))
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/chat/completions"
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body),
	)
	if err != nil {
		return nil, Usage{}, fmt.Errorf("building distill request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	response, err := c.httpClient().Do(request)
	if err != nil {
		// url.Error echoes the request URL with only the password masked:
		// the username (which can itself be an API key) and the query
		// string still leak, and this message reaches doctor output and
		// stored failure rows. Report the redacted endpoint instead.
		cause := err
		if urlErr, ok := errors.AsType[*url.Error](err); ok {
			cause = urlErr.Err
		}
		if errors.Is(cause, errRedirectRefused) {
			// Deterministic for every request against the same
			// configuration: fail fast, endpoint-scoped, no retries.
			if c.credentialedEndpoint() {
				// The refusal names its redirect target, and a redirect
				// can put the endpoint credential in the target hostname,
				// which RedactedEndpoint preserves. Keep the sentinel
				// alone.
				return nil, Usage{}, fmt.Errorf(
					"posting distill request to %s: %w",
					config.RedactedEndpoint(endpoint.String()),
					errRedirectRefused)
			}
			// The cause is RefuseRedirects' own message with the target
			// redacted and bounded, so it stays on the chain verbatim.
			return nil, Usage{}, fmt.Errorf(
				"posting distill request to %s: %w",
				config.RedactedEndpoint(endpoint.String()), cause)
		}
		// Beyond the URL, the cause text itself can quote
		// server-controlled bytes — a malformed redirect Location is
		// parsed before CheckRedirect runs and echoed raw by its parse
		// error, and a malformed HTTP response is quoted verbatim — so
		// the detail is withheld for credentialed endpoints and
		// control-stripped and bounded otherwise. Classification needs
		// nothing from the chain here: everything but a redirect
		// refusal is transient.
		return nil, Usage{}, &transientError{err: fmt.Errorf(
			"posting distill request to %s: %s",
			config.RedactedEndpoint(endpoint.String()),
			c.transportErrorDetail(cause))}
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(
		response.Body, int64(maxResponseBodyBytes)+1,
	))
	if err != nil {
		// Read errors quote raw wire bytes — a malformed chunked trailer
		// line is echoed verbatim — so the detail follows the same policy
		// as transport errors.
		return nil, Usage{}, &transientError{
			err: fmt.Errorf("reading distill response: %s",
				c.transportErrorDetail(err)),
		}
	}
	responseTooLarge := len(raw) > maxResponseBodyBytes
	if responseTooLarge {
		// Non-success responses are classified by their status below; their
		// diagnostic excerpt never needs the sentinel byte. A successful
		// response is checked before JSON decoding so truncation cannot
		// masquerade as a transient parse failure.
		raw = raw[:maxResponseBodyBytes]
	}
	if response.StatusCode == http.StatusBadRequest {
		detail := c.responseDetail(raw)
		// Chat servers answer 400 both for prompts that exceed the context
		// window (character budgets overshoot because token density varies
		// across content) and for genuinely bad requests; only the former
		// is recoverable by splitting the unit.
		if isContextOverflowDetail(string(raw)) {
			return nil, Usage{}, fmt.Errorf(
				"%d-char unit: %w: %s",
				utf8.RuneCountInString(text), ErrContextOverflow, detail,
			)
		}
		return nil, Usage{}, &requestStatusError{
			status: http.StatusBadRequest,
			err: fmt.Errorf(
				"%w (HTTP 400): %s", errPermanentRequest, detail,
			),
		}
	}
	if response.StatusCode == http.StatusRequestEntityTooLarge {
		// The transport-level twin of an in-band context-overflow 400:
		// the request body is too big, and only splitting the unit can
		// shrink it. A permanent failure here would retry the session
		// unchanged after every backoff, forever.
		return nil, Usage{}, fmt.Errorf(
			"%d-char unit: %w (HTTP 413): %s",
			utf8.RuneCountInString(text), ErrContextOverflow,
			c.responseDetail(raw),
		)
	}
	if response.StatusCode != http.StatusOK {
		statusErr := fmt.Errorf(
			"distill request failed with HTTP %d", response.StatusCode,
		)
		if isTransientStatus(response.StatusCode) {
			return nil, Usage{}, &transientError{
				err: statusErr,
				retryAfter: httputil.ParseRetryAfter(
					response.Header.Get("Retry-After"),
				),
			}
		}
		return nil, Usage{}, &requestStatusError{
			status: response.StatusCode,
			err: fmt.Errorf(
				"%w (HTTP %d): %s", errPermanentRequest,
				response.StatusCode, c.responseDetail(raw),
			),
		}
	}
	if responseTooLarge {
		return nil, Usage{}, fmt.Errorf(
			"%w: response body exceeds the %d-byte transport cap",
			errClientOnlyResponseLimit, maxResponseBodyBytes,
		)
	}

	var parsed struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
	}
	// A 200 with an unreadable or choiceless body is a glitch in transit or
	// in the serving layer, not a property of the input, so it retries.
	if err := json.Unmarshal(raw, &parsed); err != nil {
		cause := fmt.Errorf("parsing distill response: %w", err)
		if c.credentialedEndpoint() {
			cause = fmt.Errorf("parsing distill response failed %s",
				detailWithheld)
		}
		return nil, Usage{}, &transientError{err: cause}
	}
	if len(parsed.Choices) == 0 {
		// The body parsed, so its usage is real cost even without choices.
		return nil, parsed.Usage, &transientError{
			err: errors.New("distill response has no choices"),
		}
	}
	// From here the server reports token usage even when the attempt fails,
	// so error returns carry parsed.Usage for the caller's accounting.
	choice := parsed.Choices[0]
	if choice.FinishReason == "length" {
		return nil, parsed.Usage, fmt.Errorf(
			"at max_tokens=%d: %w", c.Request.MaxTokens, errTruncated,
		)
	}
	if choice.FinishReason != "stop" {
		// A filtered or otherwise cut-off response can still carry valid
		// JSON with fewer entries; only "stop" means the model finished.
		// Deterministic for the same input, so it fails fast.
		return nil, parsed.Usage, fmt.Errorf(
			"distill response finished with %q instead of completing; "+
				"refusing possibly incomplete entries",
			c.responseToken(choice.FinishReason),
		)
	}
	if choice.Message.Content == "" {
		// Empty content with a normal finish reason means the token budget
		// went somewhere invisible (typically hidden reasoning the request
		// shape should have disabled).
		return nil, parsed.Usage, errors.New("distill response content is empty; check the model profile's " +
			"request shape",
		)
	}
	entries, err := parseEntries(choice.Message.Content, types)
	if err != nil {
		if errors.Is(err, errClientOnlyResponseLimit) ||
			errors.Is(err, errDisallowedEntryType) {
			return nil, parsed.Usage, err
		}
		// The server was asked for constrained decoding, so a violation
		// means it did not enforce the schema; at temperature zero that is
		// deterministic and not worth retrying.
		violation := fmt.Errorf(
			"%w: distilled content violates the response schema (does "+
				"the server enforce json_schema?): %w",
			errProtocolViolation, err,
		)
		if c.credentialedEndpoint() {
			// The violating content is endpoint-supplied; its detail
			// (unknown keys, field values) can reflect the credential.
			violation = fmt.Errorf(
				"%w: distilled content violates the response schema "+
					"(does the server enforce json_schema?); detail %s",
				errProtocolViolation, detailWithheld,
			)
		}
		return nil, parsed.Usage, violation
	}
	return entries, parsed.Usage, nil
}

func (c *Client) credentialedEndpoint() bool {
	if c.APIKey != "" {
		return true
	}
	endpoint, err := url.Parse(c.BaseURL)
	if err != nil {
		return true
	}
	if endpoint.User != nil {
		return true
	}
	for _, segment := range strings.FieldsFunc(
		endpoint.RawQuery,
		func(r rune) bool { return r == '&' || r == ';' },
	) {
		key, _, _ := strings.Cut(segment, "=")
		if !strings.EqualFold(key, "api-version") {
			return true
		}
	}
	// Path segments outside the known API-surface vocabulary count too:
	// webhook-style gateways authenticate through path tokens, which can
	// be short or split across segments, so no length heuristic is safe.
	// The allowlist covers the standard OpenAI-compatible shapes (local
	// inference servers at /v1, gateways at /api/v1, Gemini's
	// /v1beta/openai) — the doctor's main diagnostic targets. Anything
	// else, Azure deployment names included, fails closed to withholding
	// the endpoint-provided detail; the HTTP status always survives.
	for segment := range strings.SplitSeq(endpoint.Path, "/") {
		if !safeEndpointPathSegments[strings.ToLower(segment)] {
			return true
		}
	}
	return false
}

func (c *Client) responseDetail(raw []byte) string {
	if _, err := url.Parse(c.BaseURL); err != nil {
		return "(response body withheld: unparseable endpoint)"
	}
	if c.credentialedEndpoint() {
		return "(response body withheld: endpoint URL carries " +
			"credential material)"
	}
	return stringutil.SafeTruncate(stripControls(string(raw)), 200)
}

func (c *Client) transportErrorDetail(cause error) string {
	if errors.Is(cause, context.Canceled) ||
		errors.Is(cause, context.DeadlineExceeded) {
		return cause.Error()
	}
	if c.credentialedEndpoint() {
		return detailWithheld
	}
	return boundedToken(cause.Error(), 200)
}

func (c *Client) responseToken(value string) string {
	if c.credentialedEndpoint() {
		return detailWithheld
	}
	return boundedToken(value, 60)
}
