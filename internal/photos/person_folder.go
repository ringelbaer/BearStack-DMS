package photos

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path"
	"strings"
	"time"

	"bearstack/internal/searchtext"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var ErrPersonFolderExcluded = errors.New("Dieser Ordner ist für die Person ausgeschlossen. Bitte den Ordner zuerst freigeben.")

// IsPersonFolderExcluded also recognizes the database guard used by all writers.
func IsPersonFolderExcluded(err error) bool {
	var sqlErr *sqlite.Error
	return errors.Is(err, ErrPersonFolderExcluded) || (errors.As(err, &sqlErr) && sqlErr.Code() == sqlite3.SQLITE_CONSTRAINT_TRIGGER && strings.Contains(sqlErr.Error(), ErrPersonFolderExcluded.Error()))
}

func setupPersonFolderSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS person_folder_exclusions (
 person_id INTEGER NOT NULL, directory TEXT NOT NULL, PRIMARY KEY(person_id,directory)) WITHOUT ROWID;
 CREATE INDEX IF NOT EXISTS idx_person_folder_exclusions_directory ON person_folder_exclusions(directory,person_id);
 CREATE INDEX IF NOT EXISTS idx_faces_person_folder ON photo_faces(person_id,directory,ignored,id);
 CREATE INDEX IF NOT EXISTS idx_faces_folder_person ON photo_faces(directory,person_id);
 CREATE TRIGGER IF NOT EXISTS person_folder_revision_insert AFTER INSERT ON person_folder_exclusions BEGIN
 UPDATE photo_person_revisions SET revision=revision+1 WHERE person_id=new.person_id; END;
 CREATE TRIGGER IF NOT EXISTS person_folder_revision_delete AFTER DELETE ON person_folder_exclusions BEGIN
 UPDATE photo_person_revisions SET revision=revision+1 WHERE person_id=old.person_id; END;
 CREATE TRIGGER IF NOT EXISTS person_folder_delete AFTER DELETE ON photo_people BEGIN
 DELETE FROM person_folder_exclusions WHERE person_id=old.id; END;
 CREATE TRIGGER IF NOT EXISTS person_folder_insert_guard BEFORE INSERT ON photo_faces
 WHEN EXISTS(SELECT 1 FROM person_folder_exclusions WHERE person_id=new.person_id AND directory=new.directory)
 BEGIN SELECT RAISE(ABORT,'Dieser Ordner ist für die Person ausgeschlossen. Bitte den Ordner zuerst freigeben.'); END;
 CREATE TRIGGER IF NOT EXISTS person_folder_update_guard BEFORE UPDATE OF person_id,directory,ignored ON photo_faces
 WHEN (new.person_id<>old.person_id OR new.directory<>old.directory OR (new.ignored=0 AND old.ignored<>0)) AND EXISTS(SELECT 1 FROM person_folder_exclusions WHERE person_id=new.person_id AND directory=new.directory)
 BEGIN SELECT RAISE(ABORT,'Dieser Ordner ist für die Person ausgeschlossen. Bitte den Ordner zuerst freigeben.'); END;`)
	return err
}

type PersonFolder struct {
	Faces       []LabelFace `json:"faces,omitempty"`
	Directory   string      `json:"directory"`
	DisplayPath string      `json:"display_path"`
	FaceIDs     []int64     `json:"face_ids"`
	Count       int         `json:"count"`
	Excluded    bool        `json:"excluded"`
}
type PersonFolderPage struct {
	PersonID int64          `json:"person_id"`
	Name     string         `json:"name"`
	HasFaces bool           `json:"-"`
	Revision int64          `json:"revision"`
	Page     int            `json:"page"`
	HasNext  bool           `json:"has_next"`
	Folders  []PersonFolder `json:"folders"`
}

// Paging bounds thumbnails and HTML even for people spanning thousands of folders.
// The covering person/directory index serves both grouping and each eight-face preview.
func (l *Library) PersonFolders(ctx context.Context, id int64, page int) (PersonFolderPage, error) {
	return l.personFolders(ctx, id, page, false)
}

// LabelPersonFolders includes bounded original-preview metadata in the same snapshot.
func (l *Library) LabelPersonFolders(ctx context.Context, id int64, page int) (PersonFolderPage, error) {
	return l.personFolders(ctx, id, page, true)
}

func (l *Library) personFolders(ctx context.Context, id int64, page int, previews bool) (PersonFolderPage, error) {
	out := PersonFolderPage{PersonID: id, Page: max(1, page), Folders: []PersonFolder{}}
	if err := l.refreshPersonIDsVisibility(ctx, id); err != nil {
		return out, err
	}
	tx, err := l.index.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	// Check live directory visibility before exposing even the person's name.
	visibleRows, err := tx.QueryContext(ctx, `SELECT directory FROM photo_faces f WHERE person_id=? AND ignored=0 AND EXISTS(SELECT 1 FROM media_index m WHERE m.path=f.path AND m.admin_only=0) GROUP BY directory UNION SELECT directory FROM person_folder_exclusions WHERE person_id=?`, id, id)
	if err != nil {
		return out, err
	}
	visibility := newFaceDirectoryVisibility(l.root)
	visible := false
	for visibleRows.Next() {
		var directory string
		if err = visibleRows.Scan(&directory); err != nil {
			break
		}
		if !visibility.private(directory) {
			visible = true
			break
		}
	}
	if err == nil {
		err = visibleRows.Err()
	}
	visibleRows.Close()
	if err != nil {
		return out, err
	}
	if !visible {
		return out, sql.ErrNoRows
	}
	if err = tx.QueryRowContext(ctx, `SELECT p.name,v.revision,`+visiblePersonSQL+` FROM photo_people p JOIN photo_person_revisions v ON v.person_id=p.id WHERE p.id=?`, id).Scan(&out.Name, &out.Revision, &out.HasFaces); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT directory,count(*),0 FROM photo_faces f WHERE person_id=? AND ignored=0
 AND EXISTS(SELECT 1 FROM media_index m WHERE m.path=f.path AND m.admin_only=0)
 GROUP BY directory UNION ALL SELECT directory,0,1 FROM person_folder_exclusions WHERE person_id=? ORDER BY directory LIMIT 41 OFFSET ?`, id, id, (out.Page-1)*40)
	if err != nil {
		return out, err
	}
	rawCount := 0
	for rows.Next() {
		var f PersonFolder
		if err = rows.Scan(&f.Directory, &f.Count, &f.Excluded); err != nil {
			break
		}
		rawCount++
		if rawCount > 40 {
			break
		}
		if visibility.private(f.Directory) {
			continue
		}
		f.DisplayPath = strings.TrimSuffix(MediaDisplayPath(path.Join(f.Directory, ".preview")), " / .preview")
		f.FaceIDs = []int64{}
		out.Folders = append(out.Folders, f)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	out.HasNext = rawCount > 40
	for i := range out.Folders {
		f := &out.Folders[i]
		if f.Excluded {
			continue
		}
		columns := "f.id"
		if previews {
			columns = `f.id,f.path,f.x,f.y,f.width,f.height,f.favorite,f.needs_review,f.source_revision,m.size_bytes,m.mod_time_unix_nano,coalesce((SELECT revision FROM photo_entities e WHERE e.id=f.entity_id),0)`
		}
		rows, err := tx.QueryContext(ctx, `SELECT `+columns+` FROM photo_faces f JOIN media_index m ON m.path=f.path WHERE f.person_id=? AND f.directory=? AND f.ignored=0 AND m.admin_only=0 ORDER BY f.id LIMIT 8`, id, f.Directory)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var row indexedLabelFace
			if previews {
				err = rows.Scan(&row.Face.ID, &row.Path, &row.Face.Bounds.X, &row.Face.Bounds.Y, &row.Face.Bounds.Width, &row.Face.Bounds.Height, &row.Face.Favorite, &row.Face.NeedsReview, &row.Face.SourceRevision, &row.Size, &row.Modified, &row.ContentRevision)
			} else {
				err = rows.Scan(&row.Face.ID)
			}
			if err != nil {
				break
			}
			f.FaceIDs = append(f.FaceIDs, row.Face.ID)
			if previews {
				f.Faces = append(f.Faces, presentLabelFace(row))
			}
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

type PersonFolderAction struct {
	Directory      string
	Action         string
	Revision       int64
	TargetRevision int64
	TargetID       int64
	Name           string
}

func validateLabelFolderAction(a LabelAction, name string) error {
	if a.Directory == nil || a.AllowDuplicate || a.FaceID != 0 || a.Favorite != nil ||
		a.AssignID != 0 || a.AssignRevision != 0 || a.SuggestionID != 0 {
		return ErrLabelInvalid
	}
	if a.Action == "folder_move" {
		if (a.TargetID > 0 && (a.TargetRevision <= 0 || name != "")) || (a.TargetID == 0 && a.TargetRevision != 0) {
			return ErrLabelInvalid
		}
	} else if a.TargetID != 0 || a.TargetRevision != 0 || name != "" {
		return ErrLabelInvalid
	}
	return nil
}

// ApplyPersonFolderAction changes the entire exact directory atomically, independent
// of the preview limit. The revision prevents acting on a stale person selection.
func (l *Library) ApplyPersonFolderAction(ctx context.Context, id int64, a PersonFolderAction) error {
	_, err := l.applyPersonFolderAction(ctx, id, a, "", nil, "")
	return err
}

// Native and web writes share validation and mutation. Native receipts commit
// with the mutation; replay is checked before consulting the changed source.
func (l *Library) applyPersonFolderAction(ctx context.Context, id int64, a PersonFolderAction, actor string, label *LabelAction, fingerprint string) (LabelReceipt, error) {
	out := LabelReceipt{SourceID: id}

	if id <= 0 || a.Revision <= 0 || a.TargetID < 0 || (a.Directory != "" && (path.Clean(a.Directory) != a.Directory || strings.HasPrefix(a.Directory, "/") || a.Directory == ".." || strings.HasPrefix(a.Directory, "../"))) {
		return out, ErrLabelInvalid
	}
	switch a.Action {
	case "move", "unnamed", "ignore", "exclude", "include":
	default:
		return out, ErrLabelInvalid
	}
	name, err := normalizedPersonName(a.Name)
	if err != nil {
		return out, err
	}
	if a.Action == "move" && (a.TargetID == id || (a.TargetID == 0 && name == "")) {
		return out, ErrLabelInvalid
	}
	if err = l.refreshPersonIDsVisibility(ctx, id, a.TargetID); err != nil {
		return out, err
	}
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE photo_labeling_identity SET id=id WHERE id=1`); err != nil {
		return out, err
	}
	if label != nil {
		var dataset string
		if err = tx.QueryRowContext(ctx, `SELECT dataset FROM photo_labeling_identity WHERE id=1`).Scan(&dataset); err != nil {
			return out, err
		}
		if dataset != label.Dataset {
			return out, ErrLabelConflict
		}
		var previous, receipt string
		err = tx.QueryRowContext(ctx, `SELECT fingerprint,result FROM photo_labeling_actions WHERE actor=? AND operation_id=?`, actor, label.OperationID).Scan(&previous, &receipt)
		if err == nil {
			if previous != fingerprint {
				return out, ErrLabelConflict
			}
			err = json.Unmarshal([]byte(receipt), &out)
			return out, err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return out, err
		}
		out.OperationID, out.Action, out.At = label.OperationID, label.Action, time.Now().Unix()
	}
	var revision int64
	if err = tx.QueryRowContext(ctx, `SELECT revision FROM photo_person_revisions WHERE person_id=?`, id).Scan(&revision); err != nil {
		return out, err
	}
	if revision != a.Revision {
		return out, ErrLabelConflict
	}
	if newFaceDirectoryVisibility(l.root).private(a.Directory) {
		return out, ErrAdminOnly()
	}
	if a.Action == "include" {
		result, err := tx.ExecContext(ctx, `DELETE FROM person_folder_exclusions WHERE person_id=? AND directory=?`, id, a.Directory)
		if err != nil {
			return out, err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return out, ErrLabelConflict
		}
	} else {
		// Stream source fingerprints instead of collecting arbitrarily many face IDs.
		rows, err := tx.QueryContext(ctx, `SELECT DISTINCT f.path,m.size_bytes,m.mod_time_unix_nano,m.xmp_fingerprint,m.admin_only FROM photo_faces f LEFT JOIN media_index m ON m.path=f.path WHERE f.person_id=? AND f.directory=? AND f.ignored=0`, id, a.Directory)
		if err != nil {
			return out, err
		}
		count := 0
		for rows.Next() {
			var rel string
			var size, mtime sql.NullInt64
			var xmp sql.NullString
			var private sql.NullBool
			if err = rows.Scan(&rel, &size, &mtime, &xmp, &private); err != nil {
				break
			}
			if !size.Valid || !private.Valid || private.Bool {
				err = ErrLabelConflict
				break
			}
			var abs string
			abs, err = l.Resolve(rel)
			if err != nil {
				break
			}
			err = l.checkFaceImageSource(faceImageKey{path: abs, size: size.Int64, mtime: mtime.Int64, xmp: xmp.String})
			if err != nil {
				if errors.Is(err, os.ErrNotExist) || errors.Is(err, errFaceSourceChanged) {
					err = ErrLabelConflict
				}
				break
			}
			count++
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return out, err
		}
		if count == 0 {
			return out, ErrLabelConflict
		}
		target := a.TargetID
		if a.Action == "move" && target > 0 {
			var valid bool
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_people p WHERE id=? AND name<>'' AND `+visiblePersonSQL+`)`, target).Scan(&valid); err != nil {
				return out, err
			}
			if !valid {
				return out, ErrLabelInvalid
			}
			if label != nil {
				var targetRevision int64
				if err = tx.QueryRowContext(ctx, `SELECT revision FROM photo_person_revisions WHERE person_id=?`, target).Scan(&targetRevision); err != nil {
					return out, err
				}
				if targetRevision != a.TargetRevision {
					return out, ErrLabelConflict
				}
			}
		}
		if a.Action == "unnamed" || a.Action == "exclude" || (a.Action == "move" && target == 0) {
			if a.Action != "move" {
				name = ""
			} else {
				var exists bool
				if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_people WHERE name_fold=?)`, searchtext.GermanFold(name)).Scan(&exists); err != nil {
					return out, err
				}
				if exists {
					return out, ErrLabelNameExists
				}
			}
			result, err := tx.ExecContext(ctx, `INSERT INTO photo_people(name,name_fold,manual_name) VALUES(?,?,1)`, name, searchtext.GermanFold(name))
			if err != nil {
				return out, err
			}
			target, err = result.LastInsertId()
			out.NewID = target
			if err != nil {
				return out, err
			}
		}
		var changed sql.Result
		if a.Action == "ignore" {
			changed, err = tx.ExecContext(ctx, `UPDATE photo_faces SET ignored=1,manual=1 WHERE person_id=? AND directory=? AND ignored=0`, id, a.Directory)
		} else {
			favorite := ""
			if a.Action == "unnamed" || a.Action == "exclude" {
				favorite = ",favorite=0"
			}
			changed, err = tx.ExecContext(ctx, `UPDATE photo_faces SET person_id=?,manual=1`+favorite+` WHERE person_id=? AND directory=? AND ignored=0`, target, id, a.Directory)
		}
		if err != nil {
			return out, err
		}
		out.Faces, err = changed.RowsAffected()
		if err != nil {
			return out, err
		}
		if a.Action == "move" {
			out.TargetID = target
		}
		if a.Action == "exclude" {
			if _, err = tx.ExecContext(ctx, `INSERT INTO person_folder_exclusions VALUES(?,?)`, id, a.Directory); err != nil {
				return out, err
			}
		}
		if target > 0 {
			if err = refreshFaceReferencesTx(ctx, tx, target); err != nil {
				return out, err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE photo_person_revisions SET revision=revision+1 WHERE person_id=?`, id); err != nil {
		return out, err
	}
	if _, err = refreshFaceMutationTx(ctx, tx, map[int64]bool{id: true}); err != nil {
		return out, err
	}
	if label != nil {
		if err = tx.QueryRowContext(ctx, `SELECT coalesce((SELECT revision FROM photo_person_revisions WHERE person_id=?),0)`, id).Scan(&out.SourceRevision); err != nil {
			return out, err
		}
		encoded, err := json.Marshal(out)
		if err != nil {
			return out, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO photo_labeling_actions VALUES(?,?,?,?)`, actor, label.OperationID, fingerprint, string(encoded)); err != nil {
			return out, err
		}
	}
	if err = tx.Commit(); err != nil {
		return out, err
	}
	l.faceRuntime.graph = nil
	return out, nil
}

func folderExcludedPeople(ctx context.Context, tx faceRowsQuery, directory string, excluded map[int64]bool) error {
	rows, err := tx.QueryContext(ctx, `SELECT person_id FROM person_folder_exclusions WHERE directory=?`, directory)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		excluded[id] = true
	}
	return rows.Err()
}

// An identity merge inherits all exclusions. Conflicting existing faces cause
// the complete merge to fail instead of silently weakening the exclusion.
func mergePersonFolderExclusionsTx(ctx context.Context, tx *sql.Tx, source, target int64) error {
	var conflict bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM person_folder_exclusions e WHERE e.person_id=? AND EXISTS(SELECT 1 FROM photo_faces f WHERE f.person_id=? AND f.directory=e.directory)) OR EXISTS(SELECT 1 FROM person_folder_exclusions e WHERE e.person_id=? AND EXISTS(SELECT 1 FROM photo_faces f WHERE f.person_id=? AND f.directory=e.directory))`, source, target, target, source).Scan(&conflict); err != nil {
		return err
	}
	if conflict {
		return ErrPersonFolderExcluded
	}
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO person_folder_exclusions SELECT ?,directory FROM person_folder_exclusions WHERE person_id=?`, target, source)
	return err
}
