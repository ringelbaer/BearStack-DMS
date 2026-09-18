package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"bearstack/internal/sqlutil"
)

const MaxFamilyTreeRoots = 200
const MaxFamilyTreePeople = 10000
const MaxFamilyTreeRelations = 50000

var ErrFamilyTreeSettings = errors.New("Bitte höchstens 200 unterschiedliche, benannte und sichtbare Personen auswählen.")
var ErrFamilyTreeConflict = errors.New("Die Stammbaum-Auswahl wurde zwischenzeitlich geändert. Bitte die Einstellungen neu laden.")
var ErrFamilyTreeLarge = errors.New("Die ausgewählten Familien umfassen mehr als 10.000 Personen oder 50.000 Verbindungen. Bitte die Auswahl verkleinern; es wird kein unvollständiger Stammbaum angezeigt.")

func setupFamilyTreeSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS photo_family_tree_roots(person_id INTEGER PRIMARY KEY);
 CREATE TABLE IF NOT EXISTS photo_family_tree_settings(id INTEGER PRIMARY KEY CHECK(id=1),revision INTEGER NOT NULL DEFAULT 1);
 INSERT OR IGNORE INTO photo_family_tree_settings(id) VALUES(1);
 CREATE TRIGGER IF NOT EXISTS family_tree_root_insert AFTER INSERT ON photo_family_tree_roots BEGIN UPDATE photo_family_tree_settings SET revision=revision+1 WHERE id=1; END;
 CREATE TRIGGER IF NOT EXISTS family_tree_root_delete AFTER DELETE ON photo_family_tree_roots BEGIN UPDATE photo_family_tree_settings SET revision=revision+1 WHERE id=1; END;
 CREATE TRIGGER IF NOT EXISTS family_tree_person_delete AFTER DELETE ON photo_people BEGIN DELETE FROM photo_family_tree_roots WHERE person_id=old.id; END;`)
	return err
}

func mergeFamilyTreeRootTx(ctx context.Context, tx *sql.Tx, source, target int64) error {
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO photo_family_tree_roots SELECT ? FROM photo_family_tree_roots WHERE person_id=?`, target, source); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM photo_family_tree_roots WHERE person_id=?`, source)
	return err
}

type FamilyTreeRoot struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
}
type FamilyTreeSettings struct {
	Revision int64            `json:"revision"`
	Roots    []FamilyTreeRoot `json:"roots"`
}
type FamilyTreePerson struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	BirthDate string   `json:"birth_date"`
	DeathDate string   `json:"death_date"`
	FaceID    int64    `json:"face_id"`
	Tags      []string `json:"tags"`
}
type FamilyTreeRelation struct {
	ID          int64  `json:"id"`
	From        int64  `json:"from"`
	To          int64  `json:"to"`
	Kind        string `json:"kind"`
	WeddingDate string `json:"wedding_date,omitempty"`
	DivorceDate string `json:"divorce_date,omitempty"`
}
type FamilyTree struct {
	ID        int64                `json:"id"`
	Roots     []int64              `json:"roots"`
	People    []FamilyTreePerson   `json:"people"`
	Relations []FamilyTreeRelation `json:"relations"`
}
type FamilyTreePage struct {
	Trees []FamilyTree `json:"trees"`
}

// The global navigation only needs an indexed existence check, never a graph walk.
func (l *Library) FamilyTreeEnabled(ctx context.Context) (bool, error) {
	if l == nil || !l.index.available() {
		return false, nil
	}
	var enabled bool
	err := l.index.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_family_tree_roots)`).Scan(&enabled)
	return enabled, err
}

func familyTreeRootIDs(ctx context.Context, db personDetailsReader) ([]int64, int64, error) {
	var revision int64
	if err := db.QueryRowContext(ctx, `SELECT revision FROM photo_family_tree_settings WHERE id=1`).Scan(&revision); err != nil {
		return nil, 0, err
	}
	rows, err := db.QueryContext(ctx, `SELECT person_id FROM photo_family_tree_roots ORDER BY person_id`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	return ids, revision, rows.Err()
}

func (l *Library) FamilyTreeSettings(ctx context.Context) (FamilyTreeSettings, error) {
	out := FamilyTreeSettings{Roots: []FamilyTreeRoot{}}
	tx, err := l.index.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	ids, revision, err := familyTreeRootIDs(ctx, tx)
	if err != nil {
		return out, err
	}
	out.Revision = revision
	people, err := l.familyTreePeople(ctx, tx, ids, newFaceDirectoryVisibility(l.root))
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		p, ok := people[id]
		out.Roots = append(out.Roots, FamilyTreeRoot{ID: id, Name: p.Name, Available: ok})
	}
	return out, tx.Commit()
}

func (l *Library) SetFamilyTreeRoots(ctx context.Context, ids []int64, revision int64) error {
	if len(ids) > MaxFamilyTreeRoots || revision < 1 {
		return ErrFamilyTreeSettings
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return ErrFamilyTreeSettings
		}
		seen[id] = true
	}
	tx, err := l.index.beginTagWrite(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	previous, current, err := familyTreeRootIDs(ctx, tx)
	if err != nil {
		return err
	}
	if current != revision {
		return ErrFamilyTreeConflict
	}
	existing := map[int64]bool{}
	for _, id := range previous {
		existing[id] = true
	}
	visible, err := l.familyTreePeople(ctx, tx, ids, newFaceDirectoryVisibility(l.root))
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, ok := visible[id]; !ok && !existing[id] {
			return ErrFamilyTreeSettings
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM photo_family_tree_roots`); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = tx.ExecContext(ctx, `INSERT INTO photo_family_tree_roots VALUES(?)`, id); err != nil {
			return err
		}
	}
	// Also advance for empty -> empty, so all successful form saves invalidate their revision.
	if _, err = tx.ExecContext(ctx, `UPDATE photo_family_tree_settings SET revision=revision+1 WHERE id=1`); err != nil {
		return err
	}
	return tx.Commit()
}

// Read a bounded frontier in a single snapshot. Live folder markers and imported
// name provenance are checked without changing the index inside this read transaction.
// No hidden person may appear in the graph or act as a bridge to other relatives.
func (l *Library) familyTreePeople(ctx context.Context, tx *sql.Tx, ids []int64, visibility *faceDirectoryVisibility) (map[int64]FamilyTreePerson, error) {
	out := map[int64]FamilyTreePerson{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	list := sqlutil.Placeholders(len(ids))
	rows, err := tx.QueryContext(ctx, `SELECT p.id,p.name,coalesce(d.birth_date,''),coalesce(d.death_date,''),p.name_source,p.manual_name
 FROM photo_people p LEFT JOIN person_details d ON d.person_id=p.id WHERE p.id IN (`+list+`) AND p.name<>''`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p FamilyTreePerson
		var source string
		var manual bool
		if err = rows.Scan(&p.ID, &p.Name, &p.BirthDate, &p.DeathDate, &source, &manual); err != nil {
			break
		}
		if !manual && source != "" && visibility.private(parentPath(source)) {
			continue
		}
		p.Tags = []string{}
		out[p.ID] = p
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	// Aggregate directory metadata, not images or embeddings. Memory is bounded by
	// the requested people and one streaming row even for large photo collections.
	rows, err = tx.QueryContext(ctx, `SELECT f.person_id,f.directory,min(f.id) FROM photo_faces f JOIN media_index m ON m.path=f.path
 WHERE f.person_id IN (`+list+`) AND f.ignored=0 AND m.admin_only=0 GROUP BY f.person_id,f.directory ORDER BY f.person_id,min(f.id)`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, face int64
		var dir string
		if err = rows.Scan(&id, &dir, &face); err != nil {
			break
		}
		p, ok := out[id]
		if ok && p.FaceID == 0 && !visibility.private(dir) {
			p.FaceID = face
			out[id] = p
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	for id, p := range out {
		if p.FaceID == 0 {
			delete(out, id)
		}
	}
	rows, err = tx.QueryContext(ctx, `SELECT person_id,tag FROM person_tag_index WHERE person_id IN (`+list+`) ORDER BY person_id,tag`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var tag string
		if err = rows.Scan(&id, &tag); err != nil {
			break
		}
		if p, ok := out[id]; ok {
			p.Tags = append(p.Tags, tag)
			out[id] = p
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	return out, err
}

func familyTreeRelations(ctx context.Context, tx *sql.Tx, ids []int64) ([]FamilyTreeRelation, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	list := sqlutil.Placeholders(len(ids))
	args := make([]any, 0, len(ids)*6)
	for range 6 {
		for _, id := range ids {
			args = append(args, id)
		}
	}
	// Every direction uses an existing endpoint index. UNION ALL preserves repeated
	// marriages; the caller deduplicates visits using the marriage record ID.
	rows, err := tx.QueryContext(ctx, `SELECT 0,parent_id,person_id,role,'','' FROM person_parents WHERE person_id IN (`+list+`)
 UNION ALL SELECT 0,parent_id,person_id,role,'','' FROM person_parents WHERE parent_id IN (`+list+`)
 UNION ALL SELECT 0,person_a,person_b,'sibling','','' FROM person_siblings WHERE person_a IN (`+list+`)
 UNION ALL SELECT 0,person_a,person_b,'sibling','','' FROM person_siblings WHERE person_b IN (`+list+`)
 UNION ALL SELECT id,person_a,person_b,'marriage',wedding_date,divorce_date FROM person_marriages WHERE person_a IN (`+list+`)
 UNION ALL SELECT id,person_a,person_b,'marriage',wedding_date,divorce_date FROM person_marriages WHERE person_b IN (`+list+`) LIMIT ?`, append(args, MaxFamilyTreeRelations*2+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FamilyTreeRelation{}
	for rows.Next() {
		var e FamilyTreeRelation
		if err := rows.Scan(&e.ID, &e.From, &e.To, &e.Kind, &e.WeddingDate, &e.DivorceDate); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if len(out) > MaxFamilyTreeRelations*2 {
		return nil, ErrFamilyTreeLarge
	}
	return out, rows.Err()
}

// FamilyTrees walks both directions through all recorded relation types and then
// partitions the visible graph into connected components. Selected relatives share
// a single component, and each person and relationship is emitted exactly once.
func (l *Library) FamilyTrees(ctx context.Context) (FamilyTreePage, error) {
	out := FamilyTreePage{Trees: []FamilyTree{}}
	tx, err := l.index.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	roots, _, err := familyTreeRootIDs(ctx, tx)
	if err != nil {
		return out, err
	}
	if len(roots) == 0 {
		return out, sql.ErrNoRows
	}
	queue := slices.Clone(roots)
	seen := map[int64]bool{}
	for _, id := range roots {
		seen[id] = true
	}
	people := map[int64]FamilyTreePerson{}
	edges := map[string]FamilyTreeRelation{}
	visibility := newFaceDirectoryVisibility(l.root)
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		count := min(200, len(queue))
		batch := queue[:count]
		queue = queue[count:]
		visible, err := l.familyTreePeople(ctx, tx, batch, visibility)
		if err != nil {
			return out, err
		}
		ids := make([]int64, 0, len(visible))
		for id, p := range visible {
			people[id] = p
			ids = append(ids, id)
		}
		slices.Sort(ids)
		relations, err := familyTreeRelations(ctx, tx, ids)
		if err != nil {
			return out, err
		}
		for _, edge := range relations {
			key := fmt.Sprintf("%s:%d:%d:%d", edge.Kind, edge.ID, edge.From, edge.To)
			edges[key] = edge
			for _, id := range []int64{edge.From, edge.To} {
				if !seen[id] {
					seen[id] = true
					queue = append(queue, id)
				}
			}
		}
		if len(seen) > MaxFamilyTreePeople || len(edges) > MaxFamilyTreeRelations {
			return out, ErrFamilyTreeLarge
		}
	}
	// Union/find is iterative, including long chains and cyclic sibling/marriage links.
	parent := map[int64]int64{}
	for id := range people {
		parent[id] = id
	}
	find := func(id int64) int64 {
		for parent[id] != id {
			parent[id] = parent[parent[id]]
			id = parent[id]
		}
		return id
	}
	for _, edge := range edges {
		if _, ok := people[edge.From]; !ok {
			continue
		}
		if _, ok := people[edge.To]; !ok {
			continue
		}
		a, b := find(edge.From), find(edge.To)
		if a != b {
			parent[max(a, b)] = min(a, b)
		}
	}
	components := map[int64]int{}
	for _, id := range roots {
		if _, ok := people[id]; !ok {
			continue
		}
		component := find(id)
		index, ok := components[component]
		if !ok {
			index = len(out.Trees)
			components[component] = index
			out.Trees = append(out.Trees, FamilyTree{ID: id, Roots: []int64{}, People: []FamilyTreePerson{}, Relations: []FamilyTreeRelation{}})
		}
		out.Trees[index].Roots = append(out.Trees[index].Roots, id)
	}
	for id, p := range people {
		if index, ok := components[find(id)]; ok {
			out.Trees[index].People = append(out.Trees[index].People, p)
		}
	}
	for _, edge := range edges {
		if _, ok := people[edge.From]; !ok {
			continue
		}
		if _, ok := people[edge.To]; !ok {
			continue
		}
		if index, ok := components[find(edge.From)]; ok {
			out.Trees[index].Relations = append(out.Trees[index].Relations, edge)
		}
	}
	for i := range out.Trees {
		tree := &out.Trees[i]
		slices.SortFunc(tree.People, func(a, b FamilyTreePerson) int {
			if a.ID < b.ID {
				return -1
			}
			if a.ID > b.ID {
				return 1
			}
			return 0
		})
		slices.SortFunc(tree.Relations, func(a, b FamilyTreeRelation) int { return compareFamilyTreeRelation(a, b) })
	}
	return out, tx.Commit()
}

func compareFamilyTreeRelation(a, b FamilyTreeRelation) int {
	if a.From != b.From {
		if a.From < b.From {
			return -1
		}
		return 1
	}
	if a.To != b.To {
		if a.To < b.To {
			return -1
		}
		return 1
	}
	if a.Kind < b.Kind {
		return -1
	}
	if a.Kind > b.Kind {
		return 1
	}
	if a.ID < b.ID {
		return -1
	}
	if a.ID > b.ID {
		return 1
	}
	return 0
}
