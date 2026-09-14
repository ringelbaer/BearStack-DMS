package photos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

func (l *Library) LabelSession(ctx context.Context) (LabelSession, error) {
	out := LabelSession{Protocol: 1, FaceFavorites: true, NamedPeople: true, NamedSearch: true, MergeSuggestions: true, MergeNaming: true, ManualMerge: true, MergeSideActions: true, NamedFaceBatch: true}
	err := l.index.labelIdentity(ctx, &out)
	return out, err
}
func (l *Library) LabelCandidates(ctx context.Context, after, upper int64) (LabelCandidates, error) {
	return l.labelPeopleList(ctx, after, upper, labelPeopleUnnamed, "")
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
	person, faces, err := l.index.labelPersonPage(ctx, id, offset, limit, after)
	if err != nil {
		return person, err
	}
	person.Faces = make([]LabelFace, 0, len(faces))
	for _, face := range faces {
		person.Faces = append(person.Faces, presentLabelFace(face))
	}
	return person, nil
}

// Presentation is shared by person details and merge portraits. Database rows
// retain source identity only until the library has built the public response.
func presentLabelFace(row indexedLabelFace) LabelFace {
	face := row.Face
	face.DisplayPath = mediaDisplayPath(row.Path)
	face.OriginalKey = labelOriginalKey(row.Path, row.Size, row.Modified)
	return face
}

func (l *Library) LabelSuggestions(ctx context.Context, q string, exact bool) ([]LabelPerson, error) {
	filter, arg := labelSuggestionFilter(q, exact)
	if err := l.refreshPeopleVisibility(ctx, `p.name<>'' AND `+filter, arg); err != nil {
		return nil, err
	}
	return l.index.labelSuggestions(ctx, q, exact)
}
func (l *Library) LabelReceipt(ctx context.Context, actor, operation, dataset string) (LabelReceipt, error) {
	return l.index.labelReceipt(ctx, actor, operation, dataset)
}
func (l *Library) ApplyLabelAction(ctx context.Context, actor string, id int64, a LabelAction) (LabelReceipt, error) {
	var out LabelReceipt
	if len(a.OperationID) < 16 || len(a.OperationID) > 128 || strings.TrimSpace(a.OperationID) != a.OperationID || a.Revision <= 0 || id <= 0 || strings.IndexFunc(a.OperationID, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	}) >= 0 {
		return out, ErrLabelInvalid
	}
	switch a.Action {
	case "name", "assign", "detach", "ignore", "rename", "unassign", "favorite", "accept_merge", "reject_merge", "name_merge", "merge_groups", "name_groups", "unassign_faces", "assign_faces", "name_faces", "ignore_faces":
	default:
		return out, ErrLabelInvalid
	}
	name, err := normalizedPersonName(a.Name)
	if err != nil {
		return out, ErrLabelInvalid
	}
	manualMerge := a.Action == "merge_groups" || a.Action == "name_groups"
	if err := validateLabelFaceBatch(id, a, name); err != nil {
		return out, err
	}
	if err := validateLabelGroupSelection(id, a, name); err != nil {
		return out, err
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
	if manualMerge {
		for _, group := range a.Groups {
			visibilityFilter += ` OR p.id=?`
			visibilityArgs = append(visibilityArgs, group.ID)
		}
	}
	if (a.Action == "name" || a.Action == "rename" || a.Action == "name_faces" || a.Action == "name_groups" || (a.Action == "name_merge" && a.AssignID == 0)) && !a.AllowDuplicate {
		visibilityFilter += ` OR p.name=?`
		visibilityArgs = append(visibilityArgs, name)
	}
	if err = l.refreshPeopleVisibility(ctx, visibilityFilter, visibilityArgs...); err != nil {
		return out, err
	}
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	receipt, mutation, err := l.index.applyLabelAction(ctx, actor, id, a, name, fingerprint)
	if err != nil {
		return receipt, err
	}
	if mutation.syncReferences {
		l.syncFaceMutation(ctx, mutation.affected, mutation.baseRevision, mutation.committedRevision)
	} else if mutation.resetGraph {
		l.faceRuntime.graph = nil
	}
	return receipt, nil
}

func LabelActor(source, subject, username string, accountID int64) string {
	encoded, _ := json.Marshal([]any{source, subject, accountID, username})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func labelOriginalKey(source string, size, modified int64) string {
	encoded, _ := json.Marshal([]any{source, size, modified})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
