package photos

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"bearstack/internal/searchtext"
	"bearstack/internal/sqlutil"
)

func (l *Library) refreshPeoplePageVisibility(ctx context.Context, id int64, q string, knownOnly bool) error {
	if id != 0 {
		return l.refreshPersonIDsVisibility(ctx, id)
	}
	if q == "" && !knownOnly {
		return l.refreshPeopleVisibility(ctx, "")
	}
	filter := `p.name_fold LIKE ? ESCAPE '\'`
	if knownOnly {
		filter += ` AND p.name<>''`
	}
	return l.refreshPeopleVisibility(ctx, filter, searchtext.LikeContainsPattern(searchtext.GermanFold(q)))
}

type faceVisibilityFlight struct {
	done    chan struct{}
	err     error
	started time.Time
}

type faceVisibilityState struct {
	mu      sync.Mutex
	flights map[string]*faceVisibilityFlight
}

// Completed results are never cached. A caller only shares a scan that started
// after it arrived, so a newly added marker cannot be hidden by an older scan.
// Callers arriving during a scan can share the next scan after it finishes.
func (s *faceVisibilityState) check(ctx context.Context, key string, run func() error) error {
	requested := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.mu.Lock()
		if flight := s.flights[key]; flight != nil {
			s.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-flight.done:
				if flight.started.Before(requested) || errors.Is(flight.err, context.Canceled) || errors.Is(flight.err, context.DeadlineExceeded) {
					continue // Another request's cancellation is not our result.
				}
				return flight.err
			}
		}
		flight := &faceVisibilityFlight{done: make(chan struct{}), started: time.Now()}
		if s.flights == nil {
			s.flights = make(map[string]*faceVisibilityFlight)
		}
		s.flights[key] = flight
		s.mu.Unlock()
		flight.err = run()
		s.mu.Lock()
		delete(s.flights, key)
		close(flight.done)
		s.mu.Unlock()
		return flight.err
	}
}

// RefreshFaceVisibility includes queued work for worker/status operations.
func (l *Library) RefreshFaceVisibility(ctx context.Context) error {
	return l.refreshFaceDirectories(ctx, `SELECT directory FROM photo_face_directories WHERE face_count>0
 UNION SELECT directory FROM photo_face_job_directories WHERE job_count>0
 UNION SELECT m.directory FROM photo_people p JOIN media_index m ON m.path=p.name_source WHERE p.manual_name=0`)
}

// filter is internal SQL over p (photo_people), with values bound separately.
// A person's name can originate in a different directory from their faces.
func (l *Library) refreshPeopleVisibility(ctx context.Context, filter string, args ...any) error {
	if filter == "" {
		return l.refreshFaceDirectories(ctx, `SELECT directory FROM photo_face_directories WHERE face_count>0
 UNION SELECT m.directory FROM photo_people p JOIN media_index m ON m.path=p.name_source WHERE p.manual_name=0`)
	}
	query := `SELECT DISTINCT f.directory FROM photo_people p JOIN photo_faces f ON f.person_id=p.id WHERE (` + filter + `)
 UNION SELECT m.directory FROM photo_people p JOIN media_index m ON m.path=p.name_source WHERE p.manual_name=0 AND (` + filter + `)`
	return l.refreshFaceDirectories(ctx, query, append(append([]any{}, args...), args...)...)
}

func (l *Library) refreshPersonIDsVisibility(ctx context.Context, ids ...int64) error {
	if len(ids) == 0 {
		return ctx.Err()
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return l.refreshPeopleVisibility(ctx, `p.id IN (`+sqlutil.Placeholders(len(ids))+`)`, args...)
}

func (l *Library) refreshFaceDirectories(ctx context.Context, query string, args ...any) error {
	if l == nil || !l.index.available() {
		return nil
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return err
	}
	return l.faceVisibility.check(ctx, query+string(encoded), func() error {
		rows, err := l.index.db.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		var dirs []string
		for rows.Next() {
			var dir string
			if err := rows.Scan(&dir); err != nil {
				rows.Close()
				return err
			}
			dirs = append(dirs, dir)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		finish := StartListTraceStep(ctx, "photos.faces.visibility", ListTraceInt("directories", len(dirs)))
		defer finish()
		visibility := newFaceDirectoryVisibility(l.root)
		for _, dir := range dirs {
			if err := ctx.Err(); err != nil {
				return err
			}
			private, err := visibility.check(dir)
			if err != nil {
				return err // An offline mount must not delete stored face data.
			}
			if private {
				if _, err := l.index.db.ExecContext(ctx, `UPDATE media_index SET admin_only=1 WHERE directory=? AND admin_only=0`, dir); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Each shared ancestor is checked once per operation, including its symlink and
// marker checks. Errors fail closed; this cache never survives an operation.
type faceDirectoryVisibility struct {
	root    string
	checked map[string]faceDirectoryCheck
}

type faceDirectoryCheck struct {
	private bool
	err     error
}

func newFaceDirectoryVisibility(root string) *faceDirectoryVisibility {
	return &faceDirectoryVisibility{root: root, checked: make(map[string]faceDirectoryCheck)}
}

func (v *faceDirectoryVisibility) private(dir string) bool {
	private, err := v.check(dir)
	return private || err != nil
}

func (v *faceDirectoryVisibility) check(dir string) (private bool, err error) {
	if cached, ok := v.checked[dir]; ok {
		return cached.private, cached.err
	}
	defer func() { v.checked[dir] = faceDirectoryCheck{private: private, err: err} }()
	clean, err := CleanPath(dir)
	if err != nil {
		return false, err
	}
	if clean != dir {
		return false, ErrPathEscapesRoot()
	}
	if dir != "" {
		if private, err := v.check(parentPath(dir)); private || err != nil {
			return private, err
		}
	}
	abs := filepath.Join(v.root, filepath.FromSlash(dir))
	var info os.FileInfo
	if dir == "" {
		info, err = os.Stat(abs) // Configured roots may themselves be symlinks.
	} else {
		info, err = os.Lstat(abs)
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, ErrPathEscapesRoot()
	}
	if !info.IsDir() {
		return false, os.ErrNotExist
	}
	marker, err := os.Stat(filepath.Join(abs, AdminOnlyMarkerName))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !marker.IsDir(), nil
}
