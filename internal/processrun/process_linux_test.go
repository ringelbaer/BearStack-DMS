//go:build linux

package processrun

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestBubblewrapDeniesOtherDocumentsAndOriginalWrites(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bubblewrap not installed")
	}
	t.Setenv("BEARSTACK_CONVERTER_SANDBOX", "bubblewrap")
	root, output := t.TempDir(), t.TempDir()
	input, secret := filepath.Join(root, "input.txt"), filepath.Join(root, "credentials.txt")
	for path, data := range map[string]string{input: "original", secret: "secret"} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	script := `cat "$1" > "$3/result" || exit 10
if cat "$2" >/dev/null 2>&1; then exit 11; fi
if (printf changed > "$1") 2>/dev/null; then exit 12; fi
printf shadow > "$2" || exit 13
printf temp > "$HOME/tmp" || exit 14
if cat "$4" >/dev/null 2>&1; then exit 15; fi`
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", script, "sh", input, secret, output, "/proc/"+strconv.Itoa(os.Getpid())+"/environ")
	diag, err := Run(cmd, Files{Read: []string{input}, Write: []string{output}})
	if err != nil {
		t.Fatalf("sandbox: %v: %s", err, diag)
	}
	data, err := os.ReadFile(secret)
	if err != nil || string(data) != "secret" {
		t.Fatalf("host secret modified: %q %v", data, err)
	}
	for _, path := range []string{input, filepath.Join(output, "result")} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "original" {
			t.Fatalf("%s: %q %v", path, data, err)
		}
	}
}

func TestBubblewrapCannotReachHostLoopback(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bubblewrap not installed")
	}
	t.Setenv("BEARSTACK_CONVERTER_SANDBOX", "bubblewrap")
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", `if (echo test > /dev/tcp/127.0.0.1/"$1") 2>/dev/null; then exit 17; fi`, "bash", port)
	if diag, err := Run(cmd, Files{}); err != nil {
		t.Fatalf("network isolation failed: %v: %s", err, diag)
	}
}

func TestRequestedSandboxFailureNeverRetriesOutside(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "bwrap"), []byte("#!/bin/sh\nexit 99\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("BEARSTACK_CONVERTER_SANDBOX", "bubblewrap")
	target := filepath.Join(t.TempDir(), "should-not-exist")
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", `printf unconfined > "$1"`, "sh", target)
	if _, err := Run(cmd, Files{}); err == nil {
		t.Fatal("sandbox failure was ignored")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("command retried without sandbox: %v", err)
	}
}
