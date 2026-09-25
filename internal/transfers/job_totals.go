package transfers

import (
	"context"
	"database/sql"
	"strings"
)

// These counters replace scans of every item on each progress request. Triggers
// keep them in the same transaction as the items, including retries, recovery,
// bulk planning, deletes and rollback. Resume/progress-only writes do no work.
var jobTotalColumns = []struct{ name, expression string }{
	{"total", "1"},
	{"bytes", "i.size"},
	{"missing", "i.state IN ('missing','uploading')"},
	{"missing_bytes", "CASE WHEN i.state IN ('missing','uploading') THEN i.size ELSE 0 END"},
	{"existing", "i.state='existing'"},
	{"conflicts", "i.state='conflict'"},
	{"done", "i.state='done'"},
	{"failed", "i.state='failed'"},
	{"uploaded_bytes", "CASE WHEN i.state='done' THEN i.size ELSE 0 END"},
}

func setupJobTotals(ctx context.Context, db *sql.DB, version int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS jobs_history ON jobs(created DESC,id DESC) WHERE submitted=1;
CREATE INDEX IF NOT EXISTS jobs_connection_history ON jobs(connection_id,created DESC,id DESC) WHERE submitted=1;`); err != nil {
		return err
	}
	if version < 2 {
		var definitions, names, sums, assignments, insert, remove, update []string
		for _, column := range jobTotalColumns {
			name := column.name
			definitions = append(definitions, name+" INTEGER NOT NULL DEFAULT 0")
			names = append(names, name)
			sums = append(sums, "SUM("+column.expression+")")
			assignments = append(assignments, name+"=excluded."+name)
			old := strings.ReplaceAll(column.expression, "i.", "OLD.")
			new := strings.ReplaceAll(column.expression, "i.", "NEW.")
			insert = append(insert, name+"="+name+"+("+new+")")
			remove = append(remove, name+"="+name+"-("+old+")")
			update = append(update, name+"="+name+
				"-CASE WHEN job_id=OLD.job_id THEN ("+old+") ELSE 0 END"+
				"+CASE WHEN job_id=NEW.job_id THEN ("+new+") ELSE 0 END")
		}
		_, err = tx.ExecContext(ctx, `
CREATE TABLE job_totals(job_id TEXT PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,`+strings.Join(definitions, ",")+`);
INSERT INTO job_totals(job_id) SELECT id FROM jobs;
INSERT INTO job_totals(job_id,`+strings.Join(names, ",")+`)
 SELECT i.job_id,`+strings.Join(sums, ",")+` FROM items i GROUP BY i.job_id
 ON CONFLICT(job_id) DO UPDATE SET `+strings.Join(assignments, ",")+`;
CREATE TRIGGER job_totals_create AFTER INSERT ON jobs BEGIN
 INSERT INTO job_totals(job_id) VALUES(NEW.id);
END;
CREATE TRIGGER job_totals_insert AFTER INSERT ON items BEGIN
 UPDATE job_totals SET `+strings.Join(insert, ",")+` WHERE job_id=NEW.job_id;
END;
CREATE TRIGGER job_totals_delete AFTER DELETE ON items BEGIN
 UPDATE job_totals SET `+strings.Join(remove, ",")+` WHERE job_id=OLD.job_id;
END;
CREATE TRIGGER job_totals_update AFTER UPDATE OF job_id,size,state ON items
 WHEN OLD.job_id<>NEW.job_id OR OLD.size<>NEW.size OR OLD.state<>NEW.state BEGIN
 UPDATE job_totals SET `+strings.Join(update, ",")+` WHERE job_id IN (OLD.job_id,NEW.job_id);
END;
PRAGMA user_version=2;`)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
