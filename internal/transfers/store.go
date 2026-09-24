package transfers

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

type Engine struct {
	db         *sql.DB
	cipher     cipher.AEAD
	providers  Registry
	source     Source
	authorize  func(context.Context, string) bool
	mu         sync.Mutex
	progressMu sync.Mutex
	cancels    map[string]context.CancelFunc
	progress   map[string]map[int64]int64
	wake       chan struct{}
}

func ID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func Open(directory string, providers Registry, source Source, authorize func(context.Context, string) bool) (*Engine, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(directory, "credentials.key")
	key, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		// Never replace a missing key belonging to an existing store.
		if info, e := os.Stat(filepath.Join(directory, "transfers.db")); e == nil && info.Size() > 0 {
			return nil, errors.New("transfer credentials key is missing")
		}
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		var f *os.File
		f, err = os.OpenFile(keyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			_, err = f.Write(key)
			err = errors.Join(err, f.Sync(), f.Close())
		}
	}
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("invalid transfer key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(directory, "transfers.db")
	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	f.Close()
	u := url.URL{Scheme: "file", Path: dbPath}
	q := u.Query()
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		db.Close()
		return nil, err
	}
	if version > 1 {
		db.Close()
		return nil, errors.New("transfer database is newer than this application")
	}
	_, err = db.Exec(`PRAGMA journal_mode=WAL;
 CREATE TABLE IF NOT EXISTS connections(id TEXT PRIMARY KEY, data TEXT NOT NULL, secret BLOB);
 CREATE TABLE IF NOT EXISTS jobs(id TEXT PRIMARY KEY, connection_id TEXT NOT NULL REFERENCES connections(id), revision INTEGER NOT NULL, actor TEXT NOT NULL, selection TEXT NOT NULL, source_name TEXT NOT NULL, target TEXT NOT NULL, base TEXT NOT NULL, state TEXT NOT NULL, error TEXT NOT NULL DEFAULT '', created INTEGER NOT NULL, expires INTEGER NOT NULL, updated INTEGER NOT NULL, attempts INTEGER NOT NULL DEFAULT 0, retry_at INTEGER NOT NULL DEFAULT 0, submitted INTEGER NOT NULL DEFAULT 0, queue_seq INTEGER NOT NULL DEFAULT 0, target_exists INTEGER NOT NULL DEFAULT 0);
 CREATE INDEX IF NOT EXISTS jobs_pending ON jobs(state,retry_at,created);
 CREATE TABLE IF NOT EXISTS items(id INTEGER PRIMARY KEY, job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE, path TEXT NOT NULL, display_path TEXT NOT NULL, relative TEXT NOT NULL, directory TEXT NOT NULL, size INTEGER NOT NULL, modified INTEGER NOT NULL, state TEXT NOT NULL DEFAULT 'missing', error TEXT NOT NULL DEFAULT '', resume TEXT NOT NULL DEFAULT '{}', UNIQUE(job_id,path), UNIQUE(job_id,relative));
 CREATE INDEX IF NOT EXISTS items_directory ON items(job_id,directory);
 CREATE INDEX IF NOT EXISTS items_pending ON items(job_id,state,id);
 CREATE TABLE IF NOT EXISTS directories(job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,relative TEXT NOT NULL,blocked INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(job_id,relative));
 PRAGMA user_version=1;`)
	if err != nil {
		db.Close()
		return nil, err
	}
	e := &Engine{db: db, cipher: aead, providers: providers, source: source, authorize: authorize, cancels: map[string]context.CancelFunc{}, progress: map[string]map[int64]int64{}, wake: make(chan struct{}, 1)}
	// A crash must never publish an incomplete preview or skip an uncertain upload.
	_, err = db.Exec(`UPDATE jobs SET state='queued' WHERE state='running'; UPDATE items SET state='missing' WHERE state='uploading'; UPDATE jobs SET state='preparing' WHERE state='planning';`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return e, nil
}
func (e *Engine) Close() error { return e.db.Close() }
func (e *Engine) signal() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}
func (e *Engine) Providers() []ProviderInfo {
	out := []ProviderInfo{}
	for _, p := range e.providers {
		out = append(out, p.Info())
	}
	return out
}
func (e *Engine) Connections(ctx context.Context) ([]Connection, error) {
	rows, err := e.db.QueryContext(ctx, "SELECT data FROM connections ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Connection{}
	for rows.Next() {
		var raw string
		var c Connection
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (e *Engine) connection(ctx context.Context, id string) (Connection, []byte, error) {
	var c Connection
	var raw string
	var secret []byte
	err := e.db.QueryRowContext(ctx, "SELECT data,secret FROM connections WHERE id=?", id).Scan(&raw, &secret)
	if err != nil {
		return c, nil, err
	}
	err = json.Unmarshal([]byte(raw), &c)
	return c, secret, err
}
func (e *Engine) Connection(ctx context.Context, id string) (Connection, error) {
	c, _, err := e.connection(ctx, id)
	return c, err
}
func (e *Engine) seal(id string, secret []byte) ([]byte, error) {
	nonce := make([]byte, e.cipher.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return e.cipher.Seal(nonce, nonce, secret, []byte(id)), nil
}
func (e *Engine) client(ctx context.Context, c Connection) (Client, error) {
	stored, secret, err := e.connection(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	if stored.Revision != c.Revision || !stored.Enabled || !stored.Connected {
		return nil, Fail(Conflict, "Verbindung geändert oder deaktiviert; Ziel erneut prüfen")
	}
	n := e.cipher.NonceSize()
	if len(secret) < n {
		return nil, Fail(Authentication, "Bitte verbinden")
	}
	plain, err := e.cipher.Open(nil, secret[:n], secret[n:], []byte(c.ID))
	if err != nil {
		return nil, Fail(Authentication, "Zugangsdaten nicht lesbar")
	}
	p := e.providers[c.Provider]
	if p == nil {
		return nil, Fail(Unsupported, "Speicheranbieter nicht verfügbar")
	}
	return p.Connect(ctx, c.Config, plain)
}
func (e *Engine) SaveConnection(ctx context.Context, c Connection) (Connection, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p := e.providers[c.Provider]
	if p == nil {
		return c, Fail(Invalid, "Unbekannter Speicheranbieter")
	}
	if len(c.Name) == 0 || utf8.RuneCountInString(c.Name) > 120 {
		return c, Fail(Invalid, "Name erforderlich (maximal 120 Zeichen)")
	}
	if err := p.Validate(c.Config); err != nil {
		return c, err
	}
	var secret []byte
	pause := false
	if c.ID == "" {
		c.ID = ID()
		c.Revision = 1
		c.Connected = false
		c.Account = ""
		c.Base = Location{}
	} else {
		old, stored, err := e.connection(ctx, c.ID)
		if err != nil {
			return c, err
		}
		if old.Revision != c.Revision {
			return c, Fail(Conflict, "Einstellungen wurden inzwischen geändert")
		}
		if old.Provider != c.Provider {
			return c, Fail(Invalid, "Anbieter einer bestehenden Verbindung kann nicht geändert werden")
		}
		pause = old.Enabled && !c.Enabled
		if string(old.Config) != string(c.Config) || old.Base != c.Base {
			c.Revision++
			pause = true
		}
		secret = stored
		c.Connected = old.Connected
		c.Account = old.Account
		if string(old.Config) != string(c.Config) {
			c.Connected = false
			c.Account = ""
			secret = nil
			c.Base = Location{}
		}
	}
	raw, _ := json.Marshal(c)
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return c, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT INTO connections(id,data,secret) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data,secret=excluded.secret", c.ID, string(raw), secret); err != nil {
		return c, err
	}
	if pause {
		message := "Verbindung geändert; neue Vorschau erforderlich"
		if !c.Enabled {
			message = "Verbindung deaktiviert"
		}
		if _, err = tx.ExecContext(ctx, "UPDATE jobs SET state='paused',error=?,updated=? WHERE connection_id=? AND state IN ('preparing','planning','ready','queued','running','waiting')", message, time.Now().Unix(), c.ID); err != nil {
			return c, err
		}
	}
	if err = tx.Commit(); err != nil {
		return c, err
	}
	if pause {
		e.cancelConnection(ctx, c.ID)
	}
	e.signal()
	return c, nil
}

// Caller holds mu; cancellation never performs remote cleanup.
func (e *Engine) cancelConnection(ctx context.Context, id string) {
	rows, err := e.db.QueryContext(ctx, "SELECT id FROM jobs WHERE connection_id=?", id)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var job string
		if rows.Scan(&job) == nil {
			if cancel := e.cancels[job]; cancel != nil {
				cancel()
			}
		}
	}
}
func (e *Engine) Disconnect(ctx context.Context, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	c, _, err := e.connection(ctx, id)
	if err != nil {
		return err
	}
	c.Connected = false
	c.Enabled = false
	c.Account = ""
	c.Revision++
	raw, _ := json.Marshal(c)
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE connections SET data=?,secret=NULL WHERE id=?", string(raw), id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE jobs SET state='paused',error='Verbindung getrennt',updated=? WHERE connection_id=? AND state IN ('preparing','planning','ready','queued','running','waiting')", time.Now().Unix(), id); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	e.cancelConnection(ctx, id)
	return nil
}
func (e *Engine) BeginLogin(ctx context.Context, id string) (Login, int64, error) {
	c, err := e.Connection(ctx, id)
	if err != nil {
		return Login{}, 0, err
	}
	p := e.providers[c.Provider]
	if p == nil {
		return Login{}, 0, Fail(Unsupported, "Anbieter fehlt")
	}
	login, err := p.BeginLogin(ctx, c.Config)
	return login, c.Revision, err
}
func (e *Engine) PollLogin(ctx context.Context, id string, revision int64, state json.RawMessage) (bool, error) {
	c, err := e.Connection(ctx, id)
	if err != nil {
		return false, err
	}
	if c.Revision != revision {
		return false, Fail(Conflict, "Verbindung wurde geändert")
	}
	creds, err := e.providers[c.Provider].PollLogin(ctx, c.Config, state)
	if err != nil || creds == nil {
		return false, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	c, err = e.Connection(ctx, id)
	if err != nil {
		return false, err
	}
	if c.Revision != revision {
		return false, Fail(Conflict, "Verbindung wurde geändert")
	}
	secret, err := e.seal(id, creds.Secret)
	if err != nil {
		return false, err
	}
	c.Connected = true
	c.Account = creds.Account
	c.Base = Location{}
	c.Revision++
	raw, _ := json.Marshal(c)
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE connections SET data=?,secret=? WHERE id=?", string(raw), secret, id); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE jobs SET state='paused',error='Konto geändert; neue Vorschau erforderlich',updated=? WHERE connection_id=? AND state IN ('preparing','planning','ready','queued','running','waiting')", time.Now().Unix(), id); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	e.cancelConnection(ctx, id)
	return true, nil
}
func (e *Engine) Locations(ctx context.Context, id, parent string) ([]Location, error) {
	c, err := e.Connection(ctx, id)
	if err != nil {
		return nil, err
	}
	client, err := e.client(ctx, c)
	if err != nil {
		return nil, err
	}
	return client.Locations(ctx, parent)
}
func (e *Engine) SetBase(ctx context.Context, id string, revision int64, base Location) (Connection, error) {
	c, err := e.Connection(ctx, id)
	if err != nil {
		return c, err
	}
	if c.Revision != revision {
		return c, Fail(Conflict, "Verbindung wurde geändert")
	}
	client, err := e.client(ctx, c)
	if err != nil {
		return c, err
	}
	locations, err := client.Locations(ctx, base.ID)
	if err != nil {
		return c, err
	}
	found := false
	for _, location := range locations {
		if location.ID == base.ID && location.ID != "" {
			base = location
			found = true
			break
		}
	}
	if !found {
		return c, Fail(Invalid, "Basisziel nicht gefunden")
	}
	c.Base = base
	return e.SaveConnection(ctx, c)
}
func encode(v any) string { b, _ := json.Marshal(v); return string(b) }
