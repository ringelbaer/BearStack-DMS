package photos

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"unicode/utf8"

	"bearstack/internal/facerec"
	"bearstack/internal/searchtext"

	"github.com/coder/hnsw"
)

func encodeVector(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(x))
	}
	return b
}

func decodeVector(b []byte) []float32 {
	if len(b) != facerec.Dimensions*4 {
		return nil
	}
	v := make([]float32, facerec.Dimensions)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
		if math.IsNaN(float64(v[i])) || math.IsInf(float64(v[i]), 0) {
			return nil
		}
	}
	return v
}

func cosine(a, b []float32) float64 {
	if len(a) != len(b) {
		return -1
	}
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

func (l *Library) ensureFaceGraph(ctx context.Context, model string) error {
	rt := &l.faceRuntime
	var rev int64
	if err := l.index.db.QueryRowContext(ctx, `SELECT revision FROM photo_face_state WHERE id=1`).Scan(&rev); err != nil {
		return err
	}
	if rt.graph != nil && rt.revision == rev && rt.model == model {
		return nil
	}
	rt.graph = hnsw.NewGraph[int64]()
	rt.graph.Distance = hnsw.CosineDistance
	rt.people = map[int64]int64{}
	rt.nodes = map[int64][]int64{}
	rt.model = model
	rows, err := l.index.db.QueryContext(ctx, `SELECT f.id,f.person_id,f.embedding FROM photo_face_references r JOIN photo_faces f ON f.id=r.face_id JOIN media_index m ON m.path=f.path WHERE f.model=? AND f.ignored=0 AND m.admin_only=0 ORDER BY f.id`, model)
	if err != nil {
		rt.graph = nil
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, p int64
		var b []byte
		if err = rows.Scan(&id, &p, &b); err != nil {
			rt.graph = nil
			return err
		}
		if v := decodeVector(b); v != nil {
			rt.graph.Add(hnsw.MakeNode(id, v))
			rt.people[id] = p
			rt.nodes[p] = append(rt.nodes[p], id)
		}
		if err = ctx.Err(); err != nil {
			rt.graph = nil
			return err
		}
	}
	if err = rows.Err(); err != nil {
		rt.graph = nil
		return err
	}
	rt.revision = rev
	return nil
}

func (l *Library) nearestPerson(ctx context.Context, tx *sql.Tx, v []float32, excluded map[int64]bool) int64 {
	rt := &l.faceRuntime
	if len(rt.people) == 0 {
		return 0
	}
	scores := map[int64]float64{}
	for _, n := range rt.graph.Search(v, 25) {
		p := rt.people[n.Key]
		var source, labelSource string
		if err := tx.QueryRowContext(ctx, `SELECT f.path,p.name_source FROM photo_faces f JOIN photo_people p ON p.id=f.person_id WHERE f.id=? AND f.ignored=0`, n.Key).Scan(&source, &labelSource); err != nil {
			continue
		}
		if private, e := l.MediaAdminOnly(source); e != nil || private {
			continue
		}
		if labelSource != "" {
			if private, e := l.MediaAdminOnly(labelSource); e != nil || private {
				continue
			}
		}
		if excluded[p] {
			continue
		}
		score := cosine(v, n.Value)
		if old, ok := scores[p]; !ok || score > old {
			scores[p] = score
		}
	}
	best, second := -1.0, -1.0
	var id int64
	for p, s := range scores {
		if s > best {
			second = best
			best = s
			id = p
		} else if s > second {
			second = s
		}
	}
	// Deliberately stricter than the pair-verification example threshold: a wrong
	// automatic identity is more costly than two groups the user can merge.
	if best < 0.55 || best-second < 0.08 {
		return 0
	}
	return id
}

func overlap(a, b Face) float64 {
	x := math.Max(a.X, b.X)
	y := math.Max(a.Y, b.Y)
	w := math.Max(0, math.Min(a.X+a.Width, b.X+b.Width)-x)
	h := math.Max(0, math.Min(a.Y+a.Height, b.Y+b.Height)-y)
	inter := w * h
	area := a.Width*a.Height + b.Width*b.Height - inter
	if area <= 0 {
		return 0
	}
	return inter / area
}

func normalizedPersonName(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 200 || strings.ContainsAny(name, "\x00\r\n") {
		return "", errors.New("ungültiger Personenname")
	}
	return name, nil
}

func (l *Library) CommitFaceResult(ctx context.Context, j FaceJob, result facerec.Result) error {
	if result.Model != j.Model || len(result.Faces) > facerec.MaxFaces {
		return errors.New("inkompatible Gesichtsanalyse")
	}
	for i := range result.Faces {
		if err := facerec.Validate(&result.Faces[i]); err != nil {
			return err
		}
	}
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	media, err := l.MediaContext(ctx, j.Path)
	if err != nil {
		return err
	}
	if media.AdminOnly {
		return ErrAdminOnly()
	}
	if media.SizeBytes != j.Size || media.ModTime.UnixNano() != j.ModTime || media.XMPFingerprint != j.XMP {
		return errors.New("Foto während Analyse geändert")
	}
	if err = l.ensureFaceGraph(ctx, j.Model); err != nil {
		return err
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var baseRevision int64
	if err = tx.QueryRowContext(ctx, `SELECT revision FROM photo_face_state WHERE id=1`).Scan(&baseRevision); err != nil {
		return err
	}
	if baseRevision != l.faceRuntime.revision {
		l.faceRuntime.graph = nil
		return errors.New("Referenzindex wurde während der Analyse geändert")
	}

	var size, mtime int64
	var private int
	var xmp string
	if err = tx.QueryRowContext(ctx, `SELECT size_bytes,mod_time_unix_nano,admin_only,xmp_fingerprint FROM media_index WHERE path=?`, j.Path).Scan(&size, &mtime, &private, &xmp); err != nil {
		return err
	}
	if private != 0 || size != j.Size || mtime != j.ModTime || xmp != j.XMP {
		return errors.New("Foto während Analyse geändert")
	}
	oldRows, err := tx.QueryContext(ctx, `SELECT f.id,f.person_id,f.x,f.y,f.width,f.height,(f.manual OR p.manual_name),f.ignored FROM photo_faces f JOIN photo_people p ON p.id=f.person_id WHERE f.path=?`, j.Path)
	if err != nil {
		return err
	}
	var old []RecognizedFace
	for oldRows.Next() {
		var f RecognizedFace
		if err = oldRows.Scan(&f.ID, &f.PersonID, &f.X, &f.Y, &f.Width, &f.Height, &f.Manual, &f.Ignored); err != nil {
			oldRows.Close()
			return err
		}
		old = append(old, f)
	}
	err = oldRows.Err()
	oldRows.Close()
	if err != nil {
		return err
	}
	affected := map[int64]bool{}
	for _, f := range old {
		affected[f.PersonID] = true
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM photo_faces WHERE path=?`, j.Path); err != nil {
		return err
	}
	used := map[int64]bool{}
	for _, d := range result.Faces {
		box := Face{X: d.X, Y: d.Y, Width: d.Width, Height: d.Height}
		var person int64
		manual, ignored := false, false
		xmpConflict := false
		// Carry overrides only on a one-to-one region match, never by detection order.
		var matches []RecognizedFace
		for _, f := range old {
			if (f.Manual || f.Ignored) && overlap(box, Face{X: f.X, Y: f.Y, Width: f.Width, Height: f.Height}) >= 0.7 {
				matches = append(matches, f)
			}
		}
		if len(matches) == 1 {
			f := matches[0]
			n := 0
			for _, other := range result.Faces {
				if overlap(Face{X: other.X, Y: other.Y, Width: other.Width, Height: other.Height}, Face{X: f.X, Y: f.Y, Width: f.Width, Height: f.Height}) >= 0.7 {
					n++
				}
			}
			if n == 1 {
				person, manual, ignored = f.PersonID, f.Manual, f.Ignored
			}
		}
		if person == 0 {
			names := map[string]bool{}
			for _, f := range media.Faces {
				if overlap(box, f) >= 0.5 {
					if name, e := normalizedPersonName(f.Name); e == nil && name != "" {
						names[name] = true
					}
				}
			}
			xmpConflict = len(names) > 1
			if len(names) == 1 {
				for name := range names {
					var count int
					if err = tx.QueryRowContext(ctx, `SELECT count(*),coalesce(min(id),0) FROM photo_people WHERE name_fold=?`, searchtext.GermanFold(name)).Scan(&count, &person); err != nil {
						return err
					}
					if count == 0 {
						candidate := l.nearestPerson(ctx, tx, d.Embedding, used)
						if candidate != 0 {
							var currentName string
							var manualName bool
							if err = tx.QueryRowContext(ctx, `SELECT name,manual_name FROM photo_people WHERE id=?`, candidate).Scan(&currentName, &manualName); err != nil {
								return err
							}
							if currentName == "" {
								person = candidate
								if !manualName {
									if _, err = tx.ExecContext(ctx, `UPDATE photo_people SET name=?,name_fold=?,name_source=? WHERE id=?`, name, searchtext.GermanFold(name), j.Path, person); err != nil {
										return err
									}
								}
							}
						}
						if person == 0 {
							res, e := tx.ExecContext(ctx, `INSERT INTO photo_people(name,name_fold,name_source) VALUES(?,?,?)`, name, searchtext.GermanFold(name), j.Path)
							if e != nil {
								return e
							}
							person, _ = res.LastInsertId()
						}

					} else if count > 1 {
						xmpConflict = true
						person = 0
					}
				}
			}
		}
		if person == 0 && !ignored && !xmpConflict {
			person = l.nearestPerson(ctx, tx, d.Embedding, used)
		}
		if person == 0 {
			res, e := tx.ExecContext(ctx, `INSERT INTO photo_people DEFAULT VALUES`)
			if e != nil {
				return e
			}
			person, _ = res.LastInsertId()
		}
		used[person] = true
		affected[person] = true
		_, err = tx.ExecContext(ctx, `INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model,manual,ignored) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, j.Path, media.Directory, person, d.X, d.Y, d.Width, d.Height, d.Confidence, encodeVector(d.Embedding), j.Model, manual, ignored)
		if err != nil {
			return err
		}
	}
	for p := range affected {
		if err = refreshFaceReferencesTx(ctx, tx, p); err != nil {
			return err
		}
	}
	res, err := tx.ExecContext(ctx, `UPDATE photo_face_jobs SET status='done',error='',attempts=0,retry_at=0 WHERE path=? AND source_size=? AND source_mtime=? AND source_xmp=? AND model=?`, j.Path, j.Size, j.ModTime, j.XMP, j.Model)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return errors.New("Gesichtsauftrag wurde ersetzt")
	}
	var committedRevision int64
	if err = tx.QueryRowContext(ctx, `SELECT revision FROM photo_face_state WHERE id=1`).Scan(&committedRevision); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return l.syncFaceGraphPeople(ctx, affected, committedRevision)
}

func refreshFaceReferencesTx(ctx context.Context, tx *sql.Tx, p int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM photo_face_references WHERE person_id=?`, p); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO photo_face_references(face_id,person_id) SELECT f.id,f.person_id FROM photo_faces f JOIN media_index m ON m.path=f.path WHERE f.person_id=? AND f.ignored=0 AND m.admin_only=0 AND f.model=(SELECT model FROM photo_face_state WHERE id=1) ORDER BY f.manual DESC,f.confidence DESC,f.id LIMIT 5`, p)
	return err
}

func (l *Library) syncFaceGraphPeople(ctx context.Context, people map[int64]bool, committedRevision int64) error {
	rt := &l.faceRuntime
	if rt.graph == nil {
		return nil
	}
	for p := range people {
		rows, err := l.index.db.QueryContext(ctx, `SELECT f.id,f.embedding FROM photo_face_references r JOIN photo_faces f ON f.id=r.face_id WHERE r.person_id=? AND f.model=?`, p, rt.model)
		if err != nil {
			rt.graph = nil
			return err
		}
		next := map[int64][]float32{}
		for rows.Next() {
			var id int64
			var b []byte
			if err = rows.Scan(&id, &b); err != nil {
				rows.Close()
				rt.graph = nil
				return err
			}
			if v := decodeVector(b); v != nil {
				next[id] = v
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			rt.graph = nil
			return err
		}
		for _, id := range rt.nodes[p] {
			if _, ok := next[id]; !ok {
				rt.graph.Delete(id)
				delete(rt.people, id)
			}
		}
		// hnsw v0.6.1 leaves empty layers after deleting the final node.
		if len(rt.people) == 0 {
			rt.graph = hnsw.NewGraph[int64]()
			rt.graph.Distance = hnsw.CosineDistance
		}
		rt.nodes[p] = nil
		for id, v := range next {
			if _, ok := rt.people[id]; !ok {
				rt.graph.Add(hnsw.MakeNode(id, v))
			}
			rt.people[id] = p
			rt.nodes[p] = append(rt.nodes[p], id)
		}
	}
	// Never acknowledge concurrent index deletions that were not synchronized here.
	rt.revision = committedRevision
	return nil
}
