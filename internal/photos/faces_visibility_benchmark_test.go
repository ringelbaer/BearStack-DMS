package photos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"bearstack/internal/facerec"
)

func BenchmarkFaceVisibility(b *testing.B) {
	ctx := context.Background()
	root, data := b.TempDir(), b.TempDir()
	l, err := New(root, filepath.Join(data, "cache"), filepath.Join(data, "photos.db"), 60)
	if err != nil {
		b.Fatal(err)
	}
	defer l.Close()
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer tx.Rollback()
	vector := encodeVector(faceDetection(0).Embedding)
	for i := 1; i <= 1000; i++ {
		dir := fmt.Sprintf("year/album%04d", i)
		if err := os.MkdirAll(filepath.Join(root, dir), 0750); err != nil {
			b.Fatal(err)
		}
		path := dir + "/photo.jpg"
		if _, err := tx.Exec(`INSERT INTO media_index(path,name,directory,type,mime_type,size_bytes,mod_time_unix_nano,indexed_at) VALUES(?, 'photo.jpg',?,'image','image/jpeg',1,1,'')`, path, dir); err != nil {
			b.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO photo_people(id) VALUES(?)`, i); err != nil {
			b.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model) VALUES(?,?,?,.1,.1,.2,.2,.99,?,?)`, path, dir, i, vector, facerec.Model); err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	for _, scoped := range []bool{false, true} {
		name := "AllPeople1000Directories"
		if scoped {
			name = "OnePersonOf1000Directories"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var err error
				if scoped {
					err = l.refreshPersonIDsVisibility(ctx, 1)
				} else {
					err = l.refreshPeopleVisibility(ctx, "")
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
