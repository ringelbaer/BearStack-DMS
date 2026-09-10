// Package testutil provides fixtures shared by application and integration tests.
package testutil

import (
	_ "embed"
	"os"
	"path/filepath"
	"testing"
)

//go:embed testdata/fake-soffice.sh
var fakeSoffice []byte

// InstallSoffice installs a deterministic stand-in; it does not validate real
// Office conversion. PATH and all files are restored/removed by the test cleanup.
func InstallSoffice(t testing.TB) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "soffice"), fakeSoffice, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
