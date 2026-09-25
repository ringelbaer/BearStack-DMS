package transfers

import (
	"context"
	"encoding/json"
)

const jobSelect = `SELECT j.id,j.connection_id,j.revision,j.actor,j.selection,j.source_name,j.target,j.base,j.state,j.error,j.created,j.expires,j.updated,j.attempts,j.retry_at,j.submitted,j.target_exists,c.data,
 t.total,t.bytes,t.missing,t.missing_bytes,t.existing,t.conflicts,t.done,t.failed,t.uploaded_bytes
 FROM jobs j JOIN connections c ON c.id=j.connection_id JOIN job_totals t ON t.job_id=j.id`

func scanJob(row interface{ Scan(...any) error }) (Job, error) {
	var j Job
	var selection, base, conn string
	err := row.Scan(&j.ID, &j.ConnectionID, &j.Revision, &j.Actor, &selection, &j.SourceName, &j.Target, &base, &j.State, &j.Error, &j.Created, &j.Expires, &j.Updated, &j.Attempts, &j.RetryAt, &j.Submitted, &j.TargetExists, &conn,
		&j.Total, &j.Bytes, &j.Missing, &j.MissingBytes, &j.Existing, &j.Conflicts, &j.Done, &j.Failed, &j.UploadedBytes)
	if err != nil {
		return j, err
	}
	if err = json.Unmarshal([]byte(selection), &j.Selection); err != nil {
		return j, err
	}
	if err = json.Unmarshal([]byte(base), &j.Base); err != nil {
		return j, err
	}
	var c Connection
	if err = json.Unmarshal([]byte(conn), &c); err != nil {
		return j, err
	}
	j.ConnectionName, j.Provider = c.Name, c.Provider
	return j, nil
}

func (e *Engine) addJobProgress(jobs []Job) {
	e.progressMu.Lock()
	defer e.progressMu.Unlock()
	for i := range jobs {
		for _, n := range e.progress[jobs[i].ID] {
			jobs[i].InFlightBytes += n
		}
	}
}

func (e *Engine) Job(ctx context.Context, id string) (Job, error) {
	j, err := scanJob(e.db.QueryRowContext(ctx, jobSelect+` WHERE j.id=?`, id))
	if err != nil {
		return j, err
	}
	jobs := []Job{j}
	e.addJobProgress(jobs)
	return jobs[0], nil
}

func jobListQuery(connection string, page int) (string, []any) {
	query := jobSelect + ` WHERE j.submitted=1`
	args := []any{}
	if connection != "" {
		query += ` AND j.connection_id=?`
		args = append(args, connection)
	}
	query += ` ORDER BY j.created DESC,j.id DESC LIMIT 30 OFFSET ?`
	args = append(args, (max(1, min(page, 1000000))-1)*30)
	return query, args
}

func (e *Engine) Jobs(ctx context.Context, connection string, page int) ([]Job, error) {
	query, args := jobListQuery(connection, page)
	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	e.addJobProgress(jobs)
	return jobs, nil
}
