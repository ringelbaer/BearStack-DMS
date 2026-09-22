// Package processrun owns bounded converter execution and optional filesystem confinement.
package processrun

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const DiagnosticLimit = 4096

var ErrOutputLimit = errors.New("converter output exceeds limit")

// Files grants access only to inputs and dedicated output/profile directories.
// Bubblewrap is opt-in for native installations, and requires user namespaces on Linux.
// Once requested, unavailable confinement is an error, never an unsandboxed retry.
type Files struct{ Read, Write []string }

type buffer struct {
	mu        sync.Mutex
	data      bytes.Buffer
	limit     int
	truncated bool
}

func (b *buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	remaining := max(0, b.limit-b.data.Len())
	if n > remaining {
		b.truncated = true
		p = p[:remaining]
	}
	_, _ = b.data.Write(p)
	return n, nil // Drain pipes even after the retained prefix is full.
}
func (b *buffer) diagnostic() []byte {
	if b.truncated {
		return append(b.data.Bytes(), []byte("\n[output truncated]")...)
	}
	return b.data.Bytes()
}

// Run retains a bounded diagnostic prefix, including on successful commands.
func Run(cmd *exec.Cmd, files Files) ([]byte, error) {
	output := &buffer{limit: DiagnosticLimit}
	cmd.Stdout, cmd.Stderr = output, output
	err := run(cmd, files)
	return output.diagnostic(), err
}

// Output separates machine-readable stdout from diagnostics. Oversized results
// fail explicitly; callers must never persist silently truncated OCR or metadata.
func Output(cmd *exec.Cmd, files Files, limit int) ([]byte, []byte, error) {
	stdout, stderr := &buffer{limit: limit}, &buffer{limit: DiagnosticLimit}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := run(cmd, files)
	if stdout.truncated {
		err = errors.Join(err, ErrOutputLimit)
	}
	return stdout.data.Bytes(), stderr.diagnostic(), err
}

func run(cmd *exec.Cmd, files Files) error {
	work, err := os.MkdirTemp("", "bearstack-converter-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	// Do not pass server credentials, proxy credentials, loader overrides or the
	// user's office/browser configuration into parsers of untrusted documents.
	cmd.Env = nil
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "PATH", "LANG", "LC_ALL", "LC_CTYPE", "TZ", "VIPS_CONCURRENCY":
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "HOME="+work, "TMPDIR="+work, "XDG_CACHE_HOME="+work, "XDG_CONFIG_HOME="+work)
	files.Write = append(append([]string(nil), files.Write...), work)
	mode := os.Getenv("BEARSTACK_CONVERTER_SANDBOX")
	if mode != "" && mode != "off" {
		if mode != "bubblewrap" {
			return fmt.Errorf("unknown BEARSTACK_CONVERTER_SANDBOX mode %q", mode)
		}
		if err := confine(cmd, files); err != nil {
			return err
		}
	}
	prepareProcess(cmd)
	cmd.WaitDelay = 2 * time.Second
	err = cmd.Run()
	cleanupProcess(cmd)
	return err
}

func absolutePaths(paths []string) ([]string, error) {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		result = append(result, absolute)
	}
	return result, nil
}
