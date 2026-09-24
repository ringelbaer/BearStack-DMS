package transfers

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// State-level checks use only local rows, independent of provider protocols.
func stateEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := Open(filepath.Join(t.TempDir(), "transfers"), Registry{}, nil, func(context.Context, string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	c := Connection{ID: "c", Name: "c", Provider: "opaque", Enabled: true, Connected: true, Revision: 1}
	if _, err = e.db.Exec("INSERT INTO connections(id,data) VALUES(?,?)", c.ID, encode(c)); err != nil {
		t.Fatal(err)
	}
	return e
}
func stateJob(t *testing.T, e *Engine, id, state string, submitted bool) {
	t.Helper()
	now := time.Now().Unix()
	_, err := e.db.Exec(`INSERT INTO jobs(id,connection_id,revision,actor,selection,source_name,target,base,state,created,expires,updated,submitted) VALUES(?,'c',1,'actor','{}','source','target','{}',?,?,?,?,?)`, id, state, now, now+1800, now, submitted)
	if err != nil {
		t.Fatal(err)
	}
}
func TestConfirmationFIFOAndExpiry(t *testing.T) {
	e := stateEngine(t)
	ctx := context.Background()
	stateJob(t, e, "a", "ready", false)
	stateJob(t, e, "b", "ready", false)
	for _, id := range []string{"b", "a", "b"} {
		if err := e.Action(ctx, id, "start"); err != nil {
			t.Fatal(err)
		}
	}
	id, err := e.claim(ctx, "queued", "running")
	if err != nil || id != "b" {
		t.Fatalf("not confirmation FIFO: %s %v", id, err)
	}
	id, err = e.claim(ctx, "queued", "running")
	if err != nil || id != "a" {
		t.Fatalf("second %s %v", id, err)
	}
	stateJob(t, e, "expired", "ready", false)
	e.db.Exec("UPDATE jobs SET expires=? WHERE id='expired'", time.Now().Add(-time.Hour).Unix())
	if err = e.Action(ctx, "expired", "start"); Kind(err) != Conflict {
		t.Fatalf("expired start %v", err)
	}
	e.cleanup(ctx)
	if _, err = e.Job(ctx, "expired"); err == nil {
		t.Fatal("expired preview retained")
	}
	if _, err = e.Job(ctx, "a"); err != nil {
		t.Fatal("confirmed job removed")
	}
}
func TestRecoveryUncertainUploadsAndIndependentCredentialEncryption(t *testing.T) {
	e := stateEngine(t)
	ctx := context.Background()
	stateJob(t, e, "j", "running", true)
	_, err := e.db.Exec(`INSERT INTO items(job_id,path,display_path,relative,directory,size,modified,state) VALUES('j','a.jpg','Fotos / a.jpg','["target","a.jpg"]','["target"]',4,1,'uploading')`)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := e.seal("first", []byte("account-token"))
	if err != nil {
		t.Fatal(err)
	}
	n := e.cipher.NonceSize()
	if _, err = e.cipher.Open(nil, sealed[:n], sealed[n:], []byte("second")); err == nil {
		t.Fatal("credentials interchangeable between connections")
	}
	var dbPath string
	if err = e.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&dbPath); err != nil {
		t.Fatal(err)
	}
	e.Close()
	reopened, err := Open(filepath.Dir(dbPath), Registry{}, nil, func(context.Context, string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	j, err := reopened.Job(ctx, "j")
	if err != nil || j.State != "queued" {
		t.Fatalf("recovery %+v %v", j, err)
	}
	items, err := reopened.Items(ctx, "j", 0)
	if err != nil || len(items) != 1 || items[0].State != "missing" {
		t.Fatalf("uncertain file %+v %v", items, err)
	}
}
func TestSegmentBoundaries(t *testing.T) {
	for _, parts := range [][]string{{".."}, {"a/b"}, {"a\\b"}, {"tab\tname"}, {strings.Repeat("a", 256)}, {string([]byte{255})}} {
		if ValidateSegments(parts) == nil {
			t.Fatalf("accepted %q", parts)
		}
	}
	if err := ValidateSegments([]string{"2026", "Familie – Café", "photo #1%.jpg"}); err != nil {
		t.Fatal(err)
	}
}
