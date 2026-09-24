// Package contracttest provides the shared, create-only provider acceptance suite.
// Provider implementations must run Run against their real protocol adapter.
package contracttest

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"bearstack/internal/transfers"
)

type Fixture struct {
	Client transfers.Client
	Base   transfers.Location
	// Read verifies remote bytes independently from the adapter under test.
	Read func([]string) []byte
}
type memoryFile struct{ *bytes.Reader }

func (memoryFile) Close() error { return nil }
func Upload(parts []string, data []byte) transfers.Upload {
	return transfers.Upload{Segments: parts, Size: int64(len(data)), Body: memoryFile{bytes.NewReader(data)}, Modified: time.Unix(1700000000, 0)}
}
func Run(t *testing.T, fixture func(*testing.T) Fixture) {
	t.Helper()
	t.Run("CreateProgressAndPreserve", func(t *testing.T) {
		f := fixture(t)
		ctx := context.Background()
		locations, err := f.Client.Locations(ctx, f.Base.ID)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, location := range locations {
			if location.ID == f.Base.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("base location is not selectable")
		}

		parts := []string{"export", "nested", "photo ü#%.jpg"}
		if err := f.Client.EnsureDirectories(ctx, f.Base, parts[:2]); err != nil {
			t.Fatal(err)
		}
		original := bytes.Repeat([]byte("original"), 4096)
		u := Upload(parts, original)
		var last int64
		u.Progress = func(n int64) {
			if n < last || n > u.Size {
				t.Errorf("invalid progress %d after %d", n, last)
			}
			last = n
		}
		if err := f.Client.CreateFile(ctx, f.Base, u); err != nil {
			t.Fatal(err)
		}
		if last != u.Size {
			t.Fatalf("progress %d != %d", last, u.Size)
		}
		if got := f.Read(parts); !bytes.Equal(got, original) {
			t.Fatal("remote bytes differ")
		}
		for _, other := range [][]byte{original, []byte("replacement")} {
			if err := f.Client.CreateFile(ctx, f.Base, Upload(parts, other)); transfers.Kind(err) != transfers.Conflict || err == nil {
				t.Fatalf("overwrite accepted: %v", err)
			}
			if got := f.Read(parts); !bytes.Equal(got, original) {
				t.Fatal("existing content changed")
			}
		}
		entry, err := f.Client.Stat(ctx, f.Base, parts)
		if err != nil || entry == nil || entry.Size != int64(len(original)) || entry.Directory {
			t.Fatalf("stat %#v %v", entry, err)
		}
		count := 0
		err = f.Client.Inventory(ctx, f.Base, parts[:2], func(e transfers.Entry) error { count++; return nil })
		if err != nil || count != 1 {
			t.Fatalf("inventory %d %v", count, err)
		}
		if err := f.Client.EnsureDirectories(ctx, f.Base, parts); transfers.Kind(err) != transfers.Conflict || err == nil {
			t.Fatalf("file as directory: %v", err)
		}
	})
	t.Run("ConcurrentCreate", func(t *testing.T) {
		f := fixture(t)
		ctx := context.Background()
		if err := f.Client.EnsureDirectories(ctx, f.Base, []string{"race"}); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, body := range []string{"first", "second"} {
			wg.Go(func() { errs <- f.Client.CreateFile(ctx, f.Base, Upload([]string{"race", "photo.jpg"}, []byte(body))) })
		}
		wg.Wait()
		close(errs)
		success, conflict := 0, 0
		for err := range errs {
			if err == nil {
				success++
			} else if transfers.Kind(err) == transfers.Conflict {
				conflict++
			} else {
				t.Fatal(err)
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatalf("success=%d conflict=%d", success, conflict)
		}
	})
	t.Run("PathsCancellationAndRetry", func(t *testing.T) {
		f := fixture(t)
		ctx := context.Background()
		for _, parts := range [][]string{nil, {"..", "escape.jpg"}, {"x/y"}, {"x\\y"}, {"."}, {""}, {"x\x00y"}} {
			if err := f.Client.CreateFile(ctx, f.Base, Upload(parts, []byte("data"))); err == nil {
				t.Fatalf("accepted %q", parts)
			}
		}
		if err := f.Client.EnsureDirectories(ctx, f.Base, []string{"cancel"}); err != nil {
			t.Fatal(err)
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		parts := []string{"cancel", "photo.jpg"}
		if err := f.Client.CreateFile(cancelled, f.Base, Upload(parts, []byte("data"))); err == nil {
			t.Fatal("ignored cancellation")
		}
		if got := f.Read(parts); len(got) != 0 {
			t.Fatal("cancelled upload created content")
		}
		if err := f.Client.CreateFile(ctx, f.Base, Upload(parts, []byte("data"))); err != nil {
			t.Fatal(err)
		}
	})
}

var _ transfers.File = memoryFile{}
var _ io.ReaderAt = memoryFile{}
