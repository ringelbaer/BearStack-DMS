package photos

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type relocationCandidate struct {
	old, new string
	hashes   map[string]bool
}

func (l *Library) recognizePhotoRelocations(ctx context.Context) error {
	var missing bool
	if err := l.index.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_entities e WHERE kind='folder' AND NOT EXISTS(SELECT 1 FROM photo_identity_scan s WHERE s.path=e.path AND s.kind='folder'))`).Scan(&missing); err != nil || !missing {
		return err
	}
	// Unique content anchors avoid a quadratic cross product for duplicate trees.
	rows, err := l.index.db.QueryContext(ctx, `WITH old AS (
 SELECT min(e.path) path,e.kind,e.fingerprint,substr(e.path,length(rtrim(e.path,replace(e.path,'/','')))+1) name FROM photo_entities e WHERE e.kind NOT IN('folder','xmp') AND e.size_bytes>0 AND e.fingerprint<>''
 AND (e.missing_since=0 OR e.missing_since>?) AND NOT EXISTS(SELECT 1 FROM photo_identity_scan s WHERE s.path=e.path AND s.kind=e.kind)
 GROUP BY e.kind,e.fingerprint,name HAVING count(*)=1), fresh AS (
 SELECT min(path) path,kind,fingerprint,substr(path,length(rtrim(path,replace(path,'/','')))+1) name FROM photo_identity_scan WHERE kind NOT IN('folder','xmp') AND size_bytes>0 AND fingerprint<>'' GROUP BY kind,fingerprint,name HAVING count(*)=1)
 SELECT o.path,n.path,o.fingerprint FROM old o JOIN fresh n ON n.fingerprint=o.fingerprint AND n.kind=o.kind AND n.name=o.name`, time.Now().Add(-PhotoRetention).Unix())
	if err != nil {
		return err
	}
	candidates := map[string]*relocationCandidate{}
	for rows.Next() {
		var old, next, hash string
		if err = rows.Scan(&old, &next, &hash); err != nil {
			rows.Close()
			return err
		}
		a, b := strings.Split(old, "/"), strings.Split(next, "/")
		for len(a) > 0 && len(b) > 0 && a[len(a)-1] == b[len(b)-1] {
			a = a[:len(a)-1]
			b = b[:len(b)-1]
		}
		if len(a) == 0 || len(b) == 0 {
			continue
		}
		from, to := strings.Join(a, "/"), strings.Join(b, "/")
		key := from + "\x00" + to
		if candidates[key] == nil {
			candidates[key] = &relocationCandidate{from, to, map[string]bool{}}
		}
		if len(candidates[key].hashes) < 2 {
			candidates[key].hashes[hash] = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var eligible []*relocationCandidate
	oldCount, newCount := map[string]int{}, map[string]int{}
	for _, c := range candidates {
		var valid bool
		if err = l.index.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_entities e WHERE e.path=? AND e.kind='folder' AND NOT EXISTS(SELECT 1 FROM photo_identity_scan WHERE path=e.path)) AND EXISTS(SELECT 1 FROM photo_identity_scan WHERE path=? AND kind='folder')`, c.old, c.new).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			continue
		}
		if len(c.hashes) < 2 {
			start, end := prefixRange(c.old + "/")
			ns, ne := prefixRange(c.new + "/")
			var a, b int
			if err = l.index.db.QueryRowContext(ctx, `SELECT count(*) FROM photo_entities WHERE kind<>'folder' AND path>=? AND path<?`, start, end).Scan(&a); err != nil {
				return err
			}
			if err = l.index.db.QueryRowContext(ctx, `SELECT count(*) FROM photo_identity_scan WHERE kind NOT IN('folder','xmp') AND path>=? AND path<?`, ns, ne).Scan(&b); err != nil {
				return err
			}
			if a != 1 || b != 1 {
				continue
			}
		}
		eligible = append(eligible, c)
		oldCount[c.old]++
		newCount[c.new]++
	}
	// Overlapping candidates must describe the same subtree mapping. Detect
	// split/merged trees by walking ancestors, without a quadratic pair scan.
	byOld, byNew := map[string][]*relocationCandidate{}, map[string][]*relocationCandidate{}
	for _, c := range eligible {
		byOld[c.old] = append(byOld[c.old], c)
		byNew[c.new] = append(byNew[c.new], c)
	}
	contradictory := map[*relocationCandidate]bool{}
	for _, c := range eligible {
		for ancestor := parentPath(c.old); ancestor != ""; ancestor = parentPath(ancestor) {
			for _, outer := range byOld[ancestor] {
				if outer.new+strings.TrimPrefix(c.old, outer.old) != c.new {
					contradictory[c] = true
					contradictory[outer] = true
				}
			}
		}
		for ancestor := parentPath(c.new); ancestor != ""; ancestor = parentPath(ancestor) {
			for _, outer := range byNew[ancestor] {
				if outer.old+strings.TrimPrefix(c.new, outer.new) != c.old {
					contradictory[c] = true
					contradictory[outer] = true
				}
			}
		}
	}
	sort.Slice(eligible, func(i, j int) bool { return len(eligible[i].old) < len(eligible[j].old) })
	var moved []string
	for _, c := range eligible {
		if contradictory[c] || oldCount[c.old] != 1 || newCount[c.new] != 1 {
			continue
		}
		covered := false
		for _, p := range moved {
			if c.old == p || strings.HasPrefix(c.old, p+"/") {
				covered = true
			}
		}
		if covered {
			continue
		}
		var e photoEntity
		e, err = scanEntity(l.index.db.QueryRowContext(ctx, `SELECT `+entityColumns+` FROM photo_entities WHERE kind='folder' AND path=?`, c.old))
		if err != nil {
			return err
		}
		if err = l.relocatePhotoFolder(ctx, e, c.new, true); errors.Is(err, ErrLabelConflict) {
			continue
		} else if err != nil {
			return err
		}
		moved = append(moved, c.old)
	}
	return nil
}

func (l *Library) verifyRelocationTarget(ctx context.Context, old, target string) error {
	abs, err := l.Resolve(old)
	if err == nil {
		_, err = os.Lstat(abs)
	}
	if !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return ErrLabelConflict
		}
		return err
	}
	abs, err = l.Resolve(target)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return ErrLabelInvalid
	}
	start, end := prefixRange(target + "/")
	after := ""
	for {
		rows, err := l.index.db.QueryContext(ctx, `SELECT path,signature FROM photo_identity_scan WHERE path>=? AND path<? AND path>? AND kind<>'folder' ORDER BY path LIMIT 100`, start, end, after)
		if err != nil {
			return err
		}
		var items [][2]string
		for rows.Next() {
			var v [2]string
			if err = rows.Scan(&v[0], &v[1]); err != nil {
				rows.Close()
				return err
			}
			items = append(items, v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, v := range items {
			abs, err := l.Resolve(v[0])
			if err != nil {
				return err
			}
			info, err := os.Lstat(abs)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || identityStat(info) != v[1] {
				return ErrLabelConflict
			}
			after = v[0]
		}
		if len(items) < 100 {
			return nil
		}
	}
}

func (l *Library) relocatePhotoFolder(ctx context.Context, source photoEntity, target string, automatic bool) error {
	clean, err := CleanPath(target)
	if err != nil || clean == "" || clean != target || source.Path == target || strings.HasPrefix(target, source.Path+"/") || strings.HasPrefix(source.Path, target+"/") {
		return ErrLabelInvalid
	}
	if err = l.verifyRelocationTarget(ctx, source.Path, target); err != nil {
		return err
	}
	if err = l.prepareRelocationVisibility(ctx, source.Path, target); err != nil {
		return err
	}
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var revision int64
	if err = tx.QueryRowContext(ctx, `SELECT revision FROM photo_entities WHERE id=?`, source.ID).Scan(&revision); err != nil {
		return err
	}
	if revision != source.Revision {
		return ErrLabelConflict
	}
	ss, se := prefixRange(source.Path + "/")
	start, end := prefixRange(target + "/")
	var conflict bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_entities WHERE (path=? OR (path>=? AND path<?)) AND manual_revision>0)
 OR EXISTS(SELECT 1 FROM photo_faces f JOIN photo_people p ON p.id=f.person_id WHERE (f.path>=? AND f.path<?) AND (f.manual=1 OR f.favorite=1 OR f.ignored=1 OR f.drawn=1 OR (p.manual_name=1 AND NOT EXISTS(SELECT 1 FROM photo_faces old WHERE old.person_id=p.id AND old.path>=? AND old.path<?) AND NOT EXISTS(SELECT 1 FROM photo_retained_photo_faces old WHERE old.person_id=p.id AND old.path>=? AND old.path<?))))`, target, start, end, start, end, ss, se, ss, se).Scan(&conflict); err != nil {
		return err
	}
	if conflict {
		return ErrLabelConflict
	}
	var referencePending, referenceCursor int64
	if err = tx.QueryRowContext(ctx, `SELECT pending,cursor FROM photo_face_reference_settings WHERE id=1`).Scan(&referencePending, &referenceCursor); err != nil {
		return err
	}
	if err = relocationPeopleTx(ctx, tx, source.Path, target); err != nil {
		return err
	}
	// Drop only unedited destination duplicates. Additions at the destination
	// remain independent; source IDs always win the proven one-to-one mapping.
	if err = eachPhotoEntity(ctx, tx, source.Path, func(e photoEntity) error {
		next := target + strings.TrimPrefix(e.Path, source.Path)
		dest, err := scanEntity(tx.QueryRowContext(ctx, `SELECT `+entityColumns+` FROM photo_entities WHERE path=? AND kind=?`, next, e.Kind))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err = l.retireMediaCachesTx(ctx, tx, dest); err != nil {
			return err
		}
		return l.index.removeUneditedEntityTx(ctx, tx, dest)
	}); err != nil {
		return err
	}
	for _, spec := range retainedTables {
		for _, table := range []string{spec.table, "photo_retained_" + spec.table} {
			if _, err = tx.ExecContext(ctx, `UPDATE `+table+` SET `+spec.key+`=?||substr(`+spec.key+`,length(?)+1) WHERE `+spec.key+`=? OR (`+spec.key+`>=? AND `+spec.key+`<?)`, target, source.Path, source.Path, ss, se); err != nil {
				return err
			}
			if spec.table == "media_index" || spec.table == "blog_index" || spec.table == "gpx_index" || spec.table == "photo_faces" || spec.table == "photo_face_jobs" || spec.table == "folder_index" {
				col := "directory"
				if spec.table == "folder_index" {
					col = "parent"
				}
				if _, err = tx.ExecContext(ctx, `UPDATE `+table+` SET `+col+`=?||substr(`+col+`,length(?)+1) WHERE `+col+`=? OR (`+col+`>=? AND `+col+`<?)`, target, source.Path, source.Path, ss, se); err != nil {
					return err
				}
			}
		}
	}
	for _, table := range []string{"folder_index", "photo_retained_folder_index"} {
		if _, err = tx.ExecContext(ctx, `UPDATE `+table+` SET parent=?,name=? WHERE path=?`, parentPath(target), filepath.Base(target), target); err != nil {
			return err
		}
	}
	for _, pair := range [][2]string{{"photo_entities", "path"}, {"photo_xmp_people", "path"}, {"photo_people", "name_source"}, {"photo_hidden_person_names", "name_source"}} {
		if _, err = tx.ExecContext(ctx, `UPDATE `+pair[0]+` SET `+pair[1]+`=?||substr(`+pair[1]+`,length(?)+1) WHERE `+pair[1]+`=? OR (`+pair[1]+`>=? AND `+pair[1]+`<?)`, target, source.Path, source.Path, ss, se); err != nil {
			return err
		}
	}
	// Changed sources are ineligible already when the moved paths become visible.
	for _, table := range []string{"photo_faces", "photo_retained_photo_faces"} {
		if _, err = tx.ExecContext(ctx, `UPDATE `+table+` AS f SET needs_review=1,source_revision=source_revision+1 FROM photo_identity_scan s WHERE f.path=s.path AND f.entity_id IN(SELECT id FROM photo_relocation_plan) AND f.source_hash<>s.fingerprint`); err != nil {
			return err
		}
	}
	missing := retentionSelection{where: `id IN(SELECT id FROM photo_relocation_plan) AND NOT EXISTS(SELECT 1 FROM photo_identity_scan s WHERE s.path=photo_entities.path AND s.kind=photo_entities.kind)`}
	if err = l.index.retainSelectionTx(ctx, tx, missing); err != nil {
		return err
	}
	for _, spec := range retainedTables {
		if _, err = tx.ExecContext(ctx, `DELETE FROM `+spec.table+` WHERE `+spec.key+` IN(SELECT path FROM photo_entities WHERE missing_since>0 AND id IN(SELECT id FROM photo_relocation_plan))`); err != nil {
			return err
		}
	}
	for _, table := range []string{"media_search", "folder_search", "blog_search"} {
		if _, err = tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE path=? OR (path>=? AND path<?)`, source.Path, ss, se); err != nil {
			return err
		}
	}
	// Visibility changes and face archival are published with the new paths.
	for _, spec := range []struct{ table, kind string }{{"media_index", ""}, {"folder_index", "folder"}, {"blog_index", "blog"}, {"gpx_index", "gpx"}} {
		if _, err = tx.ExecContext(ctx, `UPDATE `+spec.table+` AS t SET admin_only=p.admin_only FROM photo_relocation_plan p WHERE t.path=p.path AND t.admin_only<>p.admin_only`); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE photo_retained_media_index AS t SET admin_only=p.admin_only FROM photo_relocation_plan p WHERE t.path=p.path`); err != nil {
		return err
	}
	if err = eachPhotoEntity(ctx, tx, target, func(e photoEntity) error {
		if e.MissingSince > 0 {
			return nil
		}
		return l.index.refreshEntitySearchTx(ctx, tx, e)
	}); err != nil {
		return err
	}
	if err = invalidateIdentityFoldersTx(ctx, tx, true, source.Path, target); err != nil {
		return err
	}
	if err = refreshSelectedFaceReferencesTx(ctx, tx, `SELECT person_id FROM relocation_people`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DROP TABLE relocation_people; UPDATE photo_face_reference_settings SET pending=?,cursor=? WHERE id=1; UPDATE photo_face_state SET revision=revision+1`, referencePending, referenceCursor); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE photo_entities SET revision=revision+1 WHERE id=?`, source.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO photo_relocations(entity_id,old_path,new_path,created_at,automatic) VALUES(?,?,?,?,?)`, source.ID, source.Path, target, time.Now().Unix(), automatic); err != nil {
		return err
	}
	if err = tx.Commit(); err == nil {
		l.faceRuntime.graph = nil
		l.faceSuggestions.clear()
	}
	return err
}

func (s *photoIndexStore) removeUneditedEntityTx(ctx context.Context, tx *sql.Tx, e photoEntity) error {
	if e.MissingSince > 0 {
		return ErrLabelConflict
	}
	for _, table := range []string{"media_search", "folder_search", "blog_search"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE path=?`, e.Path); err != nil {
			return err
		}
	}
	for _, spec := range retainedTables {
		if retainedForKind(spec, e.Kind) {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+spec.table+` WHERE `+spec.key+`=?`, e.Path); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE photo_entities SET missing_since=unixepoch() WHERE id=?`, e.ID); err != nil {
		return err
	}
	return s.purgeEntityTx(ctx, tx, e)
}

// Walk a subtree in bounded batches; SQLite, not Go, stores the relocation plan.
type entityQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func eachPhotoEntity(ctx context.Context, q entityQuerier, path string, fn func(photoEntity) error) error {
	start, end := prefixRange(path + "/")
	var after int64
	for {
		rows, err := q.QueryContext(ctx, `SELECT `+entityColumns+` FROM photo_entities WHERE (path=? OR(path>=? AND path<?)) AND id>? ORDER BY id LIMIT 100`, path, start, end, after)
		if err != nil {
			return err
		}
		var entries []photoEntity
		for rows.Next() {
			e, err := scanEntity(rows)
			if err != nil {
				rows.Close()
				return err
			}
			entries = append(entries, e)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err = fn(e); err != nil {
				return err
			}
			after = e.ID
		}
		if len(entries) < 100 {
			return nil
		}
	}
}
func (l *Library) prepareRelocationVisibility(ctx context.Context, from, to string) error {
	if _, err := l.index.db.ExecContext(ctx, `DELETE FROM photo_relocation_plan`); err != nil {
		return err
	}
	return eachPhotoEntity(ctx, l.index.db, from, func(e photoEntity) error {
		path := to + strings.TrimPrefix(e.Path, from)
		dir := parentPath(path)
		if e.Kind == "folder" {
			dir = path
		}
		// Deleted descendants inherit the closest existing ancestor's restrictions.
		var private bool
		for {
			var err error
			private, err = l.FolderAdminOnly(dir)
			if err == nil {
				break
			}
			if !errors.Is(err, os.ErrNotExist) || dir == "" {
				return err
			}
			dir = parentPath(dir)
		}
		_, err := l.index.db.ExecContext(ctx, `INSERT INTO photo_relocation_plan(id,path,admin_only) VALUES(?,?,?)`, e.ID, path, private)
		return err
	})
}
