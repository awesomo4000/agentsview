package server

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

type openAPISpec struct {
	Paths map[string]map[string]openAPIOperation `json:"paths"`
}

type openAPIOperation struct {
	Parameters []openAPIParameter         `json:"parameters"`
	Responses  map[string]openAPIResponse `json:"responses"`
}

type openAPIParameter struct {
	In       string `json:"in"`
	Name     string `json:"name"`
	Required bool   `json:"required"`
}

type openAPIResponse struct {
	Content map[string]any `json:"content"`
}

func missingEnvRef(tb testing.TB, name string) string {
	tb.Helper()
	require.NoError(tb, os.Unsetenv(name))
	return "${" + name + "}"
}

func readOpenAPISpec(tb testing.TB, h http.Handler) openAPISpec {
	tb.Helper()
	req := httptest.NewRequestWithContext(tb.Context(), http.MethodGet, "/api/openapi.json", nil)
	req.Host = "127.0.0.1:0"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	require.Equal(tb, http.StatusOK, w.Code, "body: %s", w.Body.String())
	var spec openAPISpec
	require.NoError(tb, json.Unmarshal(w.Body.Bytes(), &spec))
	return spec
}
func requireOpenAPIOperation(
	tb testing.TB,
	spec openAPISpec,
	method string,
	path string,
) openAPIOperation {
	tb.Helper()
	require.Contains(tb, spec.Paths, path)
	require.Contains(tb, spec.Paths[path], method)
	return spec.Paths[path][method]
}
