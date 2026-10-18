//go:build !(windows && arm64)

package duckdb

import (
	"database/sql"
	"errors"
	"net/url"
	"strings"

	// Registers the "duckdb" database/sql driver, which statically links
	// the prebuilt DuckDB library from duckdb-go-bindings.
	duckdbdriver "github.com/duckdb/duckdb-go/v2"
)

func openDuckDB(dsn string) (*sql.DB, error) {
	path, query, _ := strings.Cut(dsn, "?")
	options, err := url.ParseQuery(query)
	if err != nil {
		return nil, err
	}
	options.Set("enable_external_access", "false")
	options.Set("autoload_known_extensions", "false")
	options.Set("autoinstall_known_extensions", "false")
	return sql.Open("duckdb", path+"?"+options.Encode())
}

func isMirrorOpenInSameProcessError(err error) bool {
	return errors.Is(err, &duckdbdriver.Error{
		Msg: "Connection Error: Can't open a connection to same database file with a different configuration than existing connections",
	})
}
