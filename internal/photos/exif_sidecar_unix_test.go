//go:build unix

package photos

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestXMPSidecarFIFOIsRejectedWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "photo.xmp")
	if err := unix.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := readXMPSidecar(path); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("sidecar reader blocked on FIFO")
	}
	// Also exercise the flags used after the preliminary regular-file check.
	file, err := os.OpenFile(path, os.O_RDONLY|sidecarOpenFlags, 0)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
}
