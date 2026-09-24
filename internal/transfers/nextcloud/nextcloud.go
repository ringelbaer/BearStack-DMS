// Package nextcloud is the only component aware of DAV and Nextcloud login protocols.
package nextcloud

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"bearstack/internal/transfers"
)

type Config struct {
	Server string `json:"server"`
}
type secret struct {
	Login    string `json:"login"`
	Password string `json:"password"`
	UserID   string `json:"user_id"`
}
type loginState struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
	Expires  int64  `json:"expires"`
}
type Provider struct{ HTTP *http.Client }

func New() *Provider {
	return &Provider{HTTP: &http.Client{Timeout: 30 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (p *Provider) Info() transfers.ProviderInfo {
	return transfers.ProviderInfo{ID: "nextcloud", Name: "Nextcloud", UploadLabel: "Upload to Nextcloud", Fields: []transfers.ConfigField{{Key: "server", Label: "Serveradresse (HTTPS)", Type: "url"}}}
}
func parseConfig(raw json.RawMessage) (*url.URL, error) {
	var c Config
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&c) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, transfers.Fail(transfers.Invalid, "Ungültige Nextcloud-Konfiguration")
	}
	u, err := url.Parse(c.Server)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(u.Path, "\\\x00") || path.Clean("/"+u.Path) != "/"+strings.Trim(u.Path, "/") && strings.Trim(u.Path, "/") != "" {
		return nil, transfers.Fail(transfers.Invalid, "Eine gültige HTTPS-Serveradresse ohne Zugangsdaten ist erforderlich")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return u, nil
}
func (p *Provider) Validate(raw json.RawMessage) error { _, err := parseConfig(raw); return err }
func endpoint(base *url.URL, suffix string) *url.URL {
	u := *base
	u.Path = base.Path + suffix
	u.RawPath = ""
	return &u
}
func sameServer(base *url.URL, raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, transfers.Fail(transfers.Invalid, "Ungültige Serverantwort")
	}
	if !u.IsAbs() {
		u = base.ResolveReference(u)
	}
	if u.Scheme != base.Scheme || u.Host != base.Host || u.User != nil || u.Fragment != "" || (strings.TrimSuffix(u.Path, "/") != strings.TrimSuffix(path.Clean(u.Path), "/")) || !strings.HasPrefix(u.Path, base.Path+"/") {
		return nil, transfers.Fail(transfers.Permission, "Nextcloud verweist auf einen anderen Server")
	}
	return u, nil
}
func (p *Provider) request(ctx context.Context, method string, u *url.URL, auth *secret, body io.Reader, length int64, headers map[string]string) (*http.Response, error) {
	// Transport-level allowlist is deliberately narrower than DAV.
	switch method {
	case "GET", "HEAD", "POST", "PROPFIND", "MKCOL", "PUT", "MOVE":
	default:
		return nil, transfers.Fail(transfers.Unsupported, "Nicht erlaubte Speicheroperation")
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, transfers.Fail(transfers.Invalid, "Ungültige Anfrage")
	}
	req.ContentLength = length
	req.Header.Set("User-Agent", "BearStack storage bridge")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if auth != nil {
		req.SetBasicAuth(auth.Login, auth.Password)
	}
	client := *p.HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, transfers.Fail(transfers.Temporary, "Nextcloud ist vorübergehend nicht erreichbar")
	}
	return resp, nil
}
func responseError(resp *http.Response) error {
	kind := transfers.Temporary
	message := "Nextcloud-Anfrage fehlgeschlagen (HTTP " + strconv.Itoa(resp.StatusCode) + ")"
	switch resp.StatusCode {
	case 401:
		kind = transfers.Authentication
		message = "Nextcloud-Anmeldung erforderlich"
	case 403:
		kind = transfers.Permission
		message = "Nextcloud verweigert den Zugriff"
	case 404:
		kind = transfers.Invalid
		message = "Nextcloud-Ziel nicht gefunden"
	case 409, 412:
		kind = transfers.Conflict
		message = "Zieldatei oder Zielordner bereits belegt"
	case 507:
		kind = transfers.Quota
		message = "Nextcloud-Speicher ist voll"
	case 429:
		kind = transfers.Throttled
		message = "Nextcloud begrenzt die Anfragen"
	case 405, 413, 501:
		kind = transfers.Unsupported
		message = "Nextcloud unterstützt diese Übertragung nicht"
	}
	retry := time.Duration(0)
	if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && seconds > 0 {
		retry = time.Duration(min(seconds, 3600)) * time.Second
	} else if date, err := http.ParseTime(resp.Header.Get("Retry-After")); err == nil {
		retry = min(time.Hour, max(0, time.Until(date)))
	}
	return &transfers.Error{Kind: kind, Message: message, RetryAfter: retry}
}
func decodeJSON(resp *http.Response, out any) error {
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return responseError(resp)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return transfers.Fail(transfers.Temporary, "Ungültige Nextcloud-Antwort")
	}
	return nil
}
func (p *Provider) BeginLogin(ctx context.Context, config json.RawMessage) (transfers.Login, error) {
	base, err := parseConfig(config)
	if err != nil {
		return transfers.Login{}, err
	}
	resp, err := p.request(ctx, "POST", endpoint(base, "/index.php/login/v2"), nil, nil, 0, nil)
	if err != nil {
		return transfers.Login{}, err
	}
	var out struct {
		Login string `json:"login"`
		Poll  struct {
			Token    string `json:"token"`
			Endpoint string `json:"endpoint"`
		} `json:"poll"`
	}
	if err = decodeJSON(resp, &out); err != nil {
		return transfers.Login{}, err
	}
	loginURL, err := sameServer(base, out.Login)
	if err != nil {
		return transfers.Login{}, err
	}
	pollURL, err := sameServer(base, out.Poll.Endpoint)
	if err != nil {
		return transfers.Login{}, err
	}
	if out.Poll.Token == "" {
		return transfers.Login{}, transfers.Fail(transfers.Invalid, "Nextcloud lieferte kein Anmeldetoken")
	}
	expires := time.Now().Add(20 * time.Minute)
	state, _ := json.Marshal(loginState{pollURL.String(), out.Poll.Token, expires.Unix()})
	return transfers.Login{URL: loginURL.String(), State: state, Expires: expires}, nil
}
func (p *Provider) PollLogin(ctx context.Context, config, state json.RawMessage) (*transfers.Credentials, error) {
	base, err := parseConfig(config)
	if err != nil {
		return nil, err
	}
	var login loginState
	if json.Unmarshal(state, &login) != nil || login.Expires < time.Now().Unix() {
		return nil, transfers.Fail(transfers.Authentication, "Anmeldung abgelaufen")
	}
	poll, err := sameServer(base, login.Endpoint)
	if err != nil {
		return nil, err
	}
	body := url.Values{"token": {login.Token}}.Encode()
	resp, err := p.request(ctx, "POST", poll, nil, strings.NewReader(body), int64(len(body)), map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 404 {
		resp.Body.Close()
		return nil, nil
	}
	var out struct {
		Server   string `json:"server"`
		Login    string `json:"loginName"`
		Password string `json:"appPassword"`
	}
	if err = decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	check := strings.TrimRight(out.Server, "/") + "/"
	if _, err = sameServer(base, check); err != nil {
		return nil, err
	}
	if out.Login == "" || out.Password == "" {
		return nil, transfers.Fail(transfers.Authentication, "Unvollständige Nextcloud-Anmeldung")
	}
	auth := secret{Login: out.Login, Password: out.Password}
	userURL := endpoint(base, "/ocs/v2.php/cloud/user")
	userURL.RawQuery = "format=json"
	resp, err = p.request(ctx, "GET", userURL, &auth, nil, 0, map[string]string{"OCS-APIRequest": "true", "Accept": "application/json"})
	// The query is set separately; endpoint paths are always segment-escaped.
	if err != nil {
		return nil, err
	}
	var user struct {
		OCS struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		} `json:"ocs"`
	}
	if err = decodeJSON(resp, &user); err != nil {
		return nil, err
	}
	if transfers.ValidateSegments([]string{user.OCS.Data.ID}) != nil {
		return nil, transfers.Fail(transfers.Authentication, "Nextcloud-Benutzerkennung fehlt")
	}
	auth.UserID = user.OCS.Data.ID
	raw, _ := json.Marshal(auth)
	return &transfers.Credentials{Account: auth.Login, Secret: raw}, nil
}

type client struct {
	provider    *Provider
	base        *url.URL
	auth        secret
	dirMu       sync.Mutex
	directories map[string]bool
}

func (p *Provider) Connect(ctx context.Context, config, credentials json.RawMessage) (transfers.Client, error) {
	base, err := parseConfig(config)
	if err != nil {
		return nil, err
	}
	var auth secret
	if json.Unmarshal(credentials, &auth) != nil || auth.Login == "" || auth.Password == "" || transfers.ValidateSegments([]string{auth.UserID}) != nil {
		return nil, transfers.Fail(transfers.Authentication, "Bitte Nextcloud erneut verbinden")
	}
	return &client{provider: p, base: base, auth: auth, directories: map[string]bool{}}, nil
}
func locationSegments(id string) ([]string, error) {
	if id == "/" {
		return []string{}, nil
	}
	if !strings.HasPrefix(id, "/") {
		return nil, transfers.Fail(transfers.Invalid, "Ungültiger Basisordner")
	}
	parts := strings.Split(strings.TrimPrefix(id, "/"), "/")
	if err := transfers.ValidateSegments(parts); err != nil {
		return nil, err
	}
	return parts, nil
}
func (c *client) dav(parts []string) *url.URL {
	return endpoint(c.base, "/remote.php/dav/files/"+c.auth.UserID+"/"+strings.Join(parts, "/"))
}
func (c *client) target(base transfers.Location, segments []string) (*url.URL, error) {
	root, err := locationSegments(base.ID)
	if err != nil {
		return nil, err
	}
	if err = transfers.ValidateSegments(segments); err != nil {
		return nil, err
	}
	return c.dav(append(root, segments...)), nil
}

type davResponse struct {
	Href  string `xml:"href"`
	Props []struct {
		Status string `xml:"status"`
		Prop   struct {
			Size int64 `xml:"getcontentlength"`
			Type struct {
				Collection *struct{} `xml:"collection"`
			} `xml:"resourcetype"`
		} `xml:"prop"`
	} `xml:"propstat"`
}
type remoteEntry struct {
	Path      string
	Size      int64
	Directory bool
}

func (c *client) list(ctx context.Context, parts []string, depth string, emit func(remoteEntry) error) error {
	body := `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:getcontentlength/><d:resourcetype/></d:prop></d:propfind>`
	resp, err := c.provider.request(ctx, "PROPFIND", c.dav(parts), &c.auth, strings.NewReader(body), int64(len(body)), map[string]string{"Depth": depth, "Content-Type": "application/xml"})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 207 {
		return responseError(resp)
	}
	limited := &io.LimitedReader{R: resp.Body, N: 128 << 20}
	decoder := xml.NewDecoder(limited)
	complete := false
	root := c.dav(nil).Path
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			if !complete || limited.N == 0 {
				return transfers.Fail(transfers.Temporary, "Unvollständige Nextcloud-Verzeichnisantwort")
			}
			return nil
		}
		if end, ok := token.(xml.EndElement); ok && end.Name.Local == "multistatus" {
			complete = true
		}
		if err != nil {
			return transfers.Fail(transfers.Temporary, "Ungültige Nextcloud-Verzeichnisantwort")
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "response" {
			continue
		}
		var row davResponse
		if err = decoder.DecodeElement(&row, &start); err != nil {
			return transfers.Fail(transfers.Temporary, "Ungültige Nextcloud-Verzeichnisantwort")
		}
		u, err := sameServer(c.base, row.Href)
		if err != nil {
			return err
		}
		if u.Path != strings.TrimSuffix(root, "/") && !strings.HasPrefix(u.Path, root) {
			return transfers.Fail(transfers.Permission, "Verzeichnisantwort außerhalb des Kontos")
		}
		for _, prop := range row.Props {
			if !strings.Contains(prop.Status, " 200 ") {
				continue
			}
			if err = emit(remoteEntry{Path: strings.TrimPrefix(strings.TrimPrefix(strings.TrimSuffix(u.Path, "/"), strings.TrimSuffix(root, "/")), "/"), Size: prop.Prop.Size, Directory: prop.Prop.Type.Collection != nil}); err != nil {
				return err
			}
			break
		}
	}
}
func (c *client) Locations(ctx context.Context, parent string) ([]transfers.Location, error) {
	if parent == "" {
		parent = "/"
	}
	parts, err := locationSegments(parent)
	if err != nil {
		return nil, err
	}
	out := []transfers.Location{}
	found := false
	prefix := strings.Join(parts, "/")
	err = c.list(ctx, parts, "1", func(row remoteEntry) error {
		if row.Path == prefix {
			found = row.Directory
		}
		if row.Directory {
			out = append(out, transfers.Location{ID: "/" + row.Path, Name: path.Base(row.Path)})
		}
		return nil
	})
	if err == nil && !found {
		return nil, transfers.Fail(transfers.Conflict, "Basisziel ist kein Ordner")
	}
	for i := range out {
		if out[i].ID == "/" {
			out[i].Name = "Dateien"
		}
	}
	return out, err
}
func (c *client) Inventory(ctx context.Context, base transfers.Location, directory []string, emit func(transfers.Entry) error) error {
	root, err := locationSegments(base.ID)
	if err != nil {
		return err
	}
	if len(directory) > 0 {
		if err = transfers.ValidateSegments(directory); err != nil {
			return err
		}
	}
	full := append(append([]string{}, root...), directory...)
	parent := strings.Join(full, "/")
	err = c.list(ctx, full, "1", func(row remoteEntry) error {
		if row.Path == parent {
			if !row.Directory {
				return transfers.Fail(transfers.Conflict, "Zielpfad enthält eine Datei statt eines Ordners")
			}
			return nil
		}
		parentPath := path.Dir(row.Path)
		if parentPath == "." {
			parentPath = ""
		}
		if parentPath == parent {
			return emit(transfers.Entry{Segments: append(append([]string{}, directory...), path.Base(row.Path)), Size: row.Size, Directory: row.Directory})
		}
		return nil
	})
	if Kind404(err) {
		return nil
	}
	return err
}
func Kind404(err error) bool {
	return transfers.Kind(err) == transfers.Invalid && err.Error() == "Nextcloud-Ziel nicht gefunden"
}
func (c *client) EnsureDirectories(ctx context.Context, base transfers.Location, parts []string) error {
	root, err := locationSegments(base.ID)
	if err != nil {
		return err
	}
	if len(parts) == 0 {
		return nil
	}
	if err = transfers.ValidateSegments(parts); err != nil {
		return err
	}
	c.dirMu.Lock()
	defer c.dirMu.Unlock()
	for i := range parts {
		u := c.dav(append(append([]string{}, root...), parts[:i+1]...))
		if c.directories[u.Path] {
			continue
		}
		resp, err := c.provider.request(ctx, "MKCOL", u, &c.auth, nil, 0, nil)
		if err != nil {
			return err
		}
		code := resp.StatusCode
		resp.Body.Close()
		if code == 201 {
			c.rememberDirectory(u.Path)
			continue
		}
		if code != 405 && code != 409 {
			return responseError(resp)
		}
		found := false
		err = c.list(ctx, append(append([]string{}, root...), parts[:i+1]...), "0", func(row remoteEntry) error { found = row.Directory; return nil })
		if err != nil {
			return err
		}
		if !found {
			return transfers.Fail(transfers.Conflict, "Zielpfad ist kein Ordner")
		}
		c.rememberDirectory(u.Path)
	}
	return nil
}

type progressReader struct {
	reader io.Reader
	done   int64
	offset int64
	report func(int64)
}

func (r *progressReader) Read(b []byte) (int, error) {
	n, err := r.reader.Read(b)
	r.done += int64(n)
	if r.report != nil {
		r.report(r.offset + r.done)
	}
	return n, err
}
func (c *client) CreateFile(ctx context.Context, base transfers.Location, upload transfers.Upload) error {
	target, err := c.target(base, upload.Segments)
	if err != nil {
		return err
	}
	if upload.Size < 0 {
		return transfers.Fail(transfers.Invalid, "Ungültige Dateigröße")
	}
	if upload.Size < 20<<20 {
		reader := &progressReader{reader: io.NewSectionReader(upload.Body, 0, upload.Size), report: upload.Progress}
		resp, err := c.provider.request(ctx, "PUT", target, &c.auth, reader, upload.Size, map[string]string{"If-None-Match": "*", "Content-Type": "application/octet-stream", "X-OC-Mtime": strconv.FormatInt(upload.Modified.Unix(), 10)})
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 201 {
			return responseError(resp)
		}
		return nil
	}

	chunkSize := max(int64(10<<20), (upload.Size+9999)/10000)
	if chunkSize > 5<<30 {
		return transfers.Fail(transfers.Unsupported, "Datei überschreitet Nextcloud-Uploadgrenzen")
	}
	// Only adapter-created upload IDs are ever used as MOVE sources. Persisted
	// state is bound to this exact destination and expires before Nextcloud's TTL.
	var state chunkState
	if upload.Resume.Version == 1 {
		if json.Unmarshal(upload.Resume.Data, &state) != nil {
			state = chunkState{}
		}
	}
	token, tokenErr := hex.DecodeString(state.Token)
	reusable := tokenErr == nil && len(token) == 16 && state.Target == target.String() && state.Size == upload.Size && state.Offset >= 0 && state.Offset <= upload.Size && (state.Offset%chunkSize == 0 || state.Offset == upload.Size) && time.Since(time.Unix(state.Updated, 0)) < 23*time.Hour
	uploadURL := func() *url.URL {
		return endpoint(c.base, "/remote.php/dav/uploads/"+c.auth.UserID+"/bearstack-"+state.Token)
	}
	if reusable {
		resp, err := c.provider.request(ctx, "PROPFIND", uploadURL(), &c.auth, nil, 0, map[string]string{"Depth": "0"})
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode == 404 {
			reusable = false
		} else if resp.StatusCode != 207 {
			return responseError(resp)
		}
	}
	headers := map[string]string{"Destination": target.String(), "OC-Total-Length": strconv.FormatInt(upload.Size, 10)}
	save := func() error {
		state.Updated = time.Now().Unix()
		if upload.SaveResume == nil {
			return nil
		}
		raw, _ := json.Marshal(state)
		return upload.SaveResume(transfers.ResumeState{Version: 1, Data: raw})
	}
	if !reusable {
		nonce := make([]byte, 16)
		if _, err = rand.Read(nonce); err != nil {
			return err
		}
		state = chunkState{Token: hex.EncodeToString(nonce), Target: target.String(), Size: upload.Size}
		resp, err := c.provider.request(ctx, "MKCOL", uploadURL(), &c.auth, nil, 0, headers)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode != 201 {
			return responseError(resp)
		}
		if err = save(); err != nil {
			return err
		}
	}
	uploadRoot := uploadURL()
	var resp *http.Response
	for offset, index := state.Offset, int(state.Offset/chunkSize)+1; offset < upload.Size; offset, index = offset+chunkSize, index+1 {
		size := min(chunkSize, upload.Size-offset)
		u := *uploadRoot
		u.Path += "/" + fmt.Sprintf("%05d", index)
		reader := &progressReader{reader: io.NewSectionReader(upload.Body, offset, size), offset: offset, report: upload.Progress}
		resp, err = c.provider.request(ctx, "PUT", &u, &c.auth, reader, size, headers)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode != 201 && resp.StatusCode != 204 {
			return responseError(resp)
		}
		state.Offset = offset + size
		if err = save(); err != nil {
			return err
		}
	}
	u := *uploadRoot
	u.Path += "/.file"
	headers["Overwrite"] = "F"
	headers["X-OC-Mtime"] = strconv.FormatInt(upload.Modified.Unix(), 10)
	resp, err = c.provider.request(ctx, "MOVE", &u, &c.auth, nil, 0, headers)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		return responseError(resp)
	}
	return nil
}

func (c *client) Stat(ctx context.Context, base transfers.Location, segments []string) (*transfers.Entry, error) {
	root, err := locationSegments(base.ID)
	if err != nil {
		return nil, err
	}
	if err = transfers.ValidateSegments(segments); err != nil {
		return nil, err
	}
	full := append(root, segments...)
	var found *transfers.Entry
	err = c.list(ctx, full, "0", func(row remoteEntry) error {
		if row.Path == strings.Join(full, "/") {
			found = &transfers.Entry{Segments: segments, Size: row.Size, Directory: row.Directory}
		}
		return nil
	})
	if Kind404(err) {
		return nil, nil
	}
	return found, err
}

// Version 1 is private to this adapter; it contains no account credentials.
type chunkState struct {
	Token   string `json:"token"`
	Target  string `json:"target"`
	Size    int64  `json:"size"`
	Offset  int64  `json:"offset"`
	Updated int64  `json:"updated"`
}

// Cache verified directories only within this job and bound its memory footprint.
// Caller holds dirMu. A later job always obtains a fresh client.
func (c *client) rememberDirectory(key string) {
	if len(c.directories) >= 4096 {
		c.directories = map[string]bool{}
	}
	c.directories[key] = true
}
