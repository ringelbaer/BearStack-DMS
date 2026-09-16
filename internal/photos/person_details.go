package photos

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"bearstack/internal/sqlutil"
)

const personRelationLimit = 100

var ErrPersonDetails = errors.New("ungültige Stammdaten: benannte Personen und gültige Datumsangaben wählen; keine Selbstzuordnung oder doppelten Einträge")
var ErrPersonDetailsConflict = errors.New("Stammdaten wurden zwischenzeitlich geändert. Bitte den Dialog neu öffnen")
var ErrPersonDetailsMerge = errors.New("Stammdaten widersprechen sich; bitte vor dem Zusammenführen korrigieren")

type PersonMarriage struct {
	ID          int64        `json:"id"`
	Spouse      PersonParent `json:"spouse"`
	WeddingDate string       `json:"wedding_date"`
	DivorceDate string       `json:"divorce_date"`
}
type PersonDetails struct {
	Parents   PersonParents    `json:"parents"`
	Revision  string           `json:"revision"`
	BirthDate string           `json:"birth_date"`
	DeathDate string           `json:"death_date"`
	Siblings  []PersonParent   `json:"siblings"`
	Marriages []PersonMarriage `json:"marriages"`
}
type PersonMarriageInput struct {
	ID          int64  `json:"id"`
	SpouseID    int64  `json:"spouse_id"`
	WeddingDate string `json:"wedding_date"`
	DivorceDate string `json:"divorce_date"`
}
type PersonDetailsInput struct {
	MotherID   *int64                `json:"mother_id,omitempty"`
	FatherID   *int64                `json:"father_id,omitempty"`
	Revision   string                `json:"revision"`
	BirthDate  string                `json:"birth_date"`
	DeathDate  string                `json:"death_date"`
	SiblingIDs []int64               `json:"sibling_ids"`
	Marriages  []PersonMarriageInput `json:"marriages"`
}

func setupPersonDetails(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS person_details (
 person_id INTEGER PRIMARY KEY, birth_date TEXT NOT NULL DEFAULT '', death_date TEXT NOT NULL DEFAULT '');
 CREATE TABLE IF NOT EXISTS person_siblings (
 person_a INTEGER NOT NULL, person_b INTEGER NOT NULL, PRIMARY KEY(person_a,person_b), CHECK(person_a<person_b)) WITHOUT ROWID;
 CREATE INDEX IF NOT EXISTS idx_person_siblings_b ON person_siblings(person_b,person_a);
 CREATE TABLE IF NOT EXISTS person_marriages (
 id INTEGER PRIMARY KEY AUTOINCREMENT, person_a INTEGER NOT NULL, person_b INTEGER NOT NULL,
 wedding_date TEXT NOT NULL DEFAULT '', divorce_date TEXT NOT NULL DEFAULT '', CHECK(person_a<person_b),
 UNIQUE(person_a,person_b,wedding_date,divorce_date));
 CREATE INDEX IF NOT EXISTS idx_person_marriages_b ON person_marriages(person_b,id);
 CREATE TRIGGER IF NOT EXISTS photo_person_details_delete AFTER DELETE ON photo_people BEGIN
 DELETE FROM person_details WHERE person_id=old.id;
 DELETE FROM person_siblings WHERE person_a=old.id OR person_b=old.id;
 DELETE FROM person_marriages WHERE person_a=old.id OR person_b=old.id; END`)
	return err
}

func validPersonDates(first, last string) bool {
	for _, value := range []string{first, last} {
		if value == "" {
			continue
		}
		date, err := time.Parse("2006-01-02", value)
		if err != nil || date.Year() < 1 || date.Format("2006-01-02") != value {
			return false
		}
	}
	return first == "" || last == "" || first <= last
}

func (l *Library) refreshPersonDetailsVisibility(ctx context.Context, id int64) error {
	return l.refreshPeopleVisibility(ctx, `p.id=? OR p.id IN (
 SELECT person_b FROM person_siblings WHERE person_a=? UNION SELECT person_a FROM person_siblings WHERE person_b=?
 UNION SELECT person_b FROM person_marriages WHERE person_a=? UNION SELECT person_a FROM person_marriages WHERE person_b=?
 UNION SELECT parent_id FROM person_parents WHERE person_id=?)`, id, id, id, id, id, id)
}

type personDetailsReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// The revision covers stored relationships, including temporarily hidden ones.
// Reads are bounded by per-person limits and indexed in both directions.
func readPersonDetails(ctx context.Context, db personDetailsReader, id int64) (PersonDetailsInput, error) {
	out := PersonDetailsInput{MotherID: new(int64), FatherID: new(int64), SiblingIDs: []int64{}, Marriages: []PersonMarriageInput{}}
	err := db.QueryRowContext(ctx, `SELECT birth_date,death_date FROM person_details WHERE person_id=?`, id).Scan(&out.BirthDate, &out.DeathDate)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	rows, err := db.QueryContext(ctx, `SELECT person_b FROM person_siblings WHERE person_a=? UNION ALL SELECT person_a FROM person_siblings WHERE person_b=? ORDER BY 1`, id, id)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var other int64
		if err = rows.Scan(&other); err != nil {
			break
		}
		out.SiblingIDs = append(out.SiblingIDs, other)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = db.QueryContext(ctx, `SELECT id,CASE WHEN person_a=? THEN person_b ELSE person_a END,wedding_date,divorce_date FROM person_marriages WHERE person_a=? OR person_b=? ORDER BY id`, id, id, id)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var m PersonMarriageInput
		if err = rows.Scan(&m.ID, &m.SpouseID, &m.WeddingDate, &m.DivorceDate); err != nil {
			break
		}
		out.Marriages = append(out.Marriages, m)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	if err := db.QueryRowContext(ctx, `SELECT
 COALESCE((SELECT parent_id FROM person_parents WHERE person_id=? AND role='mother'),0),
 COALESCE((SELECT parent_id FROM person_parents WHERE person_id=? AND role='father'),0)`, id, id).Scan(out.MotherID, out.FatherID); err != nil {
		return out, err
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	hash := sha256.Sum256(encoded)
	out.Revision = hex.EncodeToString(hash[:])
	return out, nil
}

func detailPersonNames(ctx context.Context, db personDetailsReader, id int64, raw PersonDetailsInput) (map[int64]string, error) {
	args := []any{id}
	for _, parent := range []*int64{raw.MotherID, raw.FatherID} {
		if parent != nil && *parent > 0 {
			args = append(args, *parent)
		}
	}
	for _, other := range raw.SiblingIDs {
		args = append(args, other)
	}
	for _, m := range raw.Marriages {
		args = append(args, m.SpouseID)
	}
	rows, err := db.QueryContext(ctx, `SELECT p.id,p.name FROM photo_people p WHERE p.id IN (`+sqlutil.Placeholders(len(args))+`) AND p.name<>'' AND `+visiblePersonSQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := map[int64]string{}
	for rows.Next() {
		var pid int64
		var name string
		if err := rows.Scan(&pid, &name); err != nil {
			return nil, err
		}
		names[pid] = name
	}
	return names, rows.Err()
}

func (l *Library) PersonDetails(ctx context.Context, id int64) (PersonDetails, error) {
	out := PersonDetails{Siblings: []PersonParent{}, Marriages: []PersonMarriage{}}
	if id <= 0 {
		return out, ErrPersonDetails
	}
	if err := l.refreshPersonDetailsVisibility(ctx, id); err != nil {
		return out, err
	}
	tx, err := l.index.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	raw, err := readPersonDetails(ctx, tx, id)
	if err != nil {
		return out, err
	}
	names, err := detailPersonNames(ctx, tx, id, raw)
	if err != nil {
		return out, err
	}
	if names[id] == "" {
		return out, sql.ErrNoRows
	}
	if name := names[*raw.MotherID]; name != "" {
		out.Parents.Mother = &PersonParent{ID: *raw.MotherID, Name: name}
	}
	if name := names[*raw.FatherID]; name != "" {
		out.Parents.Father = &PersonParent{ID: *raw.FatherID, Name: name}
	}
	out.Revision, out.BirthDate, out.DeathDate = raw.Revision, raw.BirthDate, raw.DeathDate
	for _, other := range raw.SiblingIDs {
		if name := names[other]; name != "" {
			out.Siblings = append(out.Siblings, PersonParent{ID: other, Name: name})
		}
	}
	for _, m := range raw.Marriages {
		if name := names[m.SpouseID]; name != "" {
			out.Marriages = append(out.Marriages, PersonMarriage{ID: m.ID, Spouse: PersonParent{ID: m.SpouseID, Name: name}, WeddingDate: m.WeddingDate, DivorceDate: m.DivorceDate})
		}
	}
	return out, tx.Commit()
}

func validatePersonDetailsInput(id int64, in PersonDetailsInput) error {
	if id <= 0 || len(in.SiblingIDs) > personRelationLimit || len(in.Marriages) > personRelationLimit || !validPersonDates(in.BirthDate, in.DeathDate) {
		return ErrPersonDetails
	}
	for _, parent := range []*int64{in.MotherID, in.FatherID} {
		if parent != nil && (*parent < 0 || *parent == id) {
			return ErrPersonParents
		}
	}
	siblings := map[int64]bool{}
	marriages := map[string]bool{}
	marriageIDs := map[int64]bool{}
	for _, other := range in.SiblingIDs {
		if other <= 0 || other == id || siblings[other] {
			return ErrPersonDetails
		}
		siblings[other] = true
	}
	for _, m := range in.Marriages {
		if m.ID < 0 || m.SpouseID <= 0 || m.SpouseID == id || !validPersonDates(m.WeddingDate, m.DivorceDate) {
			return ErrPersonDetails
		}
		key := fmt.Sprintf("%d/%s/%s", m.SpouseID, m.WeddingDate, m.DivorceDate)
		if marriages[key] || (m.ID > 0 && marriageIDs[m.ID]) {
			return ErrPersonDetails
		}
		marriages[key] = true
		marriageIDs[m.ID] = true
	}
	return nil
}

func (l *Library) SetPersonDetails(ctx context.Context, id int64, in PersonDetailsInput) error {
	if err := validatePersonDetailsInput(id, in); err != nil {
		return err
	}
	if len(in.Revision) != 64 {
		return ErrPersonDetailsConflict
	}
	if err := l.refreshPersonDetailsVisibility(ctx, id); err != nil {
		return err
	}
	refs := append([]int64{id}, in.SiblingIDs...)
	for _, parent := range []*int64{in.MotherID, in.FatherID} {
		if parent != nil && *parent > 0 {
			refs = append(refs, *parent)
		}
	}
	for _, m := range in.Marriages {
		refs = append(refs, m.SpouseID)
	}
	if err := l.refreshPersonIDsVisibility(ctx, refs...); err != nil {
		return err
	}
	tx, err := l.index.beginTagWrite(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	old, err := readPersonDetails(ctx, tx, id)
	if err != nil {
		return err
	}
	if old.Revision != in.Revision {
		return ErrPersonDetailsConflict
	}
	names, err := detailPersonNames(ctx, tx, id, in)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if names[ref] == "" {
			return ErrPersonDetails
		}
	}
	oldNames, err := detailPersonNames(ctx, tx, id, old)
	if err != nil {
		return err
	}
	if in.MotherID != nil || in.FatherID != nil {
		// Omitted fields preserve existing parents. A blank field cannot remove
		// an undisclosed parent; explicit visible replacements remain possible.
		resolve := func(next *int64, previous int64) int64 {
			if next == nil || (*next == 0 && previous > 0 && oldNames[previous] == "") {
				return previous
			}
			return *next
		}
		mother, father := resolve(in.MotherID, *old.MotherID), resolve(in.FatherID, *old.FatherID)
		if mother != *old.MotherID || father != *old.FatherID {
			if err := writePersonParentsTx(ctx, tx, id, mother, father); err != nil {
				return err
			}
		}
	}
	// Only visible rows are replaceable; hidden relatives are neither disclosed nor removed.
	for _, other := range old.SiblingIDs {
		if oldNames[other] != "" {
			if _, err = tx.ExecContext(ctx, `DELETE FROM person_siblings WHERE person_a=? AND person_b=?`, min(id, other), max(id, other)); err != nil {
				return err
			}
		}
	}
	existing := map[int64]PersonMarriageInput{}
	for _, m := range old.Marriages {
		if oldNames[m.SpouseID] != "" {
			existing[m.ID] = m
		}
	}
	for _, m := range in.Marriages {
		if m.ID > 0 {
			if _, ok := existing[m.ID]; !ok {
				return ErrPersonDetails
			}
		}
	}
	// Reinsert edited rows with their stable IDs, also allowing an atomic date swap.
	for mid := range existing {
		if _, err = tx.ExecContext(ctx, `DELETE FROM person_marriages WHERE id=?`, mid); err != nil {
			return err
		}
	}
	for _, other := range in.SiblingIDs {
		if _, err = tx.ExecContext(ctx, `INSERT INTO person_siblings(person_a,person_b) VALUES(?,?)`, min(id, other), max(id, other)); err != nil {
			return err
		}
	}
	for _, m := range in.Marriages {
		var mid any
		if m.ID > 0 {
			mid = m.ID
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO person_marriages(id,person_a,person_b,wedding_date,divorce_date) VALUES(?,?,?,?,?)`, mid, min(id, m.SpouseID), max(id, m.SpouseID), m.WeddingDate, m.DivorceDate); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO person_details(person_id,birth_date,death_date) VALUES(?,?,?) ON CONFLICT(person_id) DO UPDATE SET birth_date=excluded.birth_date,death_date=excluded.death_date`, id, in.BirthDate, in.DeathDate); err != nil {
		return err
	}
	if err = validatePersonRelationLimits(ctx, tx, refs); err != nil {
		return err
	}
	return tx.Commit()
}

func validatePersonRelationLimits(ctx context.Context, tx *sql.Tx, ids []int64) error {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	var invalid bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_people p WHERE p.id IN (`+sqlutil.Placeholders(len(ids))+`) AND (
 (SELECT count(*) FROM person_siblings WHERE person_a=p.id OR person_b=p.id)>100 OR
 (SELECT count(*) FROM person_marriages WHERE person_a=p.id OR person_b=p.id)>100))`, args...).Scan(&invalid)
	if err != nil {
		return err
	}
	if invalid {
		return errors.New("Höchstens 100 Geschwister und 100 Ehen pro Person möglich")
	}
	return nil
}

func mergePersonFamilyTx(ctx context.Context, tx *sql.Tx, source, target int64) error {
	if err := mergePersonParentsTx(ctx, tx, source, target); err != nil {
		return err
	}
	// Most automatically merged unnamed groups have no personal records.
	// Avoid loading two complete profiles in that common case.
	var hasDetails bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM person_details WHERE person_id=? AND (birth_date<>'' OR death_date<>''))
 OR EXISTS(SELECT 1 FROM person_siblings WHERE person_a=? OR person_b=?)
 OR EXISTS(SELECT 1 FROM person_marriages WHERE person_a=? OR person_b=?)`, source, source, source, source, source).Scan(&hasDetails); err != nil {
		return err
	}
	if !hasDetails {
		return nil
	}
	from, err := readPersonDetails(ctx, tx, source)
	if err != nil {
		return err
	}
	to, err := readPersonDetails(ctx, tx, target)
	if err != nil {
		return err
	}
	for _, dates := range [][2]string{{from.BirthDate, to.BirthDate}, {from.DeathDate, to.DeathDate}} {
		if dates[0] != "" && dates[1] != "" && dates[0] != dates[1] {
			return ErrPersonDetailsMerge
		}
	}
	if to.BirthDate == "" {
		to.BirthDate = from.BirthDate
	}
	if to.DeathDate == "" {
		to.DeathDate = from.DeathDate
	}
	if !validPersonDates(to.BirthDate, to.DeathDate) {
		return ErrPersonDetailsMerge
	}
	for _, other := range from.SiblingIDs {
		if other == target {
			return ErrPersonDetailsMerge
		}
	}
	for _, m := range from.Marriages {
		if m.SpouseID == target {
			return ErrPersonDetailsMerge
		}
	}
	for _, other := range from.SiblingIDs {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO person_siblings(person_a,person_b) VALUES(?,?)`, min(target, other), max(target, other)); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM person_siblings WHERE person_a=? OR person_b=?`, source, source); err != nil {
		return err
	}
	for _, m := range from.Marriages {
		var duplicate bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM person_marriages WHERE person_a=? AND person_b=? AND wedding_date=? AND divorce_date=?)`, min(target, m.SpouseID), max(target, m.SpouseID), m.WeddingDate, m.DivorceDate).Scan(&duplicate); err != nil {
			return err
		}
		if duplicate {
			_, err = tx.ExecContext(ctx, `DELETE FROM person_marriages WHERE id=?`, m.ID)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE person_marriages SET person_a=?,person_b=? WHERE id=?`, min(target, m.SpouseID), max(target, m.SpouseID), m.ID)
		}
		if err != nil {
			return err
		}
	}
	if to.BirthDate != "" || to.DeathDate != "" {
		if _, err = tx.ExecContext(ctx, `INSERT INTO person_details(person_id,birth_date,death_date) VALUES(?,?,?) ON CONFLICT(person_id) DO UPDATE SET birth_date=excluded.birth_date,death_date=excluded.death_date`, target, to.BirthDate, to.DeathDate); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM person_details WHERE person_id=?`, source); err != nil {
		return err
	}
	if err = validatePersonRelationLimits(ctx, tx, []int64{target}); err != nil {
		return ErrPersonDetailsMerge
	}
	return nil
}
