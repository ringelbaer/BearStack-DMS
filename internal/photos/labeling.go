package photos

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"bearstack/internal/searchtext"
)

var ErrLabelConflict = errors.New("Personengruppe wurde geändert; bitte erneut prüfen")
var ErrLabelInvalid = errors.New("ungültige Benennungsaktion")
var ErrLabelNameExists = errors.New("Name bereits vorhanden")

type LabelSession struct {
	MergeNaming      bool   `json:"merge_naming"`
	MergeSuggestions bool   `json:"merge_suggestions"`
	NamedSearch      bool   `json:"named_search"`
	NamedPeople      bool   `json:"named_people"`
	FaceFavorites    bool   `json:"face_favorites"`
	Protocol         int    `json:"protocol"`
	Instance         string `json:"instance"`
	Dataset          string `json:"dataset"`
	UpperID          int64  `json:"upper_id"`
}
type LabelPerson struct {
	ID       int64       `json:"id"`
	Name     string      `json:"name"`
	Revision int64       `json:"revision"`
	Count    int64       `json:"count"`
	FaceID   int64       `json:"face_id"`
	Faces    []LabelFace `json:"faces,omitempty"`
	Offset   int         `json:"offset"`
}
type LabelFace struct {
	OriginalKey string          `json:"original_key"`
	Favorite    bool            `json:"favorite"`
	ID          int64           `json:"id"`
	DisplayPath string          `json:"display_path"`
	Bounds      LabelFaceBounds `json:"bounds"`
}

// LabelFaceBounds uses normalized coordinates in the EXIF-oriented original image.
type LabelFaceBounds struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}
type LabelCandidates struct {
	People  []LabelPerson `json:"people"`
	Next    int64         `json:"next"`
	HasNext bool          `json:"has_next"`
}
type LabelAction struct {
	AssignID       int64  `json:"assign_id,omitempty"`
	AssignRevision int64  `json:"assign_revision,omitempty"`
	SuggestionID   int64  `json:"suggestion_id,omitempty"`
	Favorite       *bool  `json:"favorite,omitempty"`
	OperationID    string `json:"operation_id"`
	Dataset        string `json:"dataset"`
	Revision       int64  `json:"revision"`
	Action         string `json:"action"`
	Name           string `json:"name,omitempty"`
	AllowDuplicate bool   `json:"allow_duplicate,omitempty"`
	TargetID       int64  `json:"target_id,omitempty"`
	TargetRevision int64  `json:"target_revision,omitempty"`
	FaceID         int64  `json:"face_id,omitempty"`
}
type LabelReceipt struct {
	SourceRevision int64  `json:"source_revision"`
	OperationID    string `json:"operation_id"`
	Action         string `json:"action"`
	SourceID       int64  `json:"source_id"`
	TargetID       int64  `json:"target_id"`
	NewID          int64  `json:"new_id"`
	Faces          int64  `json:"faces"`
	Groups         int    `json:"groups"`
	At             int64  `json:"at"`
}

const labelColumns = `p.id,p.name,r.revision,(SELECT count(*) FROM photo_faces f WHERE f.person_id=p.id AND f.ignored=0),(SELECT min(id) FROM photo_faces f WHERE f.person_id=p.id AND f.ignored=0)`
const labelFrom = ` FROM photo_people p JOIN photo_person_revisions r ON r.person_id=p.id `
const labelExists = ` EXISTS(SELECT 1 FROM photo_faces f WHERE f.person_id=p.id AND f.ignored=0) `

func (l *Library) LabelSession(ctx context.Context) (LabelSession, error) {
	out := LabelSession{Protocol: 1, FaceFavorites: true, NamedPeople: true, NamedSearch: true, MergeSuggestions: true, MergeNaming: true}
	err := l.index.db.QueryRowContext(ctx, `SELECT instance,dataset,(SELECT coalesce(max(id),0) FROM photo_people) FROM photo_labeling_identity WHERE id=1`).Scan(&out.Instance, &out.Dataset, &out.UpperID)
	return out, err
}
func scanLabel(row interface{ Scan(...any) error }) (LabelPerson, error) {
	var p LabelPerson
	err := row.Scan(&p.ID, &p.Name, &p.Revision, &p.Count, &p.FaceID)
	return p, err
}
func (l *Library) LabelCandidates(ctx context.Context, after, upper int64) (LabelCandidates, error) {
	out := LabelCandidates{People: []LabelPerson{}, Next: after}
	if err := l.refreshPeopleVisibility(ctx, `p.id>? AND p.id<=?`, after, upper); err != nil {
		return out, err
	}
	rows, err := l.index.db.QueryContext(ctx, `SELECT `+labelColumns+labelFrom+` WHERE p.name='' AND p.id>? AND p.id<=? AND `+labelExists+` ORDER BY p.id LIMIT 21`, after, upper)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		p, e := scanLabel(rows)
		if e != nil {
			return out, e
		}
		out.People = append(out.People, p)
	}
	if len(out.People) > 20 {
		out.HasNext = true
		out.People = out.People[:20]
	}
	if len(out.People) > 0 {
		out.Next = out.People[len(out.People)-1].ID
	}
	return out, rows.Err()
}
func (l *Library) LabelPerson(ctx context.Context, id int64, offset int, limits ...int) (LabelPerson, error) {
	limit := 4
	if len(limits) > 0 {
		limit = limits[0]
	}
	return l.labelPersonPage(ctx, id, offset, limit, 0)
}
func (l *Library) LabelPersonAfter(ctx context.Context, id, after int64, limit int) (LabelPerson, error) {
	return l.labelPersonPage(ctx, id, 0, limit, after)
}
func (l *Library) labelPersonPage(ctx context.Context, id int64, offset, limit int, after int64) (LabelPerson, error) {
	if limit < 1 || limit > 40 {
		return LabelPerson{}, ErrLabelInvalid
	}
	if err := l.refreshPersonIDsVisibility(ctx, id); err != nil {
		return LabelPerson{}, err
	}
	tx, err := l.index.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return LabelPerson{}, err
	}
	defer tx.Rollback()
	p, err := scanLabel(tx.QueryRowContext(ctx, `SELECT `+labelColumns+labelFrom+` WHERE p.id=? AND `+labelExists, id))
	if err != nil {
		return p, err
	}
	p.Offset = offset
	p.Faces = []LabelFace{}
	rows, err := tx.QueryContext(ctx, `SELECT f.id,f.path,f.x,f.y,f.width,f.height,f.favorite,m.size_bytes,m.mod_time_unix_nano FROM photo_faces f JOIN media_index m ON m.path=f.path WHERE f.person_id=? AND f.ignored=0 AND f.id>? ORDER BY f.id LIMIT ? OFFSET ?`, id, after, limit, offset)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var f LabelFace
		var source string
		var size, modified int64
		if err = rows.Scan(&f.ID, &source, &f.Bounds.X, &f.Bounds.Y, &f.Bounds.Width, &f.Bounds.Height, &f.Favorite, &size, &modified); err != nil {
			rows.Close()
			return p, err
		}
		f.DisplayPath = mediaDisplayPath(source)
		// Share decoded originals across faces without exposing filesystem paths.
		// Indexed file metadata invalidates the key after a detected file change.
		encoded, _ := json.Marshal([]any{source, size, modified})
		sum := sha256.Sum256(encoded)
		f.OriginalKey = hex.EncodeToString(sum[:])
		p.Faces = append(p.Faces, f)
	}
	err = rows.Err()
	rows.Close()
	return p, err
}
func (l *Library) LabelSuggestions(ctx context.Context, q string, exact bool) ([]LabelPerson, error) {
	filter := `p.name_fold LIKE ? ESCAPE '\'`
	arg := searchtext.LikeContainsPattern(searchtext.GermanFold(q))
	if exact {
		filter = `p.name=?`
		arg, _ = normalizedPersonName(q)
	}
	if err := l.refreshPeopleVisibility(ctx, `p.name<>'' AND `+filter, arg); err != nil {
		return nil, err
	}
	rows, err := l.index.db.QueryContext(ctx, `SELECT `+labelColumns+labelFrom+` WHERE p.name<>'' AND `+filter+` AND `+labelExists+` ORDER BY p.name_fold,p.id LIMIT 20`, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LabelPerson{}
	for rows.Next() {
		p, e := scanLabel(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (l *Library) LabelReceipt(ctx context.Context, actor, operation, dataset string) (LabelReceipt, error) {
	var out LabelReceipt
	var encoded string
	err := l.index.db.QueryRowContext(ctx, `SELECT a.result FROM photo_labeling_actions a,photo_labeling_identity i WHERE a.actor=? AND a.operation_id=? AND i.dataset=?`, actor, operation, dataset).Scan(&encoded)
	if err == nil {
		err = json.Unmarshal([]byte(encoded), &out)
	}
	return out, err
}
func (l *Library) ApplyLabelAction(ctx context.Context, actor string, id int64, a LabelAction) (LabelReceipt, error) {
	var out LabelReceipt
	if len(a.OperationID) < 16 || len(a.OperationID) > 128 || strings.TrimSpace(a.OperationID) != a.OperationID || a.Revision <= 0 || id <= 0 || strings.IndexFunc(a.OperationID, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	}) >= 0 {
		return out, ErrLabelInvalid
	}
	switch a.Action {
	case "name", "assign", "detach", "ignore", "rename", "unassign", "favorite", "accept_merge", "reject_merge", "name_merge":
	default:
		return out, ErrLabelInvalid
	}
	name, err := normalizedPersonName(a.Name)
	if err != nil {
		return out, ErrLabelInvalid
	}
	if (a.Action == "name" || a.Action == "rename") && name == "" || a.Action == "favorite" && a.Favorite == nil {
		return out, ErrLabelInvalid
	}
	merging := a.Action == "accept_merge" || a.Action == "reject_merge" || a.Action == "name_merge"
	if merging && (a.SuggestionID <= 0 || a.TargetID <= 0 || a.TargetID == id || a.TargetRevision <= 0) {
		return out, ErrLabelInvalid
	}
	if a.Action == "name_merge" && ((a.AssignID == 0 && (name == "" || a.AssignRevision != 0)) ||
		(a.AssignID != 0 && (a.AssignID <= 0 || a.AssignID == id || a.AssignID == a.TargetID || a.AssignRevision <= 0 || name != ""))) {
		return out, ErrLabelInvalid
	}
	encoded, _ := json.Marshal(struct {
		ID     int64
		Action LabelAction
	}{id, a})
	sum := sha256.Sum256(encoded)
	fingerprint := hex.EncodeToString(sum[:])
	visibilityFilter := `p.id IN (?,?,?)`
	visibilityArgs := []any{id, a.TargetID, a.AssignID}
	if (a.Action == "name" || a.Action == "rename" || (a.Action == "name_merge" && a.AssignID == 0)) && !a.AllowDuplicate {
		visibilityFilter += ` OR p.name=?`
		visibilityArgs = append(visibilityArgs, name)
	}
	if err = l.refreshPeopleVisibility(ctx, visibilityFilter, visibilityArgs...); err != nil {
		return out, err
	}
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	// Reserve SQLite's writer before reading revisions. Web renames and index
	// invalidation also write this database, outside the face runtime mutex.
	// This avoids a WAL snapshot-upgrade race between revision check and mutation.
	if _, err = tx.ExecContext(ctx, `UPDATE photo_labeling_identity SET id=id WHERE id=1`); err != nil {
		return out, err
	}
	var dataset string
	if err = tx.QueryRowContext(ctx, `SELECT dataset FROM photo_labeling_identity WHERE id=1`).Scan(&dataset); err != nil {
		return out, err
	}
	if dataset != a.Dataset {
		return out, ErrLabelConflict
	}
	var previous, receipt string
	err = tx.QueryRowContext(ctx, `SELECT fingerprint,result FROM photo_labeling_actions WHERE actor=? AND operation_id=?`, actor, a.OperationID).Scan(&previous, &receipt)
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
	source, err := scanLabel(tx.QueryRowContext(ctx, `SELECT `+labelColumns+labelFrom+` WHERE p.id=? AND `+labelExists, id))
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrLabelConflict
	}
	if err != nil {
		return out, err
	}
	managing := a.Action == "rename" || a.Action == "unassign" || a.Action == "favorite"
	if source.Revision != a.Revision || (!merging && (source.Name != "") != managing) {
		return out, ErrLabelConflict
	}
	var baseRevision int64
	if managing {
		if err = tx.QueryRowContext(ctx, `SELECT revision FROM photo_face_state WHERE id=1`).Scan(&baseRevision); err != nil {
			return out, err
		}
	}
	out = LabelReceipt{OperationID: a.OperationID, Action: a.Action, SourceID: id, Faces: source.Count, Groups: 1, At: time.Now().Unix()}
	switch a.Action {
	case "accept_merge", "reject_merge", "name_merge":
		if err = validateFaceMergeSuggestionTx(ctx, tx, id, a.TargetID, &faceMergeExpectation{a.SuggestionID, a.Revision, a.TargetRevision}); err != nil {
			return out, err
		}
		out.TargetID, out.Groups = a.TargetID, 0
		if a.Action == "name_merge" {
			out, err = nameMergeTx(ctx, tx, source, a, name, out)
		} else if a.Action == "reject_merge" {
			_, err = tx.ExecContext(ctx, `UPDATE photo_face_merge_suggestions SET rejected=1 WHERE id=?`, a.SuggestionID)
			out.Faces = 0
		} else {
			err = mergePersonTx(ctx, tx, id, a.TargetID)
		}
	case "name", "rename":
		if !a.AllowDuplicate {
			var exists bool
			err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_people p WHERE p.name=? AND p.id<>? AND `+labelExists+`)`, name, id).Scan(&exists)
			if err != nil {
				return out, err
			}
			if exists {
				return out, ErrLabelNameExists
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE photo_people SET name=?,name_fold=?,manual_name=1,name_source='' WHERE id=?`, name, searchtext.GermanFold(name), id)
		if a.Action == "rename" {
			out.Groups = 0
		}
	case "assign":
		target, e := scanLabel(tx.QueryRowContext(ctx, `SELECT `+labelColumns+labelFrom+` WHERE p.id=? AND `+labelExists, a.TargetID))
		if errors.Is(e, sql.ErrNoRows) {
			return out, ErrLabelConflict
		}
		if e != nil {
			return out, e
		}
		if a.TargetID == id || target.Name == "" || target.Revision != a.TargetRevision {
			return out, ErrLabelConflict
		}
		_, err = tx.ExecContext(ctx, `UPDATE photo_faces SET person_id=?,manual=1 WHERE person_id=? AND ignored=0`, a.TargetID, id)
		out.TargetID = a.TargetID
	case "ignore":
		_, err = tx.ExecContext(ctx, `UPDATE photo_faces SET ignored=1,manual=1 WHERE person_id=? AND ignored=0`, id)
	case "detach", "unassign", "favorite":
		if a.Action == "detach" && source.Count < 2 {
			return out, ErrLabelConflict
		}
		var belongs bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_faces WHERE id=? AND person_id=? AND ignored=0)`, a.FaceID, id).Scan(&belongs); err != nil {
			return out, err
		}
		if !belongs {
			return out, ErrLabelConflict
		}
		out.Faces = 1
		out.Groups = 0
		if a.Action == "favorite" {
			_, err = tx.ExecContext(ctx, `UPDATE photo_faces SET favorite=? WHERE id=?`, *a.Favorite, a.FaceID)
			break
		}
		result, e := tx.ExecContext(ctx, `INSERT INTO photo_people(name,name_fold,manual_name) VALUES('','',1)`)
		if e != nil {
			return out, e
		}
		out.NewID, err = result.LastInsertId()
		if err != nil {
			return out, err
		}
		_, err = tx.ExecContext(ctx, `UPDATE photo_faces SET person_id=?,manual=1 WHERE id=?`, out.NewID, a.FaceID)
		if err == nil && a.Action == "unassign" {
			_, err = tx.ExecContext(ctx, `UPDATE photo_faces SET favorite=0 WHERE id=?`, a.FaceID)
		}
		out.Faces = 1
		out.Groups = 0
	}
	if err != nil {
		return out, err
	}
	affected := map[int64]bool{}
	for _, pid := range []int64{id, a.TargetID, out.TargetID, out.NewID} {
		if pid > 0 {
			affected[pid] = true
		}
	}
	var committedRevision int64
	if a.Action != "reject_merge" {
		committedRevision, err = refreshFaceMutationTx(ctx, tx, affected)
		if err != nil {
			return out, err
		}
	}
	if err = tx.QueryRowContext(ctx, `SELECT coalesce((SELECT revision FROM photo_person_revisions WHERE person_id=?),0)`, id).Scan(&out.SourceRevision); err != nil {
		return out, err
	}
	encoded, err = json.Marshal(out)
	if err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO photo_labeling_actions VALUES(?,?,?,?)`, actor, a.OperationID, fingerprint, string(encoded)); err != nil {
		return out, err
	}
	if err = tx.Commit(); err != nil {
		return out, err
	}
	if managing {
		l.syncFaceMutation(ctx, affected, baseRevision, committedRevision)
	} else if a.Action != "reject_merge" {
		l.faceRuntime.graph = nil
	}
	return out, nil
}
func LabelActor(source, subject, username string, accountID int64) string {
	encoded, _ := json.Marshal([]any{source, subject, accountID, username})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
