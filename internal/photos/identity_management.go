package photos

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"time"
)

type RetainedPhoto struct {
	ID           int64     `json:"id"`
	Kind         string    `json:"kind"`
	Path         string    `json:"path"`
	Revision     int64     `json:"revision"`
	MissingSince time.Time `json:"missing_since"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type PhotoRelocation struct {
	ID        int64     `json:"id"`
	OldPath   string    `json:"old_path"`
	NewPath   string    `json:"new_path"`
	At        time.Time `json:"at"`
	Automatic bool      `json:"automatic"`
}
type PhotoIdentityStatus struct {
	Files          int64             `json:"files"`
	Fingerprinted  int64             `json:"fingerprinted"`
	ReviewRequired int64             `json:"review_required"`
	Missing        []RetainedPhoto   `json:"missing"`
	Relocations    []PhotoRelocation `json:"relocations"`
	Next           int64             `json:"next"`
	HasNext        bool              `json:"has_next"`
}

func (l *Library) PhotoIdentities(ctx context.Context, after int64) (PhotoIdentityStatus, error) {
	out := PhotoIdentityStatus{Missing: []RetainedPhoto{}, Relocations: []PhotoRelocation{}}
	if l == nil || !l.index.available() {
		return out, sql.ErrNoRows
	}
	if err := l.index.db.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(fingerprint<>''),0) FROM photo_entities WHERE kind<>'folder' AND missing_since=0`).Scan(&out.Files, &out.Fingerprinted); err != nil {
		return out, err
	}
	if err := l.index.db.QueryRowContext(ctx, `SELECT count(*) FROM photo_faces WHERE needs_review=1`).Scan(&out.ReviewRequired); err != nil {
		return out, err
	}
	rows, err := l.index.db.QueryContext(ctx, `SELECT id,kind,path,revision,missing_since FROM photo_entities WHERE missing_since>0 AND id>? ORDER BY id LIMIT 101`, after)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var item RetainedPhoto
		var missing int64
		if err = rows.Scan(&item.ID, &item.Kind, &item.Path, &item.Revision, &missing); err != nil {
			rows.Close()
			return out, err
		}
		item.MissingSince = time.Unix(missing, 0)
		item.ExpiresAt = item.MissingSince.Add(PhotoRetention)
		out.Missing = append(out.Missing, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Missing) > 100 {
		out.Missing = out.Missing[:100]
		out.HasNext = true
		out.Next = out.Missing[99].ID
	}
	rows, err = l.index.db.QueryContext(ctx, `SELECT id,old_path,new_path,created_at,automatic FROM photo_relocations ORDER BY id DESC LIMIT 100`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item PhotoRelocation
		var at int64
		if err = rows.Scan(&item.ID, &item.OldPath, &item.NewPath, &at, &item.Automatic); err != nil {
			return out, err
		}
		item.At = time.Unix(at, 0)
		out.Relocations = append(out.Relocations, item)
	}
	return out, rows.Err()
}

func (l *Library) ResolvePhotoRelocation(ctx context.Context, id, revision int64, target string) error {
	if err := l.lockPhotoIdentities(ctx); err != nil {
		return err
	}
	e, err := scanEntity(l.index.db.QueryRowContext(ctx, `SELECT `+entityColumns+` FROM photo_entities WHERE id=? AND kind='folder' AND missing_since>0`, id))
	if err == nil && (e.Revision != revision || time.Now().Unix() >= e.MissingSince+int64(PhotoRetention/time.Second)) {
		err = ErrLabelConflict
	}
	if err == nil {
		var complete bool
		complete, err = l.scanPhotoIdentities(ctx)
		if err == nil && !complete {
			err = ErrLabelConflict
		}
	}
	if err == nil {
		err = l.relocatePhotoFolder(ctx, e, target, false)
	}
	if err == nil {
		err = l.restorePresentEntities(ctx)
	}
	l.identityMu.Unlock()
	if err != nil {
		return err
	}
	_, err = l.RebuildIndex(ctx)
	return err
}

func (l *Library) ForgetRetainedPhoto(ctx context.Context, id, revision int64) error {
	if err := l.lockPhotoIdentities(ctx); err != nil {
		return err
	}
	defer l.identityMu.Unlock()
	e, err := scanEntity(l.index.db.QueryRowContext(ctx, `SELECT `+entityColumns+` FROM photo_entities WHERE id=? AND missing_since>0`, id))
	if err != nil {
		return err
	}
	if e.Revision != revision {
		return ErrLabelConflict
	}
	// Manual deletion also refuses a returning source, preventing a stale dialog
	// from discarding data which can now be restored.
	abs, err := l.Resolve(e.Path)
	if err == nil {
		_, err = os.Lstat(abs)
	}
	if !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return ErrLabelConflict
		}
		return err
	}
	start, end := prefixRange(e.Path + "/")
	after := int64(0)
	for {
		rows, err := l.index.db.QueryContext(ctx, `SELECT `+entityColumns+` FROM photo_entities WHERE missing_since>0 AND (id=? OR (?='folder' AND path>=? AND path<?)) AND id>? ORDER BY id LIMIT 100`, e.ID, e.Kind, start, end, after)
		if err != nil {
			return err
		}
		var entries []photoEntity
		for rows.Next() {
			v, err := scanEntity(rows)
			if err != nil {
				rows.Close()
				return err
			}
			entries = append(entries, v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, v := range entries {
			if err = l.purgeRetainedEntity(ctx, v); err != nil {
				return err
			}
			after = v.ID
		}
		if len(entries) < 100 {
			break
		}
	}
	return l.processPhotoCacheGC(ctx)
}

// Backfill is independent of the optional crawler and visits only previously
// indexed files. A moving keyset cursor prevents inaccessible paths starving
// the rest of a collection. The caller runs a single background worker.
func (l *Library) BackfillPhotoIdentities(ctx context.Context, after int64) (int64, error) {
	var next int64
	_, err := withLowIndexPriority(func() (IndexStats, error) {
		var err error
		next, err = l.backfillPhotoIdentities(ctx, after)
		return IndexStats{}, err
	})
	return next, err
}
func (l *Library) backfillPhotoIdentities(ctx context.Context, after int64) (int64, error) {
	if l == nil || !l.index.available() {
		return 0, nil
	}
	if err := l.lockPhotoIdentities(ctx); err != nil {
		return after, err
	}
	defer l.identityMu.Unlock()
	rows, err := l.index.db.QueryContext(ctx, `SELECT `+entityColumns+` FROM photo_entities WHERE fingerprint='' AND kind<>'folder' AND missing_since=0 AND id>? ORDER BY id LIMIT 100`, after)
	if err != nil {
		return after, err
	}
	var entries []photoEntity
	for rows.Next() {
		e, err := scanEntity(rows)
		if err != nil {
			rows.Close()
			return after, err
		}
		entries = append(entries, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return after, err
	}
	for _, e := range entries {
		if err = ctx.Err(); err != nil {
			return after, err
		}
		after = e.ID
		abs, err := l.Resolve(e.Path)
		if err != nil {
			continue
		}
		info, err := os.Lstat(abs)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		hash, err := hashPhotoFile(ctx, abs, info)
		if err != nil {
			if ctx.Err() != nil {
				return after, ctx.Err()
			}
			continue
		}
		tx, err := l.index.db.BeginTx(ctx, nil)
		if err != nil {
			return after, err
		}
		_, err = tx.ExecContext(ctx, `UPDATE photo_entities SET fingerprint=?,size_bytes=?,mtime=? WHERE id=? AND fingerprint='' AND missing_since=0`, hash, info.Size(), info.ModTime().UnixNano(), e.ID)
		if err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE photo_faces SET source_hash=? WHERE entity_id=? AND source_hash='' AND needs_review=0 AND source_size=? AND source_mtime=?`, hash, e.ID, info.Size(), info.ModTime().UnixNano())
		}
		if err == nil {
			err = tx.Commit()
		} else {
			tx.Rollback()
		}
		if err != nil {
			return after, err
		}
	}
	if len(entries) < 100 {
		return 0, nil
	}
	return after, nil
}

// AddContentStates joins the requested media and preview groups in shared batches.
// It never reads originals or walks thumbnail directories.
func (l *Library) AddContentStates(ctx context.Context, groups ...[]Media) error {
	if l == nil || !l.index.available() {
		return nil
	}
	positions := map[string][]*Media{}
	paths := []string{}
	for _, items := range groups {
		for i := range items {
			path := items[i].Path
			if path == "" { // Virtual face previews have no media identity.
				continue
			}
			if _, exists := positions[path]; !exists {
				paths = append(paths, path)
			}
			positions[path] = append(positions[path], &items[i])
		}
	}
	for start := 0; start < len(paths); start += 200 {
		end := min(start+200, len(paths))
		args := make([]any, end-start)
		for i, path := range paths[start:end] {
			args[i] = path
		}
		finish := StartListTraceStep(ctx, "photos.identity.content_states_batch", ListTraceInt("paths", len(args)))
		rows, err := l.index.db.QueryContext(ctx, `SELECT id,path,revision,EXISTS(SELECT 1 FROM photo_faces f WHERE f.entity_id=e.id AND f.needs_review=1) FROM photo_entities e WHERE missing_since=0 AND kind IN('image','video','audio') AND path IN (`+strings.TrimRight(strings.Repeat("?,", len(args)), ",")+`)`, args...)
		finish()
		if err != nil {
			return err
		}
		for rows.Next() {
			var id, rev int64
			var path string
			var review bool
			if err = rows.Scan(&id, &path, &rev, &review); err != nil {
				rows.Close()
				return err
			}
			for _, item := range positions[path] {
				item.EntityID = id
				item.ContentRevision = rev
				item.NeedsReview = review
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (l *Library) lockPhotoIdentities(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if l.identityMu.TryLock() {
			return nil
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
