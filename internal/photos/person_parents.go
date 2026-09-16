package photos

import (
	"context"
	"database/sql"
	"errors"
)

type PersonParent struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type PersonParents struct {
	Mother *PersonParent `json:"mother,omitempty"`
	Father *PersonParent `json:"father,omitempty"`
}

var ErrPersonParents = errors.New("ungültige Elternzuordnung: benannte Personen wählen; Selbstzuordnungen, gleiche Eltern und Kreise sind nicht erlaubt")
var ErrParentMerge = errors.New("Elternzuordnungen widersprechen sich; bitte vor dem Zusammenführen korrigieren")

func setupPersonParents(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS person_parents (
 person_id INTEGER NOT NULL, role TEXT NOT NULL CHECK(role IN ('mother','father')),
 parent_id INTEGER NOT NULL, PRIMARY KEY(person_id,role), CHECK(person_id<>parent_id)) WITHOUT ROWID;
 CREATE INDEX IF NOT EXISTS idx_person_parents_parent ON person_parents(parent_id,person_id);
 CREATE TRIGGER IF NOT EXISTS photo_person_parents_delete AFTER DELETE ON photo_people BEGIN
 DELETE FROM person_parents WHERE person_id=old.id OR parent_id=old.id; END`)
	return err
}

func (l *Library) PersonParents(ctx context.Context, id int64) (PersonParents, error) {
	out := PersonParents{}
	if err := l.refreshPeopleVisibility(ctx, `p.id=? OR p.id IN (SELECT parent_id FROM person_parents WHERE person_id=?)`, id, id); err != nil {
		return out, err
	}
	rows, err := l.index.db.QueryContext(ctx, `SELECT r.role,p.id,p.name FROM person_parents r JOIN photo_people p ON p.id=r.parent_id
 WHERE r.person_id=? AND p.name<>'' AND `+visiblePersonSQL+`
 AND EXISTS(SELECT 1 FROM photo_people child WHERE child.id=r.person_id AND child.name<>''
 AND EXISTS(SELECT 1 FROM photo_faces f JOIN media_index m ON m.path=f.path WHERE f.person_id=child.id AND f.ignored=0 AND m.admin_only=0))`, id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var role string
		var p PersonParent
		if err := rows.Scan(&role, &p.ID, &p.Name); err != nil {
			return out, err
		}
		if role == "mother" {
			out.Mother = &p
		} else {
			out.Father = &p
		}
	}
	return out, rows.Err()
}

func validateParentGraphTx(ctx context.Context, tx *sql.Tx, id int64) error {
	var invalid bool
	err := tx.QueryRowContext(ctx, `WITH RECURSIVE ancestors(id) AS (
 SELECT parent_id FROM person_parents WHERE person_id=?
 UNION SELECT r.parent_id FROM person_parents r JOIN ancestors a ON r.person_id=a.id)
 SELECT EXISTS(SELECT 1 FROM ancestors WHERE id=?) OR EXISTS(
 SELECT 1 FROM person_parents WHERE person_id=? GROUP BY parent_id HAVING count(*)>1)`, id, id, id).Scan(&invalid)
	if err != nil {
		return err
	}
	if invalid {
		return ErrPersonParents
	}
	return nil
}

func (l *Library) SetPersonParents(ctx context.Context, id, mother, father int64) error {
	if id <= 0 || mother < 0 || father < 0 || mother == id || father == id || (mother > 0 && mother == father) {
		return ErrPersonParents
	}
	if err := l.refreshPersonIDsVisibility(ctx, id, mother, father); err != nil {
		return err
	}
	tx, err := l.index.beginTagWrite(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, pid := range []int64{id, mother, father} {
		if pid == 0 {
			continue
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_people p WHERE p.id=? AND p.name<>'' AND `+visiblePersonSQL+`)`, pid).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrPersonParents
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM person_parents WHERE person_id=?`, id); err != nil {
		return err
	}
	for _, v := range []struct {
		role string
		id   int64
	}{{"mother", mother}, {"father", father}} {
		if v.id > 0 {
			if _, err := tx.ExecContext(ctx, `INSERT INTO person_parents(person_id,role,parent_id) VALUES(?,?,?)`, id, v.role, v.id); err != nil {
				return err
			}
		}
	}
	if err := validateParentGraphTx(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit()
}

// Merge metadata in the same transaction as the faces. Conflicting families are
// rejected atomically rather than silently dropping one of the relationships.
func mergePersonParentsTx(ctx context.Context, tx *sql.Tx, source, target int64) error {
	var conflict bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM person_parents s JOIN person_parents t ON t.role=s.role
 WHERE s.person_id=? AND t.person_id=? AND s.parent_id<>t.parent_id)
 OR EXISTS(SELECT 1 FROM person_parents WHERE (person_id=? AND parent_id=?) OR (person_id=? AND parent_id=?))`, source, target, source, target, target, source).Scan(&conflict); err != nil {
		return err
	}
	if conflict {
		return ErrParentMerge
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO person_parents(person_id,role,parent_id) SELECT ?,role,parent_id FROM person_parents WHERE person_id=?`, target, source); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM person_parents WHERE person_id=?`, source); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE person_parents SET parent_id=? WHERE parent_id=?`, target, source); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM person_parents WHERE parent_id=? GROUP BY person_id HAVING count(*)>1)`, target).Scan(&conflict); err != nil {
		return err
	}
	if conflict {
		return ErrParentMerge
	}
	if err := validateParentGraphTx(ctx, tx, target); err != nil {
		if errors.Is(err, ErrPersonParents) {
			return ErrParentMerge
		}
		return err
	}
	return nil
}
