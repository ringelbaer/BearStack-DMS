package photos

import (
	"context"
	"database/sql"
	"time"
)

// RefreshFaceVisibility checks filesystem markers before exposing aggregate names/counts.
// No image files are decoded and shared directory ancestors are checked once per call.
func (l *Library) RefreshFaceVisibility(ctx context.Context) error {
	if l == nil || !l.index.available() {
		return nil
	}
	rows, err := l.index.db.QueryContext(ctx, `SELECT directory FROM photo_face_directories WHERE face_count>0 UNION SELECT directory FROM photo_face_job_directories WHERE job_count>0`)
	if err != nil {
		return err
	}
	var dirs []string
	for rows.Next() {
		var d string
		if err = rows.Scan(&d); err != nil {
			rows.Close()
			return err
		}
		dirs = append(dirs, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	cache := map[string]bool{}
	for _, d := range dirs {
		if err = ctx.Err(); err != nil {
			return err
		}
		abs, e := l.Resolve(d)
		private := true
		if e == nil {
			private = directoryAdminOnlyFromAbsCached(d, abs, cache)
		}
		if private {
			if _, err = l.index.db.ExecContext(ctx, `UPDATE media_index SET admin_only=1 WHERE directory=? AND admin_only=0`, d); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l *Library) PrepareFaceQueue(ctx context.Context, model string) error {
	if err := l.RefreshFaceVisibility(ctx); err != nil {
		return err
	}
	var enabled bool
	var current string
	if err := l.index.db.QueryRowContext(ctx, `SELECT enabled,model FROM photo_face_state WHERE id=1`).Scan(&enabled, &current); err != nil {
		return err
	}
	if enabled && current == model {
		return nil
	}
	if _, err := l.index.db.ExecContext(ctx, `UPDATE photo_face_state SET enabled=1,model=? WHERE id=1`, model); err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_, _ = l.index.db.ExecContext(context.WithoutCancel(ctx), `UPDATE photo_face_state SET enabled=0 WHERE id=1`)
		}
	}()
	// Keyset chunks keep transactions short even for an initial million-photo backlog.
	after := ""
	for {
		rows, err := l.index.db.QueryContext(ctx, `SELECT path,size_bytes,mod_time_unix_nano,xmp_fingerprint FROM media_index WHERE path>? AND admin_only=0 AND type='image' ORDER BY path LIMIT 1000`, after)
		if err != nil {
			return err
		}
		jobs := []FaceJob{}
		n := 0
		for rows.Next() {
			var j FaceJob
			if err = rows.Scan(&j.Path, &j.Size, &j.ModTime, &j.XMP); err != nil {
				rows.Close()
				return err
			}
			after = j.Path
			n++
			if CanThumbnail(j.Path) {
				jobs = append(jobs, j)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		tx, err := l.index.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		for _, j := range jobs {
			_, err = tx.ExecContext(ctx, `INSERT INTO photo_face_jobs(path,directory,source_size,source_mtime,source_xmp,model) VALUES(?,?,?,?,?,?) ON CONFLICT(path) DO UPDATE SET source_size=excluded.source_size,source_mtime=excluded.source_mtime,source_xmp=excluded.source_xmp,model=excluded.model,status='queued',attempts=0,retry_at=0,error='' WHERE photo_face_jobs.source_size<>excluded.source_size OR photo_face_jobs.source_mtime<>excluded.source_mtime OR photo_face_jobs.source_xmp<>excluded.source_xmp OR photo_face_jobs.model<>excluded.model`, j.Path, parentPath(j.Path), j.Size, j.ModTime, j.XMP, model)
			if err != nil {
				tx.Rollback()
				return err
			}
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		if n < 1000 {
			complete = true
			return nil
		}
	}
}

func (l *Library) NextFaceJob(ctx context.Context) (FaceJob, error) {
	var j FaceJob
	err := l.index.db.QueryRowContext(ctx, `SELECT path,source_size,source_mtime,source_xmp,model,attempts FROM photo_face_jobs WHERE status='queued' AND retry_at<=? ORDER BY retry_at,path LIMIT 1`, time.Now().Unix()).Scan(&j.Path, &j.Size, &j.ModTime, &j.XMP, &j.Model, &j.Attempts)
	return j, err
}

func (l *Library) FailFaceJob(ctx context.Context, j FaceJob, reason string) error {
	attempts := j.Attempts + 1
	status := "queued"
	if attempts >= 5 {
		status = "failed"
	}
	_, err := l.index.db.ExecContext(ctx, `UPDATE photo_face_jobs SET attempts=?,retry_at=?,status=?,error=? WHERE path=? AND source_size=? AND source_mtime=? AND model=? AND source_xmp=?`, attempts, time.Now().Add(time.Duration(1<<min(attempts, 8))*time.Minute).Unix(), status, reason, j.Path, j.Size, j.ModTime, j.Model, j.XMP)
	return err
}

func (l *Library) RetryFaceJobs(ctx context.Context) error {
	_, err := l.index.db.ExecContext(ctx, `UPDATE photo_face_jobs SET status='queued',attempts=0,retry_at=0,error='' WHERE status='failed' OR (status='queued' AND attempts>0)`)
	return err
}

func (l *Library) FaceStatus(ctx context.Context) (FaceStatus, error) {
	var s FaceStatus
	if err := l.RefreshFaceVisibility(ctx); err != nil {
		return s, err
	}
	err := l.index.db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM photo_face_jobs WHERE status='queued'),(SELECT count(*) FROM photo_face_jobs WHERE status='done'),(SELECT count(*) FROM photo_face_jobs WHERE status='failed'),(SELECT count(*) FROM photo_faces WHERE ignored=0),(SELECT count(DISTINCT person_id) FROM photo_faces WHERE ignored=0)`).Scan(&s.Queued, &s.Done, &s.Failed, &s.Faces, &s.People)
	if err != nil {
		return s, err
	}
	rows, err := l.index.db.QueryContext(ctx, `SELECT path,error,attempts FROM photo_face_jobs WHERE error<>'' ORDER BY path LIMIT 20`)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var e FaceJobError
		if err = rows.Scan(&e.Path, &e.Error, &e.Attempts); err != nil {
			return s, err
		}
		s.Errors = append(s.Errors, e)
	}
	return s, rows.Err()
}

func (l *Library) ClearFaces(ctx context.Context) error {
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"photo_faces", "photo_face_references", "photo_people", "photo_face_jobs"} {
		if _, err = tx.ExecContext(ctx, `DELETE FROM `+table); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE photo_face_state SET enabled=0 WHERE id=1`); err != nil {
		return err
	}
	l.faceRuntime.graph = nil
	l.faceRuntime.people = nil
	l.faceRuntime.nodes = nil
	return tx.Commit()
}

func (l *Library) SetFaceProcessingEnabled(ctx context.Context, enabled bool) error {
	if enabled {
		return nil
	} // PrepareFaceQueue performs the backfill before enabling incremental scheduling.
	_, err := l.index.db.ExecContext(ctx, `UPDATE photo_face_state SET enabled=0 WHERE id=1`)
	return err
}

func (s *photoIndexStore) queueFaceMediaTx(ctx context.Context, tx *sql.Tx, items []Media) error {
	var enabled bool
	var model string
	if err := tx.QueryRowContext(ctx, `SELECT enabled,model FROM photo_face_state WHERE id=1`).Scan(&enabled, &model); err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	for _, m := range items {
		if m.AdminOnly || m.Type != MediaTypeImage || !CanThumbnail(m.Path) {
			continue
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO photo_face_jobs(path,directory,source_size,source_mtime,source_xmp,model) VALUES(?,?,?,?,?,?) ON CONFLICT(path) DO UPDATE SET source_size=excluded.source_size,source_mtime=excluded.source_mtime,source_xmp=excluded.source_xmp,model=excluded.model,status='queued',attempts=0,retry_at=0,error='' WHERE photo_face_jobs.source_size<>excluded.source_size OR photo_face_jobs.source_mtime<>excluded.source_mtime OR photo_face_jobs.source_xmp<>excluded.source_xmp OR photo_face_jobs.model<>excluded.model`, m.Path, m.Directory, m.SizeBytes, m.ModTime.UnixNano(), m.XMPFingerprint, model)
		if err != nil {
			return err
		}
	}
	return nil
}
