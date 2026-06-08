package sqlitedsn

import (
	"net/url"
	"path/filepath"
	"testing"
)

func TestWithPragmasPreservesExistingQueryAndEncodesPragmas(t *testing.T) {
	dsn, err := WithPragmas("file:/tmp/test.db?cache=shared", "foreign_keys(ON)", "", "busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("cache") != "shared" {
		t.Fatalf("cache query = %q", query.Get("cache"))
	}
	pragmas := query["_pragma"]
	if len(pragmas) != 2 || pragmas[0] != "foreign_keys(ON)" || pragmas[1] != "busy_timeout(5000)" {
		t.Fatalf("pragmas = %#v", pragmas)
	}
}

func TestFilePathEncodesFilesystemPathAsFileURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Bear Stack", "test.db")
	dsn, err := FilePath(path, "journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "file" {
		t.Fatalf("scheme = %q", parsed.Scheme)
	}
	if parsed.Query().Get("_pragma") != "journal_mode(WAL)" {
		t.Fatalf("query = %q", parsed.RawQuery)
	}
	if got := parsed.Path; got != filepath.ToSlash(path) {
		t.Fatalf("path = %q, want %q", got, filepath.ToSlash(path))
	}
}
