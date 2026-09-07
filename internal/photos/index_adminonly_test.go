package photos

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type adminRefreshConnector struct {
	mediaQueryConnector
	executions *atomic.Int64
}

func (c adminRefreshConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.mediaQueryConnector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return adminRefreshConn{conn.(mediaQueryConn), c.executions}, nil
}

type adminRefreshConn struct {
	mediaQueryConn
	executions *atomic.Int64
}

func (c adminRefreshConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.executions.Add(1)
	return c.mediaQueryConn.ExecContext(ctx, query, args)
}

func (c adminRefreshConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.Conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	return adminRefreshStmt{stmt, c.executions}, nil
}

type adminRefreshStmt struct {
	driver.Stmt
	executions *atomic.Int64
}

func (s adminRefreshStmt) Exec(args []driver.Value) (driver.Result, error) {
	s.executions.Add(1)
	return s.Stmt.Exec(args)
}

func TestAdminOnlyRefreshBatchesUnchangedDirectories(t *testing.T) {
	l := newTestLibrary(t, t.TempDir())
	defer l.Close()
	const count = 2*adminOnlyUpdateBatchSize + 1
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("album-%04d", i)
		if err := os.Mkdir(filepath.Join(l.Root(), name), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := l.saveFolder(Folder{Path: name, Name: name, ModTime: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	dsn, err := indexSQLiteDSN(l.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	var queries, executions atomic.Int64
	db := sql.OpenDB(adminRefreshConnector{mediaQueryConnector{l.index.db.Driver(), dsn, &queries}, &executions})
	if err := l.index.db.Close(); err != nil {
		t.Fatal(err)
	}
	l.index.db = db
	if err := l.refreshAdminOnlyIndexFlags(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := queries.Load(); got != 4 {
		t.Fatalf("%d unchanged folders need %d queries, want 4", count, got)
	}
	if got := executions.Load(); got != 0 {
		t.Fatalf("unchanged folders caused %d writes, want none", got)
	}
}

func adminRefreshFixture(t *testing.T) *Library {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "album", "child"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"root.jpg", "album/a.jpg", "album/child/b.jpg"} {
		writeJPEG(t, filepath.Join(root, name), color.White)
	}
	l := newTestLibrary(t, root)
	if _, err := l.RebuildIndex(context.Background()); err != nil {
		l.Close()
		t.Fatal(err)
	}
	date := time.Now()
	if err := l.saveBlogBatch(context.Background(), []BlogPost{{Path: "album/child/log.md", Name: "log.md", Text: "entry", Date: &date, ModTime: date}}); err != nil {
		l.Close()
		t.Fatal(err)
	}
	return l
}

func TestAdminOnlyRefreshChangesAllTablesAndPreservesUnaffectedRows(t *testing.T) {
	l := adminRefreshFixture(t)
	defer func() { l.Close() }()
	for _, phase := range []struct {
		marker      string
		remove      bool
		root, album int
	}{{"album/.adminonly", false, 0, 1}, {"album/.adminonly", true, 0, 0}, {".adminonly", false, 1, 1}} {
		marker := filepath.Join(l.Root(), phase.marker)
		if phase.remove {
			if err := os.Remove(marker); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(marker, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		root, cache, dbPath := l.Root(), l.CacheDir(), l.DBPath()
		if err := l.Close(); err != nil {
			t.Fatal(err)
		}
		var err error
		l, err = New(root, cache, dbPath, 50)
		if err != nil {
			t.Fatal(err)
		}
		for table, pathColumn := range map[string]string{"media_index": "path", "blog_index": "path", "folder_index": "path"} {
			rows, err := l.index.db.Query(`SELECT ` + pathColumn + `, admin_only FROM ` + table)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var path string
				var flag int
				if err := rows.Scan(&path, &flag); err != nil {
					rows.Close()
					t.Fatal(err)
				}
				want := phase.album
				if path == "root.jpg" {
					want = phase.root
				}
				if flag != want {
					t.Errorf("%s %s: admin_only=%d, want %d", table, path, flag, want)
				}
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			rows.Close()
		}
		var count int
		if err := l.index.db.QueryRow(`SELECT public_recursive_media_count FROM folder_index WHERE path='album'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if want := 2 * (1 - phase.album); count != want {
			t.Errorf("public recursive count=%d, want %d", count, want)
		}
	}
}

func TestAdminOnlyRefreshRollsBackOnError(t *testing.T) {
	l := adminRefreshFixture(t)
	defer l.Close()
	if _, err := l.index.db.Exec(`CREATE TRIGGER fail_visibility BEFORE UPDATE OF admin_only ON blog_index BEGIN SELECT RAISE(ABORT, 'visibility test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.Root(), "album", AdminOnlyMarkerName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := l.refreshAdminOnlyIndexFlags(context.Background()); err == nil || !strings.Contains(err.Error(), "visibility test failure") {
		t.Fatalf("refresh error=%v", err)
	}
	for _, table := range []string{"media_index", "folder_index", "blog_index"} {
		var private int
		if err := l.index.db.QueryRow(`SELECT COUNT(*) FROM ` + table + ` WHERE admin_only<>0`).Scan(&private); err != nil || private != 0 {
			t.Fatalf("partial changes in %s: %d, %v", table, private, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.refreshAdminOnlyIndexFlags(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled refresh=%v", err)
	}
}
