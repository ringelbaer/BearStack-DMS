package transfers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
)

func (e *Engine) NewPreview(ctx context.Context, connectionID, actor string, selection Selection, target string) (Job, error) {
	if !e.authorize(ctx, actor) {
		return Job{}, Fail(Permission, "Administratorrechte erforderlich")
	}
	c, err := e.Connection(ctx, connectionID)
	if err != nil {
		return Job{}, err
	}
	if !c.Enabled || !c.Connected || c.Base.ID == "" {
		return Job{}, Fail(Invalid, "Aktive Verbindung mit Basisordner erforderlich")
	}
	info, err := e.source.Describe(ctx, selection)
	if err != nil {
		return Job{}, err
	}
	if !info.Virtual || target == "" {
		target = info.Target
	}
	if err = ValidateSegments([]string{target}); err != nil {
		return Job{}, err
	}
	now := time.Now().Unix()
	j := Job{ID: ID(), ConnectionID: c.ID, ConnectionName: c.Name, Provider: c.Provider, Revision: c.Revision, Actor: actor, Selection: selection, SourceName: info.Name, Target: target, Base: c.Base, State: "preparing", Created: now, Expires: now + 1800, Updated: now}
	_, err = e.db.ExecContext(ctx, `INSERT INTO jobs(id,connection_id,revision,actor,selection,source_name,target,base,state,created,expires,updated) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, j.ID, j.ConnectionID, j.Revision, actor, encode(selection), j.SourceName, target, encode(j.Base), j.State, now, j.Expires, now)
	e.signal()
	return j, err
}
func (e *Engine) Items(ctx context.Context, id string, after int64) ([]Item, error) {
	rows, err := e.db.QueryContext(ctx, `SELECT id,path,display_path,relative,size,modified,state,error,resume FROM items WHERE job_id=? AND id>? ORDER BY id LIMIT 200`, id, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		var i Item
		var rel, resume string
		if err = rows.Scan(&i.ID, &i.Path, &i.DisplayPath, &rel, &i.Size, &i.Modified, &i.State, &i.Error, &resume); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(rel), &i.Relative); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(resume), &i.Resume); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func (e *Engine) Action(ctx context.Context, id, action string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	j, err := e.Job(ctx, id)
	if err != nil {
		return err
	}
	c, err := e.Connection(ctx, j.ConnectionID)
	if err != nil {
		return err
	}
	switch action {
	case "start", "resume", "retry":
		if !c.Enabled || !c.Connected || c.Revision != j.Revision {
			return Fail(Conflict, "Verbindung geändert; neue Vorschau erforderlich")
		}
		if !e.authorize(ctx, j.Actor) {
			return Fail(Permission, "Auftraggeber hat keine Administratorrechte mehr")
		}
		if action == "start" {
			if j.Submitted {
				return nil
			}
			if j.State != "ready" || j.Expires < time.Now().Unix() {
				return Fail(Conflict, "Vorschau nicht bereit oder abgelaufen")
			}
		}
		if action != "start" && !j.Submitted {
			return Fail(Conflict, "Neue Vorschau erforderlich")
		}
		if action == "resume" && j.State != "paused" && j.State != "waiting" {
			return Fail(Conflict, "Auftrag ist nicht pausiert")
		}
		if action == "retry" && j.State != "partial" && j.State != "failed" {
			return Fail(Conflict, "Auftrag kann nicht wiederholt werden")
		}
		tx, err := e.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if action == "retry" {
			if _, err = tx.ExecContext(ctx, "UPDATE items SET state='missing',error='' WHERE job_id=? AND state IN ('failed','conflict')", id); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, "UPDATE jobs SET state='queued',queue_seq=CASE WHEN submitted=0 THEN (SELECT coalesce(max(queue_seq),0)+1 FROM jobs) ELSE queue_seq END,submitted=1,error='',attempts=0,retry_at=0,updated=? WHERE id=?", time.Now().Unix(), id); err != nil {
			return err
		}
		err = tx.Commit()
		e.signal()
		return err
	case "pause", "cancel":
		if j.State == "complete" || j.State == "cancelled" {
			return nil
		}
		state := "paused"
		if action == "cancel" {
			state = "cancelled"
		}
		_, err = e.db.ExecContext(ctx, "UPDATE jobs SET state=?,updated=? WHERE id=?", state, time.Now().Unix(), id)
		if cancel := e.cancels[id]; cancel != nil {
			cancel()
		}
		return err
	default:
		return Fail(Invalid, "Unbekannte Auftragsaktion")
	}
}
func (e *Engine) claim(ctx context.Context, from, to string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var id string
	err := e.db.QueryRowContext(ctx, "SELECT id FROM jobs WHERE state=? AND retry_at<=? ORDER BY CASE WHEN submitted=1 THEN queue_seq ELSE created END,id LIMIT 1", from, time.Now().Unix()).Scan(&id)
	if err != nil {
		return "", err
	}
	res, err := e.db.ExecContext(ctx, "UPDATE jobs SET state=?,updated=? WHERE id=? AND state=?", to, time.Now().Unix(), id, from)
	if err != nil {
		return "", err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return "", sql.ErrNoRows
	}
	return id, nil
}
func (e *Engine) activeContext(parent context.Context, id string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	e.mu.Lock()
	e.cancels[id] = cancel
	var state string
	if err := e.db.QueryRowContext(parent, "SELECT state FROM jobs WHERE id=?", id).Scan(&state); err != nil || (state != "running" && state != "planning") {
		cancel()
	}
	e.progressMu.Lock()
	e.progress[id] = map[int64]int64{}
	e.progressMu.Unlock()
	e.mu.Unlock()
	return ctx, func() {
		cancel()
		e.mu.Lock()
		delete(e.cancels, id)
		e.mu.Unlock()
		e.progressMu.Lock()
		delete(e.progress, id)
		e.progressMu.Unlock()
	}
}
func (e *Engine) finish(id, from, to, message string) {
	_, _ = e.db.Exec("UPDATE jobs SET state=?,error=?,updated=? WHERE id=? AND state=?", to, message, time.Now().Unix(), id, from)
}
func (e *Engine) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); e.previewLoop(ctx) }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer wg.Wait()
	lastClean := time.Time{}
	for {
		if ctx.Err() != nil {
			return
		}
		now := time.Now()
		_, _ = e.db.ExecContext(ctx, "UPDATE jobs SET state='queued' WHERE state='waiting' AND retry_at<=?", now.Unix())
		if now.Sub(lastClean) > time.Hour {
			e.cleanup(ctx)
			lastClean = now
		}
		id, err := e.claim(ctx, "queued", "running")
		if err == nil {
			e.transfer(ctx, id)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-e.wake:
		}
	}
}
func (e *Engine) previewLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		id, err := e.claim(ctx, "preparing", "planning")
		if err == nil {
			e.prepare(ctx, id)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (e *Engine) prepare(parent context.Context, id string) {
	ctx, release := e.activeContext(parent, id)
	defer release()
	err := e.prepareJob(ctx, id)
	if err != nil {
		if parent.Err() != nil {
			e.finish(id, "planning", "preparing", "")
		} else {
			e.finish(id, "planning", "failed", safeError(err))
		}
		return
	}
	e.finish(id, "planning", "ready", "")
}
func safeError(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Message
	}
	if errors.Is(err, context.Canceled) {
		return "Übertragung unterbrochen"
	}
	return "Übertragung fehlgeschlagen; bitte erneut prüfen"
}
func (e *Engine) prepareJob(ctx context.Context, id string) error {
	j, err := e.Job(ctx, id)
	if err != nil {
		return err
	}
	if j.Expires <= time.Now().Unix() {
		return Fail(Conflict, "Vorschau abgelaufen; bitte erneut prüfen")
	}
	ctx, cancel := context.WithDeadline(ctx, time.Unix(j.Expires, 0))
	defer cancel()

	if !e.authorize(ctx, j.Actor) {
		return Fail(Permission, "Administratorrechte entzogen")
	}
	c, err := e.Connection(ctx, j.ConnectionID)
	if err != nil {
		return err
	}
	if c.Revision != j.Revision {
		return Fail(Conflict, "Verbindung geändert")
	}
	client, err := e.client(ctx, c)
	if err != nil {
		return err
	}
	if _, err = e.db.ExecContext(ctx, "DELETE FROM items WHERE job_id=?", id); err != nil {
		return err
	}
	if _, err = e.db.ExecContext(ctx, "DELETE FROM directories WHERE job_id=?", id); err != nil {
		return err
	}
	batch := make([]SourceItem, 0, 500)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		tx, err := e.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO items(job_id,path,display_path,relative,directory,size,modified) VALUES(?,?,?,?,?,?,?) ON CONFLICT(job_id,path) DO NOTHING`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		dirStmt, err := tx.PrepareContext(ctx, "INSERT INTO directories(job_id,relative) VALUES(?,?) ON CONFLICT DO NOTHING")
		if err != nil {
			return err
		}
		defer dirStmt.Close()
		seenDirs := map[string]bool{}
		for _, item := range batch {
			parts := append([]string{j.Target}, item.Relative...)
			if err = ValidateSegments(parts); err != nil {
				return err
			}
			if _, err = stmt.ExecContext(ctx, id, item.Path, item.DisplayPath, encode(parts), encode(parts[:len(parts)-1]), item.Size, item.Modified); err != nil {
				return err
			}
			for n := 1; n < len(parts); n++ {
				raw := encode(parts[:n])
				if !seenDirs[raw] {
					if _, err = dirStmt.ExecContext(ctx, id, raw); err != nil {
						return err
					}
					seenDirs[raw] = true
				}
			}

		}
		batch = batch[:0]
		return tx.Commit()
	}
	err = e.source.Walk(ctx, j.Selection, func(item SourceItem) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		batch = append(batch, item)
		if len(batch) == cap(batch) {
			return flush()
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err = flush(); err != nil {
		return err
	}
	target, err := client.Stat(ctx, j.Base, []string{j.Target})
	if err != nil {
		return err
	}
	// A missing export root cannot contain existing descendants. The adapter
	// resolves logical prefix directories too, so this needs no DAV knowledge.
	if target == nil {
		return nil
	}
	if target != nil {
		if _, err = e.db.ExecContext(ctx, "UPDATE jobs SET target_exists=1 WHERE id=?", id); err != nil {
			return err
		}
		if !target.Directory {
			_, err = e.db.ExecContext(ctx, "UPDATE items SET state='conflict',error='Zielordnername ist durch eine Datei belegt' WHERE job_id=?", id)
			return err
		}
	}
	rows, err := e.db.QueryContext(ctx, "SELECT relative FROM directories WHERE job_id=? ORDER BY length(relative),relative", id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return err
		}
		var dir []string
		if err = json.Unmarshal([]byte(raw), &dir); err != nil {
			return err
		}
		var blocked bool
		if err = e.db.QueryRowContext(ctx, "SELECT blocked FROM directories WHERE job_id=? AND relative=?", id, raw).Scan(&blocked); err != nil {
			return err
		}
		if blocked {
			continue
		}

		entries := make([]Entry, 0, 500)
		apply := func() error {
			if len(entries) == 0 {
				return nil
			}
			tx, err := e.db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			defer tx.Rollback()
			stmt, err := tx.PrepareContext(ctx, "UPDATE items SET state=CASE WHEN size=? AND ?=0 THEN 'existing' ELSE 'conflict' END,error=CASE WHEN size=? AND ?=0 THEN '' ELSE 'Ziel vorhanden mit anderem Typ oder anderer Größe' END WHERE job_id=? AND relative=?")
			if err != nil {
				return err
			}
			defer stmt.Close()
			for _, entry := range entries {
				if !entry.Directory {
					raw := encode(entry.Segments)
					// A remote file can occupy an ancestor directory. Mark the
					// entire selected subtree before considering any upload.
					prefix := strings.TrimSuffix(raw, "]")
					res, err := tx.ExecContext(ctx, "UPDATE directories SET blocked=1 WHERE job_id=? AND (relative=? OR (relative>=? AND relative<?))", id, raw, prefix+",", prefix+"-")
					if err != nil {
						return err
					}
					if n, _ := res.RowsAffected(); n > 0 {
						if _, err = tx.ExecContext(ctx, "UPDATE items SET state='conflict',error='Zielpfad enthält eine Datei statt eines Ordners' WHERE job_id=? AND (directory=? OR (directory>=? AND directory<?))", id, raw, prefix+",", prefix+"-"); err != nil {
							return err
						}
					}
				}
				if _, err = stmt.ExecContext(ctx, entry.Size, entry.Directory, entry.Size, entry.Directory, id, encode(entry.Segments)); err != nil {
					return err
				}
			}
			entries = entries[:0]
			return tx.Commit()
		}
		err = client.Inventory(ctx, j.Base, dir, func(entry Entry) error {
			entries = append(entries, entry)
			if len(entries) == cap(entries) {
				return apply()
			}
			return nil
		})
		if err != nil {
			if Kind(err) == Conflict {
				_, err = e.db.ExecContext(ctx, "UPDATE items SET state='conflict',error=? WHERE job_id=? AND directory=?", safeError(err), id, raw)
				if err != nil {
					return err
				}
				continue
			}
			return err
		}
		if err = apply(); err != nil {
			return err
		}
	}
	return rows.Err()
}
func (e *Engine) transfer(parent context.Context, id string) {
	ctx, release := e.activeContext(parent, id)
	defer release()
	j, err := e.Job(ctx, id)
	if err != nil {
		e.finish(id, "running", "failed", safeError(err))
		return
	}
	c, err := e.Connection(ctx, j.ConnectionID)
	if err == nil && c.Revision != j.Revision {
		err = Fail(Conflict, "Verbindung geändert; neue Vorschau erforderlich")
	}
	var client Client
	if err == nil {
		client, err = e.client(ctx, c)
	}
	if err != nil {
		e.finish(id, "running", "paused", safeError(err))
		return
	}
	// Two consumers, one bounded producer. A failed connection cancels sibling work.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	work := make(chan Item, 2)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range work {
				if runCtx.Err() != nil {
					return
				}
				err := e.uploadItem(runCtx, j, c, client, item)
				if err != nil {
					select {
					case results <- err:
					default:
					}
					cancel()
					return
				}
			}
		}()
	}
	var after int64
produce:
	for {
		items, readErr := e.Items(runCtx, id, after)
		if readErr != nil {
			err = readErr
			break
		}
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			after = item.ID
			if item.State != "missing" && item.State != "uploading" {
				continue
			}
			select {
			case work <- item:
			case <-runCtx.Done():
				break produce
			}
		}
	}
	close(work)
	wg.Wait()
	close(results)
	for result := range results {
		err = result
		break
	}
	_, _ = e.db.Exec("UPDATE items SET state='missing' WHERE job_id=? AND state='uploading'", id)
	if parent.Err() != nil {
		e.finish(id, "running", "queued", "")
		return
	}
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		kind := Kind(err)
		if (kind == Temporary || kind == Throttled) && j.Attempts < 4 {
			delay := time.Duration(1<<j.Attempts) * 5 * time.Second
			var failure *Error
			if errors.As(err, &failure) {
				delay = max(delay, failure.RetryAfter)
			}
			_, _ = e.db.Exec("UPDATE jobs SET state='waiting',error=?,attempts=attempts+1,retry_at=?,updated=? WHERE id=? AND state='running'", safeError(err), time.Now().Add(delay).Unix(), time.Now().Unix(), id)
			return
		}
		e.finish(id, "running", "paused", safeError(err))
		return
	}
	final, err := e.Job(ctx, id)
	if err != nil {
		e.finish(id, "running", "failed", safeError(err))
		return
	}
	state := "complete"
	if final.Conflicts+final.Failed > 0 {
		state = "partial"
	}
	e.finish(id, "running", state, "")
}
func (e *Engine) uploadItem(ctx context.Context, j Job, c Connection, client Client, item Item) error {
	if !e.authorize(ctx, j.Actor) {
		return Fail(Permission, "Administratorrechte des Auftraggebers entzogen")
	}
	current, err := e.Connection(ctx, c.ID)
	if err != nil {
		return err
	}
	if current.Revision != c.Revision || !current.Enabled || !current.Connected {
		return Fail(Conflict, "Verbindung geändert")
	}
	file, err := e.source.Open(ctx, j.Selection, item.SourceItem)
	if err != nil {
		_, dbErr := e.db.ExecContext(ctx, "UPDATE items SET state='failed',error=? WHERE id=?", safeError(err), item.ID)
		return dbErr
	}
	defer file.Close()
	// Recheck the exact destination before every attempt, including uncertain responses.
	entry, err := client.Stat(ctx, j.Base, item.Relative)
	if err != nil {
		return err
	}
	if entry != nil {
		state, message := "existing", ""
		if entry.Directory || entry.Size != item.Size {
			state = "conflict"
			message = "Ziel vorhanden mit anderem Typ oder anderer Größe"
		}
		_, err = e.db.ExecContext(ctx, "UPDATE items SET state=?,error=? WHERE id=?", state, message, item.ID)
		return err
	}

	if err = client.EnsureDirectories(ctx, j.Base, item.Relative[:len(item.Relative)-1]); err != nil {
		if Kind(err) == Conflict {
			_, dbErr := e.db.ExecContext(ctx, "UPDATE items SET state='conflict',error=? WHERE id=?", safeError(err), item.ID)
			return dbErr
		}
		return err
	}
	if !e.authorize(ctx, j.Actor) {
		return Fail(Permission, "Administratorrechte des Auftraggebers entzogen")
	}
	if _, err = e.db.ExecContext(ctx, "UPDATE items SET state='uploading',error='' WHERE id=?", item.ID); err != nil {
		return err
	}
	defer func() { e.progressMu.Lock(); delete(e.progress[j.ID], item.ID); e.progressMu.Unlock() }()
	err = client.CreateFile(ctx, j.Base, Upload{Resume: item.Resume, SaveResume: func(state ResumeState) error {
		raw := encode(state)
		if len(raw) > 32768 {
			return Fail(Invalid, "Uploadzustand überschreitet Speichergrenze")
		}
		_, err := e.db.ExecContext(ctx, "UPDATE items SET resume=? WHERE id=?", raw, item.ID)
		return err
	}, Segments: item.Relative, Size: item.Size, Modified: time.Unix(0, item.Modified), Body: file, Progress: func(n int64) {
		e.progressMu.Lock()
		if e.progress[j.ID] != nil {
			e.progress[j.ID][item.ID] = min(item.Size, max(0, n))
		}
		e.progressMu.Unlock()
	}})
	if err != nil {
		if Kind(err) == Conflict {
			_, dbErr := e.db.ExecContext(ctx, "UPDATE items SET state='conflict',error=? WHERE id=?", safeError(err), item.ID)
			return dbErr
		}
		return err
	}
	_, err = e.db.ExecContext(ctx, "UPDATE items SET state='done',error='' WHERE id=?", item.ID)
	return err
}
func (e *Engine) cleanup(ctx context.Context) {
	_, _ = e.db.ExecContext(ctx, "DELETE FROM jobs WHERE (submitted=0 AND state NOT IN ('planning') AND expires<?) OR (state IN ('complete','partial','cancelled') AND updated<?)", time.Now().Unix(), time.Now().Add(-30*24*time.Hour).Unix())
}
