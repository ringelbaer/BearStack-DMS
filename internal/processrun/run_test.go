package processrun

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOutputAndDiagnosticsHaveIndependentLimits(t *testing.T) {
	t.Setenv("BEARSTACK_CONVERTER_SANDBOX", "off")
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", `i=0; while [ "$i" -lt 5000 ]; do printf abc; printf def >&2; i=$((i+1)); done`)
	out, diag, err := Output(cmd, Files{}, 1000)
	if !errors.Is(err, ErrOutputLimit) || len(out) != 1000 || len(diag) > DiagnosticLimit+32 || !strings.Contains(string(diag), "truncated") {
		t.Fatalf("out=%d diag=%d err=%v", len(out), len(diag), err)
	}
	cmd = exec.CommandContext(context.Background(), "/bin/sh", "-c", `i=0; while [ "$i" -lt 5000 ]; do printf abc; i=$((i+1)); done; exit 7`)
	diag, err = Run(cmd, Files{})
	var status *exec.ExitError
	if !errors.As(err, &status) || status.ExitCode() != 7 || len(diag) > DiagnosticLimit+32 {
		t.Fatalf("diag=%d err=%v", len(diag), err)
	}
}

func TestConverterDoesNotInheritCredentialsAndUsesPrivateHome(t *testing.T) {
	t.Setenv("BEARSTACK_CONVERTER_SANDBOX", "off")
	t.Setenv("BEARSTACK_AUTH_PASSWORD", "server-secret")
	t.Setenv("https_proxy", "proxy-secret")
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", `[ -z "${BEARSTACK_AUTH_PASSWORD-}${https_proxy-}" ] || exit 9; [ "$HOME" = "$TMPDIR" ] || exit 8; printf '%s' "$HOME"`)
	out, err := Run(cmd, Files{})
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if _, err := os.Stat(string(out)); !os.IsNotExist(err) {
		t.Fatalf("temporary home leaked: %v", err)
	}
}

func TestCancellationClosesDescendantPipes(t *testing.T) {
	t.Setenv("BEARSTACK_CONVERTER_SANDBOX", "off")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Run(exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 30 & wait"), Files{})
	if err == nil || time.Since(start) > 4*time.Second {
		t.Fatalf("cancellation: %v after %v", err, time.Since(start))
	}
}

func TestUnknownSandboxNeverRunsCommand(t *testing.T) {
	t.Setenv("BEARSTACK_CONVERTER_SANDBOX", "typo")
	target := filepath.Join(t.TempDir(), "ran")
	_, err := Run(exec.CommandContext(context.Background(), "/bin/sh", "-c", `touch "$1"`, "sh", target), Files{})
	if err == nil {
		t.Fatal("invalid mode accepted")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("command ran without confinement")
	}
}
