package transfers

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func checkJobTotals(t *testing.T, e *Engine, id string) {
	t.Helper()
	j, err := e.Job(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	want := [9]int64{}
	rows, err := e.db.Query("SELECT size,state FROM items WHERE job_id=?", id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var size int64
		var state string
		if err := rows.Scan(&size, &state); err != nil {
			t.Fatal(err)
		}
		want[0]++
		want[1] += size
		switch state {
		case "missing", "uploading":
			want[2]++
			want[3] += size
		case "existing":
			want[4]++
		case "conflict":
			want[5]++
		case "done":
			want[6]++
			want[8] += size
		case "failed":
			want[7]++
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	got := [9]int64{j.Total, j.Bytes, j.Missing, j.MissingBytes, j.Existing, j.Conflicts, j.Done, j.Failed, j.UploadedBytes}
	if got != want {
		t.Fatalf("job %s totals = %v, want %v", id, got, want)
	}
}

func TestJobTotalsTrackMutationsAndRollback(t *testing.T) {
	e := stateEngine(t)
	stateJob(t, e, "a", "paused", true)
	stateJob(t, e, "b", "paused", true)
	checkJobTotals(t, e, "a")
	for i, state := range []string{"missing", "uploading", "existing", "conflict", "done", "failed"} {
		_, err := e.db.Exec(`INSERT INTO items(job_id,path,display_path,relative,directory,size,modified,state) VALUES('a',?, '',?, '',?,0,?)`, fmt.Sprint(i), fmt.Sprint(i), (i+1)*13, state)
		if err != nil {
			t.Fatal(err)
		}
		checkJobTotals(t, e, "a")
	}
	for _, mutation := range []string{
		`UPDATE items SET state='done' WHERE state='uploading'`,
		`UPDATE items SET state='missing' WHERE state IN ('failed','conflict')`,
		`UPDATE items SET size=0 WHERE state='existing'`,
		`UPDATE items SET size=2147483648 WHERE state='done'`,
		`UPDATE items SET resume='{"version":1}',error='retry'`,
		`UPDATE items SET job_id='b',size=size+1 WHERE state='done'`,
		`DELETE FROM items WHERE job_id='a' AND state='existing'`,
	} {
		if _, err := e.db.Exec(mutation); err != nil {
			t.Fatal(err)
		}
		checkJobTotals(t, e, "a")
		checkJobTotals(t, e, "b")
	}
	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec("DELETE FROM items"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	checkJobTotals(t, e, "a")
	checkJobTotals(t, e, "b")
	if _, err = e.db.Exec("DELETE FROM jobs WHERE id='a'"); err != nil {
		t.Fatal(err)
	}
	var retained int
	if err = e.db.QueryRow("SELECT count(*) FROM job_totals WHERE job_id='a'").Scan(&retained); err != nil || retained != 0 {
		t.Fatalf("deleted totals retained: %d %v", retained, err)
	}
	checkJobTotals(t, e, "b")
}

func TestJobTotalsUpgradeAndRecovery(t *testing.T) {
	e := stateEngine(t)
	stateJob(t, e, "old", "running", true)
	stateJob(t, e, "empty", "complete", true)
	_, err := e.db.Exec(`INSERT INTO items(job_id,path,display_path,relative,directory,size,modified,state)
 VALUES('old','a','','a','',17,0,'uploading'),('old','b','','b','',23,0,'done');
 DROP TRIGGER job_totals_create; DROP TRIGGER job_totals_insert;
 DROP TRIGGER job_totals_update; DROP TRIGGER job_totals_delete;
 DROP TABLE job_totals; DROP INDEX jobs_history; DROP INDEX jobs_connection_history;
 PRAGMA user_version=1;`)
	if err != nil {
		t.Fatal(err)
	}
	var dbPath string
	if err = e.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&dbPath); err != nil {
		t.Fatal(err)
	}
	e.Close()
	for i := 0; i < 2; i++ {
		reopened, err := Open(filepath.Dir(dbPath), Registry{}, nil, func(context.Context, string) bool { return true })
		if err != nil {
			t.Fatal(err)
		}
		checkJobTotals(t, reopened, "old")
		checkJobTotals(t, reopened, "empty")
		j, err := reopened.Job(context.Background(), "old")
		if err != nil || j.State != "queued" || j.Total != 2 || j.Bytes != 40 || j.MissingBytes != 17 || j.UploadedBytes != 23 {
			t.Fatalf("upgraded job = %+v, %v", j, err)
		}
		reopened.Close()
	}
}

func TestJobsPagesFiltersProgressAndQueryPlan(t *testing.T) {
	e := stateEngine(t)
	ctx := context.Background()
	if _, err := e.db.Exec("INSERT INTO connections(id,data) VALUES('other',?)", encode(Connection{ID: "other", Name: "Other", Provider: "opaque"})); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 65; i++ {
		id := fmt.Sprintf("%02d", i)
		stateJob(t, e, id, "complete", true)
		if _, err := e.db.Exec("UPDATE jobs SET created=1 WHERE id=?", id); err != nil {
			t.Fatal(err)
		}
	}
	stateJob(t, e, "preview", "ready", false)
	if _, err := e.db.Exec("UPDATE jobs SET connection_id='other' WHERE id='64'"); err != nil {
		t.Fatal(err)
	}
	e.progress["64"] = map[int64]int64{1: 12, 2: 34}
	for _, tc := range []struct {
		connection   string
		page, length int
		first        string
	}{{"", 1, 30, "64"}, {"", 2, 30, "34"}, {"", 3, 5, "04"}, {"", 4, 0, ""}, {"", 0, 30, "64"}, {"other", 1, 1, "64"}, {"c", 1, 30, "63"}, {"absent", 1, 0, ""}} {
		jobs, err := e.Jobs(ctx, tc.connection, tc.page)
		if err != nil || len(jobs) != tc.length || (len(jobs) > 0 && jobs[0].ID != tc.first) {
			t.Fatalf("page %+v: %+v, %v", tc, jobs, err)
		}
		for _, job := range jobs {
			individual, err := e.Job(ctx, job.ID)
			if err != nil || !reflect.DeepEqual(job, individual) {
				t.Fatalf("list/detail mismatch: %+v %+v %v", job, individual, err)
			}
		}
	}
	j, err := e.Job(ctx, "64")
	if err != nil || j.InFlightBytes != 46 || j.ConnectionName != "Other" {
		t.Fatalf("live progress/connection missing: %+v %v", j, err)
	}
	for _, connection := range []string{"", "c"} {
		query, args := jobListQuery(connection, 1)
		rows, err := e.db.Query("EXPLAIN QUERY PLAN "+query, args...)
		if err != nil {
			t.Fatal(err)
		}
		plan := ""
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plan += detail + "\n"
		}
		err = rows.Err()
		rows.Close()
		index := "jobs_history"
		if connection != "" {
			index = "jobs_connection_history"
		}
		if err != nil || !strings.Contains(plan, index) || strings.Contains(plan, "TEMP B-TREE") || strings.Contains(plan, "items") {
			t.Fatalf("unbounded history work: %s %v", plan, err)
		}
	}
}

func BenchmarkTransferJobPage(b *testing.B) {
	for _, files := range []int{1, 10000} {
		b.Run(fmt.Sprintf("30_jobs_%d_files_each", files), func(b *testing.B) {
			e, err := Open(b.TempDir(), Registry{}, nil, nil)
			if err != nil {
				b.Fatal(err)
			}
			defer e.Close()
			tx, err := e.db.Begin()
			if err != nil {
				b.Fatal(err)
			}
			defer tx.Rollback()
			if _, err = tx.Exec("INSERT INTO connections(id,data) VALUES('c','{}')"); err != nil {
				b.Fatal(err)
			}
			stmt, err := tx.Prepare(`INSERT INTO items(job_id,path,display_path,relative,directory,size,modified,state) VALUES(?,?,'',?,'',100,0,'done')`)
			if err != nil {
				b.Fatal(err)
			}
			defer stmt.Close()
			for j := 0; j < 30; j++ {
				id := fmt.Sprint(j)
				if _, err = tx.Exec(`INSERT INTO jobs(id,connection_id,revision,actor,selection,source_name,target,base,state,created,expires,updated,submitted) VALUES(?,'c',1,'a','{}','s','t','{}','complete',1,2,1,1)`, id); err != nil {
					b.Fatal(err)
				}
				for i := 0; i < files; i++ {
					if _, err = stmt.Exec(id, fmt.Sprint(i), fmt.Sprint(i)); err != nil {
						b.Fatal(err)
					}
				}
			}
			if err = tx.Commit(); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				jobs, err := e.Jobs(context.Background(), "", 1)
				if err != nil || len(jobs) != 30 || jobs[0].Total != int64(files) {
					b.Fatalf("page %v %v", jobs, err)
				}
			}
		})
	}
}
