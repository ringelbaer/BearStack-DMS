package photos

import (
	"context"
	"path/filepath"
	"testing"
)

// 10,000 groups, 50,000 photos and two detections per photo. All content stays
// in a temporary metadata index; no originals or inference models are read.
func BenchmarkPeopleSort(b *testing.B) {
	l, err := New(b.TempDir(), filepath.Join(b.TempDir(), "cache"), filepath.Join(b.TempDir(), "photos.db"), 60)
	if err != nil {
		b.Fatal(err)
	}
	defer l.Close()
	statements := []string{
		`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<10000)
 INSERT INTO photo_people(id,name,name_fold,manual_name) SELECT x,printf('Person %05d',x),printf('person %05d',x),1 FROM n`,
		`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<50000)
 INSERT INTO media_index(path,name,directory,type,mime_type,size_bytes,mod_time_unix_nano,captured_at,indexed_at)
 SELECT printf('%05d.jpg',x),printf('%05d.jpg',x),'','image','image/jpeg',1,1700000000000000000+x,'2024-01-01T12:00:00Z','' FROM n`,
		`INSERT INTO photo_faces(person_id,path,directory,x,y,width,height,confidence,embedding,model)
 SELECT (rowid-1)/5+1,path,'',.1,.1,.2,.2,.99,zeroblob(512),'fixture' FROM media_index`,
		`INSERT INTO photo_faces(person_id,path,directory,x,y,width,height,confidence,embedding,model)
 SELECT person_id,path,directory,x,y,width,height,confidence,embedding,model FROM photo_faces`,
	}
	for _, statement := range statements {
		if _, err := l.index.db.Exec(statement); err != nil {
			b.Fatal(err)
		}
	}
	for _, sorting := range []string{"name_asc", "name_desc", "count_desc", "folder_asc", "date_desc"} {
		b.Run(sorting, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				page, err := l.People(context.Background(), 0, 1, "", false, false, sorting)
				if err != nil || len(page.People) != 60 || page.People[0].Count != 5 {
					b.Fatalf("page: %+v %v", page, err)
				}
			}
		})
	}
}
