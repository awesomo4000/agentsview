package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/dbtest"
)

const telemetryEventsURL = "http://127.0.0.1:8080/api/v1/telemetry/events"

func newTelemetryCaptureServer(
	t *testing.T, requireAuth bool, opts ...Option,
) *Server {
	t.Helper()
	cfg := config.Config{Host: "127.0.0.1", Port: 8080}
	if requireAuth {
		cfg.RequireAuth, cfg.AuthToken = true, "test-token"
	}
	return New(cfg, dbtest.OpenTestDB(t), nil, opts...)
}

func postTelemetryEvent(
	t *testing.T, srv *Server, body, origin, token string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		telemetryEventsURL, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func TestTelemetryCaptureRouteRemoved(t *testing.T) {
	srv := newTelemetryCaptureServer(t, true)

	rec := postTelemetryEvent(t, srv, `{"event":"app_opened"}`,
		"http://127.0.0.1:8080", "test-token")
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code, rec.Body.String())
}
