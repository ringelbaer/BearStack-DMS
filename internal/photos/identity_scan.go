package photos

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type identityFile struct {
	path, dir, kind, hash, signature string
	size, mtime                      int64
}

// Include inode and change time where the OS exposes them. Access time is
// deliberately excluded: reading a file must not invalidate its fingerprint.
func identityStat(info os.FileInfo) string {
	s := fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano())
	v := reflect.ValueOf(info.Sys())
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.IsValid() && v.Kind() == reflect.Struct {
		for _, field := range []string{"Dev", "Ino", "Ctim", "Ctimespec"} {
			f := v.FieldByName(field)
			if f.IsValid() && f.CanInterface() {
				s += fmt.Sprintf(":%v", f.Interface())
			}
		}
	}
	return s
}

func hashPhotoFile(ctx context.Context, abs string, before os.FileInfo) (string, error) {
	f, err := os.Open(abs)
	if err != nil {
		return "", err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) || identityStat(before) != identityStat(opened) {
		return "", ErrLabelConflict
	}
	h := sha256.New()
	buf := make([]byte, 256<<10)
	for {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		n, e := f.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", e
		}
	}
	after, err := os.Lstat(abs)
	if err != nil {
		return "", err
	}
	if !after.Mode().IsRegular() || !os.SameFile(opened, after) || identityStat(opened) != identityStat(after) {
		return "", ErrLabelConflict
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// The disk-backed inventory survives cancellation. Unchanged directories do
// not issue inventory writes or open file contents; changed directories reuse
// per-file fingerprints. Only a complete walk may drive automatic relocation.
func (l *Library) scanPhotoIdentities(ctx context.Context) (bool, error) {
	seen := map[string]bool{}
	complete := true
	var walk func(string) error
	walk = func(rel string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		abs, err := l.Resolve(rel)
		if err != nil {
			return err
		}
		entries, err := os.ReadDir(abs)
		if err != nil {
			return err
		}
		seen[rel] = true
		if rel == "" && len(entries) == 0 {
			var known bool
			if err = l.index.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_entities)`).Scan(&known); err != nil {
				return err
			}
			if known {
				complete = false
				return nil
			}
		}
		h := fnv.New128a()
		var files []identityFile
		var dirs []string
		for _, entry := range entries {
			if ignoredName(entry.Name()) || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			path := joinPath(rel, entry.Name())
			if entry.IsDir() {
				dirs = append(dirs, path)
				continue
			}
			kind, ok := supportedKind(entry.Name())
			if !ok {
				if strings.EqualFold(filepath.Ext(entry.Name()), ".xmp") {
					kind = "xmp"
				} else {
					continue
				}
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				continue
			}
			f := identityFile{path: path, dir: rel, kind: kind, size: info.Size(), mtime: info.ModTime().UnixNano(), signature: identityStat(info)}
			fmt.Fprintf(h, "%s\x00%s\x00%s\x00", f.path, f.kind, f.signature)
			files = append(files, f)
		}
		signature := fmt.Sprintf("%x", h.Sum(nil))
		var previous string
		err = l.index.db.QueryRowContext(ctx, `SELECT signature FROM photo_identity_directories WHERE path=?`, rel).Scan(&previous)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if previous != signature {
			cached := map[string]identityFile{}
			rows, err := l.index.db.QueryContext(ctx, `SELECT path,fingerprint,signature FROM photo_identity_scan WHERE directory=? AND kind<>'folder'`, rel)
			if err != nil {
				return err
			}
			for rows.Next() {
				var f identityFile
				if err = rows.Scan(&f.path, &f.hash, &f.signature); err != nil {
					rows.Close()
					return err
				}
				cached[f.path] = f
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
			if _, err = l.index.db.ExecContext(ctx, `INSERT OR IGNORE INTO photo_identity_directories(path,signature) VALUES(?,'')`, rel); err != nil {
				return err
			}
			// Persist verified blocks independently of directory publication so a
			// cancelled scan resumes even inside a very large new directory.
			pending := make([]identityFile, 0, 32)
			for i := range files {
				f := &files[i]
				old, ok := cached[f.path]
				delete(cached, f.path)
				if ok && old.signature == f.signature {
					f.hash = old.hash
					continue
				}
				info, err := os.Lstat(filepath.Join(abs, filepath.Base(f.path)))
				if err != nil {
					return err
				}
				if identityStat(info) != f.signature {
					return ErrLabelConflict
				}
				f.hash, err = hashPhotoFile(ctx, filepath.Join(abs, filepath.Base(f.path)), info)
				if err != nil {
					return err
				}
				pending = append(pending, *f)
				if len(pending) == 32 {
					if err = l.persistIdentityFiles(ctx, pending); err != nil {
						return err
					}
					pending = pending[:0]
				}
			}
			if err = l.persistIdentityFiles(ctx, pending); err != nil {
				return err
			}
			tx, err := l.index.db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			for path := range cached {
				if _, err = tx.ExecContext(ctx, `DELETE FROM photo_identity_scan WHERE path=?`, path); err != nil {
					break
				}
			}
			if err == nil {
				_, err = tx.ExecContext(ctx, `INSERT INTO photo_identity_directories(path,signature) VALUES(?,?) ON CONFLICT(path) DO UPDATE SET signature=excluded.signature`, rel, signature)
			}
			if err == nil {
				err = tx.Commit()
			} else {
				tx.Rollback()
			}
			if err != nil {
				return err
			}
		}
		if rel != "" {
			if _, err = l.index.db.ExecContext(ctx, `INSERT OR IGNORE INTO photo_identity_scan(path,directory,kind,fingerprint,size_bytes,mtime,signature) VALUES(?,?,'folder','',0,0,'')`, rel, parentPath(rel)); err != nil {
				return err
			}
		}
		for _, dir := range dirs {
			if err := walk(dir); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				complete = false
			}
		}
		return nil
	}
	if err := walk(""); err != nil {
		return false, err
	}
	if !complete {
		return false, nil
	}
	rows, err := l.index.db.QueryContext(ctx, `SELECT path FROM photo_identity_directories`)
	if err != nil {
		return false, err
	}
	var stale []string
	for rows.Next() {
		var dir string
		if err = rows.Scan(&dir); err != nil {
			rows.Close()
			return false, err
		}
		if !seen[dir] {
			stale = append(stale, dir)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	for _, dir := range stale {
		if _, err = l.index.db.ExecContext(ctx, `DELETE FROM photo_identity_scan WHERE directory=? OR path=?; DELETE FROM photo_identity_directories WHERE path=?`, dir, dir, dir); err != nil {
			return false, err
		}
	}
	return true, nil
}

// No original reads on request paths. Until the background verifier catches up,
// changed metadata conservatively sets needs_review rather than deleting faces.
func (l *Library) publishIdentityFingerprints(ctx context.Context) error {
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE photo_thumbnail_index AS t SET content_verified=1,source_size_bytes=s.size_bytes,source_mod_time_unix_nano=s.mtime FROM photo_identity_scan s JOIN photo_entities e ON e.path=s.path AND e.kind=s.kind WHERE t.media_path=s.path AND e.fingerprint<>'' AND e.fingerprint=s.fingerprint AND t.source_size_bytes=e.size_bytes AND t.source_mod_time_unix_nano=e.mtime AND (e.size_bytes<>s.size_bytes OR e.mtime<>s.mtime)`); err != nil {
		return err
	}
	// Content changes invalidate derived media caches even when size/mtime were
	// preserved by an external editor. Hashing itself happened before this tx.
	if err = l.invalidateChangedContentTx(ctx, tx); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE photo_faces AS f SET needs_review=0,source_revision=source_revision+1 FROM photo_identity_scan s WHERE f.path=s.path AND f.source_hash<>'' AND f.source_hash=s.fingerprint AND f.needs_review=1`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE photo_entities AS e SET fingerprint=s.fingerprint,size_bytes=s.size_bytes,mtime=s.mtime,revision=e.revision+CASE WHEN e.fingerprint<>'' AND e.fingerprint<>s.fingerprint THEN 1 ELSE 0 END
 FROM photo_identity_scan s WHERE e.path=s.path AND e.kind=s.kind AND e.missing_since=0 AND (e.fingerprint<>s.fingerprint OR e.size_bytes<>s.size_bytes OR e.mtime<>s.mtime)`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE photo_faces SET source_hash=(SELECT fingerprint FROM photo_entities WHERE id=entity_id) WHERE source_hash='' AND needs_review=0 AND EXISTS(SELECT 1 FROM photo_entities e WHERE e.id=entity_id AND e.size_bytes=source_size AND e.mtime=source_mtime AND e.fingerprint<>'')`); err != nil {
		return err
	}
	return tx.Commit()
}

func (l *Library) persistIdentityFiles(ctx context.Context, files []identityFile) error {
	if len(files) == 0 {
		return nil
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, f := range files {
		if _, err = tx.ExecContext(ctx, `INSERT INTO photo_identity_scan(path,directory,kind,fingerprint,size_bytes,mtime,signature) VALUES(?,?,?,?,?,?,?) ON CONFLICT(path) DO UPDATE SET directory=excluded.directory,kind=excluded.kind,fingerprint=excluded.fingerprint,size_bytes=excluded.size_bytes,mtime=excluded.mtime,signature=excluded.signature`, f.path, f.dir, f.kind, f.hash, f.size, f.mtime, f.signature); err != nil {
			return err
		}
	}
	return tx.Commit()
}
