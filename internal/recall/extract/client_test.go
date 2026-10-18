package extract

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scriptedResponse struct {
	status       int
	finishReason string
	content      string
	errorBody    string
	noChoices    bool
}

func newScriptedServer(
	t *testing.T, responses []scriptedResponse, requests *[]map[string]any,
) *httptest.Server {
	t.Helper()
	var index atomic.Int64
	return httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
				assert.Failf(t, "test failed", "request path = %q, want /chat/completions", r.URL.Path)
			}
			var payload map[string]any
			if err := json.UnmarshalRead(r.Body, &payload); err != nil {
				assert.Failf(t, "test failed", "decoding request: %v", err)
			}
			*requests = append(*requests, payload)
			i := int(index.Add(1)) - 1
			if i >= len(responses) {
				assert.Failf(t, "test failed", "unexpected request %d", i)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			resp := responses[i]
			if resp.status != 0 && resp.status != http.StatusOK {
				w.WriteHeader(resp.status)
				errorBody := resp.errorBody
				if errorBody == "" {
					errorBody = `{"error":"scripted"}`
				}
				_, _ = w.Write([]byte(errorBody))
				return
			}
			choices := []map[string]any{{
				"finish_reason": resp.finishReason,
				"message": map[string]any{
					"role":    "assistant",
					"content": resp.content,
				},
			}}
			if resp.noChoices {
				choices = nil
			}
			body := map[string]any{
				"choices": choices,
				"usage": map[string]any{
					"prompt_tokens":     7,
					"completion_tokens": 3,
				},
			}
			_ = json.MarshalWrite(w, body)
		}))
}

func entriesJSON(t *testing.T, titles ...string) string {
	t.Helper()
	entries := make([]map[string]any, 0, len(titles))
	for _, title := range titles {
		entries = append(entries, map[string]any{
			"type": "fact", "title": title, "body": "b",
			"entities": []string{},
		})
	}
	raw, err := json.Marshal(map[string]any{"entries": entries})
	if err != nil {
		require.FailNow(t, fmt.Sprint(err))
	}
	return string(raw)
}

func testClient(url string) *Client {
	client := &Client{
		BaseURL: url,
		Model:   "test-model",
		// Keep transient-retry backoff out of test wall-clock time.
		RetryBackoff: time.Millisecond,
		Request: RequestShape{
			Temperature: 0,
			MaxTokens:   100,
			ExtraBody: map[string]any{
				"chat_template_kwargs": map[string]any{
					"enable_thinking": false,
				},
			},
		},
	}

	client.distiller = client.fixtureDistill
	return client
}

func TestClientRejectsReservedExtraBodyKeys(t *testing.T) {
	// A profile or override smuggling max_tokens through the extra body
	// would bypass validation and desynchronize the generation fingerprint
	// from the request actually sent.
	var requests []map[string]any
	server := newScriptedServer(t, nil, &requests)
	defer server.Close()

	client := testClient(server.URL)
	client.Request.ExtraBody = map[string]any{"max_tokens": 5}
	_, _, err := client.DistillWithRecovery(
		t.Context(), "p", "text", 3,
	)
	if err == nil || !strings.Contains(err.Error(), "max_tokens") {
		require.FailNowf(t, "test failed", "err = %v, want reserved-key rejection naming the key", err)
	}
	if len(requests) != 0 {
		require.FailNowf(t, "test failed", "requests = %d, want 0 (rejected before any call)",
			len(requests))
	}
}

func TestIsContextOverflowDetail(t *testing.T) {
	overflow := []string{
		`{"error":{"code":"context_length_exceeded"}}`,
		`This model's maximum context length is 32768 tokens.`,
		`the request exceeds the available context size`,
		`prompt is too long: 20000 tokens > 16000 maximum`,
		`input is too large for the model context`,
		`input length 5000 exceeds maximum 4096`,
		`input tokens (6000) exceed the model maximum`,
		`input tokens plus max_tokens exceed the model maximum`,
		`prompt is too large for this model`,
		`prompt too large: reduce the input`,
	}
	for _, body := range overflow {
		if !isContextOverflowDetail(body) {
			assert.Failf(t, "test failed", "must classify as overflow: %q", body)
		}
	}
	// A length-related noun alone is not an overflow: these are validation
	// errors that splitting the unit can never fix.
	notOverflow := []string{
		`{"error":{"message":"context window must be an integer"}}`,
		`invalid input length parameter`,
		`unknown field "context_size" in request`,
		`max_tokens must be a positive integer`,
		`max_tokens exceeds the maximum allowed value`,
		`max_new_tokens exceeds the model limit`,
		`Input validation error: max_tokens exceeds the maximum allowed value`,
		`Input validation error: temperature exceeds the maximum allowed value`,
		`invalid input: repetition_penalty exceeds maximum`,
		`prompt_logprobs exceeds maximum allowed value`,
		`max_tokens (5000) exceeds the context window (4096)`,
		`model "test-model" not found`,
	}
	for _, body := range notOverflow {
		if isContextOverflowDetail(body) {
			assert.Failf(t, "test failed", "must not classify as overflow: %q", body)
		}
	}
}

// TestClientErrorDetailRedactsReflectedCredentials pins that response-body
// excerpts embedded in non-transient errors cannot leak the configured
// endpoint's credentials: proxies and gateways echo the request back —
// the URI with its query values (raw or URL-escaped) and the Basic-auth
// header the userinfo becomes — and these error strings reach doctor
// stderr and stored failure rows.

// TestClientErrorDetailRedactsRawQueryForms pins the two ways decoded-form
// masking alone fails: a query url.ParseQuery rejects (masking must not
// fail open with it), and a noncanonically encoded value whose exact wire
// bytes — what a body echoing the request URI carries — differ from both
// the decoded and the canonically re-encoded form.

// TestClientErrorDetailWithholdsBodyForCredentialedEndpoints pins that a
// response body from an endpoint whose URL carries credential material is
// never excerpted at all: literal scrubbing loses against an endpoint that
// re-encodes the credential (JSON \u escapes turn sekret into
// se\u006bret) or against secrets shorter than any masking floor.

// TestClientErrorDetailStripsControlCharacters pins that a kept response
// excerpt (credential-free endpoint) cannot smuggle terminal escapes or
// carriage returns into doctor stderr and CI logs.

// TestClientBoundsUnknownFinishReasonDetail pins that a hostile 200 whose
// finish_reason approaches the transport limit cannot balloon the error —
// the manager persists error text into a failure row per session.

// TestClientDeterministicStatusesAreEndpointScoped pins that statuses a
// same-configured endpoint answers identically for every request — wrong
// method or media type on the route (405, 415), unimplemented route (501)
// — fail fast without transient retries and classify as endpoint-scoped,
// so the manager aborts the pass instead of burning one doomed call per
// session.

// TestClientWithholdsSuccessDiagnosticsForCredentialedEndpoints pins that
// HTTP 200 error paths get the same treatment as non-200 bodies: a server
// that knows the credential can reflect it through finish_reason, unknown
// keys, or any schema-violating field, and those errors reach persisted
// failure rows, scheduler logs, and doctor output.

// TestClientWithholdsBodyForPathTokenEndpoints pins that path segments
// outside the known API-surface vocabulary count as credential material:
// webhook-style gateways authenticate through path tokens — long or short,
// possibly split across segments — and a body echoing the request URI
// would otherwise carry them into persisted errors and logs through the
// kept excerpt.

// TestClientWithholdsMalformedRedirectDetail pins the transport-error
// leak path: a malformed redirect Location is parsed before CheckRedirect
// runs, and Go's parse error quotes the raw header — a server that knows
// the endpoint credential can reflect it there, bypassing the redirect
// and response-body redaction.

// TestClientBoundsTransportErrorDetail pins the credential-free side: the
// malformed-Location diagnostic stays — that is where the operator sees
// the broken proxy — control-stripped and length-bounded.

// TestClientOmitsRedirectTargetForCredentialedEndpoints pins that a
// refused redirect's error names no target when the configured endpoint
// carries credential material: a redirect can put the credential in the
// target hostname, and RedactedEndpoint preserves hostnames.

// TestClientBoundsRedirectTargetDetail pins the credential-free side: the
// redirect target stays in the error — it is the diagnostic — but a
// hostile Location cannot balloon persisted failure rows.

// TestClientSanitizesBodyReadErrorDetail pins the body-read twin of the
// transport-error policy: a malformed chunked trailer line is echoed
// verbatim by Go's read error ('malformed MIME header: missing colon:
// "<raw line>"'), so read errors carry raw wire bytes into doctor
// output, scheduler logs, and persisted failure rows unless routed
// through the same sanitizer.

// TestClientKeepsDiagnosticsForKnownAPIPaths pins the other side: the
// standard OpenAI-compatible path shapes are API surface, not secrets,
// and withholding their diagnostics would gut doctor output for every
// local inference server.

// TestClientBoundsUnknownKeyDetail pins that an unknown JSON key — which a
// hostile 200 can grow toward the transport limit — reaches error text
// only as a bounded token.

// TestClientRedirectRefusalIsEndpointScoped pins that a refused redirect —
// deterministic for every request against the same configuration — fails
// on the first attempt and classifies as endpoint-scoped, so the manager
// aborts the pass instead of retrying three times per unit and marking
// every session failed.

// TestClientRequestEntityTooLargeSplits pins that HTTP 413 — the transport
// telling us the request body is too big — surfaces as ErrContextOverflow
// so the caller splits the unit, exactly like an in-band context-overflow
// 400. As a permanent failure the session would retry unchanged after
// every backoff, forever.

// TestClientRejectsOversizedContent pins the local resource bounds: a
// configured or compromised endpoint can answer within the transport size
// limit yet carry tens of thousands of entries or multi-megabyte fields,
// and accepting them would balloon the archive and hold its write lock
// through the inserts. Violations are deterministic, so no retry.

// TestClientClassifiesTransportOverflowWithoutRetry pins the HTTP boundary:
// reading exactly at the cap cannot reveal whether the body was truncated.
// The extra sentinel byte must turn an oversized 200 into a client-only limit
// error instead of a transient JSON parse failure and retry ladder.

// TestClientRequestSchemaKeepsLargeBodyLimitLocal pins the server boundary:
// large maxLength values make some JSON-schema-to-grammar implementations
// reject the entire request, while the client still rejects oversized bodies
// in TestClientRejectsOversizedContent.

// Restricted requests validate the response against the enum sent to the
// server. A type outside the narrowed enum indicts this unit's output, while
// a type outside every enum means the server ignores the schema.

func TestSplitFloorChars(t *testing.T) {
	if got := SplitFloorChars(50000); got != 2000 {
		require.FailNowf(t, "test failed", "SplitFloorChars(50000) = %d, want 2000", got)
	}
	if got := SplitFloorChars(800); got != 100 {
		require.FailNowf(t, "test failed", "SplitFloorChars(800) = %d, want 100", got)
	}
}

// TestClientEndpointWithQueryRoutesCorrectly pins URL construction for
// endpoints carrying query parameters (Azure-style ?api-version=...):
// string concatenation would land the route inside the query value and hit
// the bare endpoint path instead.

// TestClientTransportErrorRedactsEndpoint pins that connection failures do
// not echo endpoint credentials: these errors land in doctor output and in
// stored progress rows, and endpoints may carry Basic-auth userinfo or API
// keys in query parameters. (Go's url.Error already masks the password but
// keeps the username and the query string.)
