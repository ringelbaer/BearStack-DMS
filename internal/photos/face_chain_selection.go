package photos

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"bearstack/internal/searchtext"
	"bearstack/internal/sqlutil"
)

type FaceChainSelection struct {
	Dataset string          `json:"dataset"`
	Groups  []LabelGroupRef `json:"groups"`
}
type FaceChainPageRequest struct {
	FaceChainSelection
	Page int `json:"page"`
}
type FaceChainFace struct {
	RecognizedFace
	FolderName  string `json:"folder_name"`
	DisplayPath string `json:"display_path"`
}
type FaceChainPage struct {
	Faces []FaceChainFace `json:"faces"`
	Page  int             `json:"page"`
	Total int64           `json:"total"`
	Pages int64           `json:"pages"`
}
type FaceChainAssignment struct {
	FaceChainSelection
	OperationID    string  `json:"operation_id"`
	ExcludedFaces  []int64 `json:"excluded_faces"`
	TargetID       int64   `json:"target_id"`
	TargetRevision int64   `json:"target_revision"`
	TargetName     string  `json:"target_name"`
	Name           string  `json:"name"`
}

func (s FaceChainSelection) arguments() ([]any, error) {
	if len(s.Dataset) != 32 || len(s.Groups) < 2 || len(s.Groups) > MaxFaceChainGroups {
		return nil, ErrLabelInvalid
	}
	args := make([]any, 0, len(s.Groups))
	seen := map[int64]bool{}
	for _, g := range s.Groups {
		if g.ID <= 0 || g.Revision <= 0 || seen[g.ID] {
			return nil, ErrLabelInvalid
		}
		seen[g.ID] = true
		args = append(args, g.ID)
	}
	return args, nil
}

func (s FaceChainSelection) validate(ctx context.Context, tx faceRowsQuery) error {
	args, err := s.arguments()
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT p.id,v.revision FROM photo_people p
 JOIN photo_person_revisions v ON v.person_id=p.id CROSS JOIN photo_labeling_identity i
 WHERE p.name='' AND i.id=1 AND i.dataset=? AND p.id IN (`+sqlutil.Placeholders(len(args))+`)`, append([]any{s.Dataset}, args...)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	expected := map[int64]int64{}
	for _, g := range s.Groups {
		expected[g.ID] = g.Revision
	}
	count := 0
	for rows.Next() {
		var id, revision int64
		if err := rows.Scan(&id, &revision); err != nil {
			return err
		}
		if expected[id] != revision {
			return ErrLabelConflict
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != len(s.Groups) {
		return ErrLabelConflict
	}
	return nil
}

func (l *Library) refreshFaceChainSelection(ctx context.Context, s FaceChainSelection, target int64) error {
	if _, err := s.arguments(); err != nil {
		return err
	}
	ids := make([]int64, 0, len(s.Groups)+1)
	for _, g := range s.Groups {
		ids = append(ids, g.ID)
	}
	if target > 0 {
		ids = append(ids, target)
	}
	return l.refreshPersonIDsVisibility(ctx, ids...)
}

func (l *Library) FaceChainFaces(ctx context.Context, request FaceChainPageRequest) (FaceChainPage, error) {
	out := FaceChainPage{Faces: []FaceChainFace{}, Page: request.Page}
	if request.Page < 1 || request.Page > 1000000 {
		return out, ErrLabelInvalid
	}
	args, err := request.arguments()
	if err != nil {
		return out, err
	}
	if err := l.refreshFaceChainSelection(ctx, request.FaceChainSelection, 0); err != nil {
		return out, err
	}
	tx, err := l.index.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err := request.validate(ctx, tx); err != nil {
		return out, err
	}
	filter := `f.person_id IN (` + sqlutil.Placeholders(len(args)) + `) AND f.ignored=0 AND f.needs_review=0 AND f.embedding_current=1 AND m.admin_only=0`
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM photo_faces f JOIN media_index m ON m.path=f.path WHERE `+filter, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	out.Pages = max(1, (out.Total+FaceChainPageSize-1)/FaceChainPageSize)
	if int64(request.Page) > out.Pages {
		return out, ErrLabelInvalid
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+faceColumns+` FROM photo_faces f JOIN photo_people p ON p.id=f.person_id JOIN media_index m ON m.path=f.path WHERE `+filter+` ORDER BY f.person_id,f.path,f.id LIMIT ? OFFSET ?`, append(args, FaceChainPageSize, (request.Page-1)*FaceChainPageSize)...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		face, err := scanFace(rows)
		if err != nil {
			rows.Close()
			return out, err
		}
		out.Faces = append(out.Faces, FaceChainFace{face, MediaFolderName(face.Path), mediaDisplayPath(face.Path)})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if err := tx.Commit(); err != nil {
		return out, err
	}
	// Verify only the displayed page's originals, without loading all faces or
	// decoding images. A changed source invalidates the complete selection.
	paths := map[string]bool{}
	for _, face := range out.Faces {
		if !paths[face.Path] {
			if err := l.refreshFaceSource(ctx, face.Path); err != nil {
				return out, err
			}
			paths[face.Path] = true
		}
	}
	if err := request.validate(ctx, l.index.db); err != nil {
		return out, err
	}
	return out, nil
}

// All faces are selected by default. Only exclusions cross the network; the
// database performs one atomic update even when the chain spans many pages.
func (l *Library) AssignFaceChain(ctx context.Context, actor string, a FaceChainAssignment) (LabelReceipt, error) {
	var out LabelReceipt
	args, err := a.arguments()
	if err != nil {
		return out, err
	}
	if len(a.OperationID) < 16 || len(a.OperationID) > 128 || strings.IndexFunc(a.OperationID, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	}) >= 0 || len(a.ExcludedFaces) > MaxFaceChainExclusions {
		return out, ErrLabelInvalid
	}
	name, err := normalizedPersonName(a.Name)
	if err != nil {
		return out, ErrLabelInvalid
	}
	if a.TargetID < 0 || (a.TargetID == 0 && (name == "" || a.TargetRevision != 0 || a.TargetName != "")) || (a.TargetID > 0 && (name != "" || a.TargetRevision < 0 || (a.TargetRevision == 0 && a.TargetName == ""))) {
		return out, ErrLabelInvalid
	}
	excluded := map[int64]bool{}
	for _, id := range a.ExcludedFaces {
		if id <= 0 || excluded[id] {
			return out, ErrLabelInvalid
		}
		excluded[id] = true
	}
	encoded, err := json.Marshal(a)
	if err != nil {
		return out, err
	}
	sum := sha256.Sum256(append([]byte("face_chain_assign:"), encoded...))
	fingerprint := hex.EncodeToString(sum[:])
	if err := l.refreshFaceChainSelection(ctx, a.FaceChainSelection, a.TargetID); err != nil {
		return out, err
	}
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE photo_labeling_identity SET id=id WHERE id=1`); err != nil {
		return out, err
	}
	var dataset string
	if err := tx.QueryRowContext(ctx, `SELECT dataset FROM photo_labeling_identity WHERE id=1`).Scan(&dataset); err != nil {
		return out, err
	}
	if dataset != a.Dataset {
		return out, ErrLabelConflict
	}
	var previous, result string
	err = tx.QueryRowContext(ctx, `SELECT fingerprint,result FROM photo_labeling_actions WHERE actor=? AND operation_id=?`, actor, a.OperationID).Scan(&previous, &result)
	if err == nil {
		if previous != fingerprint {
			return out, ErrLabelConflict
		}
		err = json.Unmarshal([]byte(result), &out)
		return out, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	if err := a.validate(ctx, tx); err != nil {
		return out, err
	}
	if a.TargetID > 0 {
		var valid bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_people p JOIN photo_person_revisions v ON v.person_id=p.id WHERE p.id=? AND p.name<>'' AND (?=0 OR v.revision=?) AND (?='' OR p.name=?))`, a.TargetID, a.TargetRevision, a.TargetRevision, a.TargetName, a.TargetName).Scan(&valid); err != nil {
			return out, err
		}
		if !valid {
			return out, ErrLabelConflict
		}
	} else {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_people WHERE name=?)`, name).Scan(&exists); err != nil {
			return out, err
		}
		if exists {
			return out, ErrLabelNameExists
		}
		created, err := tx.ExecContext(ctx, `INSERT INTO photo_people(name,name_fold,manual_name) VALUES(?,?,1)`, name, searchtext.GermanFold(name))
		if err != nil {
			return out, err
		}
		a.TargetID, err = created.LastInsertId()
		if err != nil {
			return out, err
		}
	}
	filter := `person_id IN (` + sqlutil.Placeholders(len(args)) + `) AND ignored=0`
	if len(a.ExcludedFaces) > 0 {
		ids := make([]any, 0, len(a.ExcludedFaces))
		for _, id := range a.ExcludedFaces {
			ids = append(ids, id)
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM photo_faces WHERE `+filter+` AND id IN (`+sqlutil.Placeholders(len(ids))+`)`, append(append([]any{}, args...), ids...)...).Scan(&count); err != nil {
			return out, err
		}
		if count != len(ids) {
			return out, ErrLabelConflict
		}
		filter += ` AND id NOT IN (` + sqlutil.Placeholders(len(ids)) + `)`
		args = append(args, ids...)
	}
	// Visibility was refreshed before the writer reservation. Read every chosen
	// source's path now so newly protected directories cannot be reassigned.
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT f.path,m.size_bytes,m.mod_time_unix_nano,m.xmp_fingerprint
 FROM photo_faces f LEFT JOIN media_index m ON m.path=f.path WHERE `+filter, args...)
	if err != nil {
		return out, err
	}
	visibility := newFaceDirectoryVisibility(l.root)
	for rows.Next() {
		var path string
		var size, modified sql.NullInt64
		var xmp sql.NullString
		if err := rows.Scan(&path, &size, &modified, &xmp); err != nil {
			rows.Close()
			return out, err
		}
		if !size.Valid || !modified.Valid || visibility.private(parentPath(path)) {
			rows.Close()
			return out, ErrLabelConflict
		}
		abs, err := l.Resolve(path)
		if err != nil {
			rows.Close()
			return out, ErrLabelConflict
		}
		if err := l.checkFaceImageSource(faceImageKey{path: abs, size: size.Int64, mtime: modified.Int64, xmp: xmp.String}); err != nil {
			rows.Close()
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, errFaceSourceChanged) {
				return out, ErrLabelConflict
			}
			return out, err
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	updated, err := tx.ExecContext(ctx, `UPDATE photo_faces SET person_id=?,manual=1 WHERE `+filter, append([]any{a.TargetID}, args...)...)
	if err != nil {
		return out, err
	}
	count, err := updated.RowsAffected()
	if err != nil {
		return out, err
	}
	if count == 0 {
		return out, ErrLabelInvalid
	}
	affected := map[int64]bool{a.TargetID: true}
	for _, g := range a.Groups {
		affected[g.ID] = true
	}
	if _, err := refreshFaceMutationTx(ctx, tx, affected); err != nil {
		return out, err
	}
	out = LabelReceipt{OperationID: a.OperationID, Action: "assign_chain", SourceID: a.Groups[0].ID, TargetID: a.TargetID, Faces: count, Groups: len(a.Groups), At: time.Now().Unix()}
	encoded, err = json.Marshal(out)
	if err != nil {
		return out, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO photo_labeling_actions VALUES(?,?,?,?)`, actor, a.OperationID, fingerprint, string(encoded)); err != nil {
		return out, err
	}
	if err := tx.Commit(); err != nil {
		return out, err
	}
	l.faceRuntime.graph = nil
	return out, nil
}
