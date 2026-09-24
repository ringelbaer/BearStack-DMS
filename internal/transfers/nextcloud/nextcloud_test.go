package nextcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"bearstack/internal/transfers"
	"bearstack/internal/transfers/contracttest"
)

type davFixture struct {
	mu        sync.Mutex
	server    *httptest.Server
	files     map[string][]byte
	dirs      map[string]bool
	chunks    map[string][]byte
	methods   []string
	failChunk bool
	raceMove  bool
	offsite   bool
}

const userRoot = "/remote.php/dav/files/user-id"

func fixture(t *testing.T) (*davFixture, *Provider, transfers.Client) {
	t.Helper()
	f := &davFixture{files: map[string][]byte{}, dirs: map[string]bool{userRoot: true}, chunks: map[string][]byte{}}
	f.server = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	p := &Provider{HTTP: f.server.Client()}
	c, err := p.Connect(context.Background(), f.config(), json.RawMessage(`{"login":"email@example.test","password":"secret-token","user_id":"user-id"}`))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, method := range f.methods {
			if method == "DELETE" {
				t.Error("DELETE was sent")
			}
		}
	})
	return f, p, c
}
func (f *davFixture) config() json.RawMessage {
	b, _ := json.Marshal(Config{Server: f.server.URL})
	return b
}
func xmlEscape(s string) string { var b bytes.Buffer; xml.EscapeText(&b, []byte(s)); return b.String() }
func (f *davFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.methods = append(f.methods, r.Method)
	pathname := strings.TrimSuffix(r.URL.Path, "/")
	if r.Method == "POST" && pathname == "/index.php/login/v2" {
		login := f.server.URL + "/login/v2/flow"
		if f.offsite {
			login = "https://elsewhere.invalid/login"
		}
		json.NewEncoder(w).Encode(map[string]any{"login": login, "poll": map[string]string{"token": "private-poll-token", "endpoint": f.server.URL + "/login/v2/poll"}})
		return
	}
	if r.Method == "POST" && pathname == "/login/v2/poll" {
		r.ParseForm()
		if r.Form.Get("token") != "private-poll-token" {
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"server": f.server.URL, "loginName": "email@example.test", "appPassword": "secret-token"})
		return
	}
	login, password, ok := r.BasicAuth()
	if !ok || login != "email@example.test" || password != "secret-token" {
		w.WriteHeader(401)
		return
	}
	if pathname == "/ocs/v2.php/cloud/user" {
		if r.URL.Query().Get("format") != "json" {
			w.WriteHeader(400)
			return
		}
		io.WriteString(w, `{"ocs":{"data":{"id":"user-id"}}}`)
		return
	}
	switch r.Method {
	case "PROPFIND":
		if _, ok := f.files[pathname]; !ok && !f.dirs[pathname] {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(207)
		io.WriteString(w, `<d:multistatus xmlns:d="DAV:">`)
		emit := func(p string, data []byte, dir bool) {
			u := url.URL{Path: p}
			kind := ""
			if dir {
				kind = "<d:collection/>"
			}
			fmt.Fprintf(w, `<d:response><d:href>%s</d:href><d:propstat><d:prop><d:getcontentlength>%d</d:getcontentlength><d:resourcetype>%s</d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`, xmlEscape(u.String()), len(data), kind)
		}
		emit(pathname, f.files[pathname], f.dirs[pathname])
		if r.Header.Get("Depth") == "1" {
			for p, data := range f.files {
				if path.Dir(p) == pathname {
					emit(p, data, false)
				}
			}
			for p := range f.dirs {
				if path.Dir(p) == pathname && p != pathname {
					emit(p, nil, true)
				}
			}
		}
		io.WriteString(w, `</d:multistatus>`)
	case "MKCOL":
		if f.dirs[pathname] {
			w.WriteHeader(405)
			return
		}
		if _, ok := f.files[pathname]; ok {
			w.WriteHeader(405)
			return
		}
		if !strings.Contains(pathname, "/dav/uploads/") && !f.dirs[path.Dir(pathname)] {
			w.WriteHeader(409)
			return
		}
		if strings.Contains(pathname, "/dav/uploads/") && (r.Header.Get("Destination") == "" || r.Header.Get("OC-Total-Length") == "") {
			w.WriteHeader(400)
			return
		}
		f.dirs[pathname] = true
		w.WriteHeader(201)
	case "PUT":
		if !f.dirs[path.Dir(pathname)] {
			w.WriteHeader(409)
			return
		}
		if strings.Contains(pathname, "/dav/uploads/") {
			if r.Header.Get("Destination") == "" || r.Header.Get("OC-Total-Length") == "" {
				w.WriteHeader(400)
				return
			}
			if f.failChunk && strings.HasSuffix(pathname, "/00002") {
				f.failChunk = false
				w.WriteHeader(503)
				return
			}
			data, err := io.ReadAll(r.Body)
			if err != nil {
				return
			}
			f.chunks[pathname] = data
			w.WriteHeader(201)
			return
		}
		if r.Header.Get("If-None-Match") != "*" {
			w.WriteHeader(400)
			return
		}
		if _, ok := f.files[pathname]; ok || f.dirs[pathname] {
			w.WriteHeader(412)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			return
		}
		f.files[pathname] = data
		w.WriteHeader(201)
	case "MOVE":
		if !strings.HasPrefix(pathname, "/remote.php/dav/uploads/user-id/bearstack-") || !strings.HasSuffix(pathname, "/.file") || r.Header.Get("Overwrite") != "F" {
			w.WriteHeader(400)
			return
		}
		dest, err := url.Parse(r.Header.Get("Destination"))
		if err != nil || dest.Host != strings.TrimPrefix(f.server.URL, "https://") || !strings.HasPrefix(dest.Path, userRoot+"/") {
			w.WriteHeader(400)
			return
		}
		if f.raceMove {
			f.files[dest.Path] = []byte("concurrent original")
		}
		if _, ok := f.files[dest.Path]; ok || f.dirs[dest.Path] {
			w.WriteHeader(412)
			return
		}
		var data []byte
		for i := 1; ; i++ {
			chunk, ok := f.chunks[path.Dir(pathname)+"/"+fmt.Sprintf("%05d", i)]
			if !ok {
				break
			}
			data = append(data, chunk...)
		}
		total, _ := strconv.ParseInt(r.Header.Get("OC-Total-Length"), 10, 64)
		if int64(len(data)) != total {
			w.WriteHeader(400)
			return
		}
		f.files[dest.Path] = data
		w.WriteHeader(201)
	default:
		w.WriteHeader(405)
	}
}
func TestNextcloudProviderContract(t *testing.T) {
	contracttest.Run(t, func(t *testing.T) contracttest.Fixture {
		f, _, c := fixture(t)
		return contracttest.Fixture{Client: c, Base: transfers.Location{ID: "/"}, Read: func(parts []string) []byte {
			f.mu.Lock()
			defer f.mu.Unlock()
			return append([]byte(nil), f.files[userRoot+"/"+strings.Join(parts, "/")]...)
		}}
	})
}
func TestLoginFlowTLSAndHostBoundary(t *testing.T) {
	f, p, _ := fixture(t)
	ctx := context.Background()
	login, err := p.BeginLogin(ctx, f.config())
	if err != nil {
		t.Fatal(err)
	}
	creds, err := p.PollLogin(ctx, f.config(), login.State)
	if err != nil || creds == nil || creds.Account != "email@example.test" || !strings.Contains(string(creds.Secret), "user-id") {
		t.Fatalf("credentials %+v %v", creds, err)
	}
	f.mu.Lock()
	f.offsite = true
	f.mu.Unlock()
	if _, err = p.BeginLogin(ctx, f.config()); transfers.Kind(err) != transfers.Permission {
		t.Fatalf("off-origin login accepted %v", err)
	}
	if _, err = New().BeginLogin(ctx, f.config()); err == nil {
		t.Fatal("accepted untrusted certificate")
	}
	for _, server := range []string{"http://host", "https://user:pass@host", "https://host/a/../b", "https://host/?password=secret"} {
		raw, _ := json.Marshal(Config{Server: server})
		if err = p.Validate(raw); err == nil {
			t.Fatalf("accepted %s", server)
		}
	}
	for _, raw := range []string{"https://another.example/path", f.server.URL + "/remote.php/../secret", f.server.URL + "/remote.php/%2e%2e/secret"} {
		base, _ := url.Parse(f.server.URL)
		if _, err = sameServer(base, raw); err == nil {
			t.Fatalf("accepted boundary %s", raw)
		}
	}
	for _, uid := range []string{"..", ".", "../other", "x/y"} {
		credentials, _ := json.Marshal(secret{Login: "a", Password: "b", UserID: uid})
		if _, err = p.Connect(ctx, f.config(), credentials); err == nil {
			t.Fatalf("accepted user id %q", uid)
		}
	}
	u, _ := url.Parse(f.server.URL + "/any")
	if _, err = p.request(ctx, "DELETE", u, nil, nil, 0, nil); transfers.Kind(err) != transfers.Unsupported {
		t.Fatalf("delete allowed: %v", err)
	}
}
func TestChunkResumeAndSafeMove(t *testing.T) {
	for _, race := range []bool{false, true} {
		t.Run(fmt.Sprint("race=", race), func(t *testing.T) {
			f, _, c := fixture(t)
			ctx := context.Background()
			base := transfers.Location{ID: "/"}
			if err := c.EnsureDirectories(ctx, base, []string{"large"}); err != nil {
				t.Fatal(err)
			}
			data := bytes.Repeat([]byte("a"), 21<<20)
			upload := contracttest.Upload([]string{"large", "movie.mp4"}, data)
			var state transfers.ResumeState
			upload.SaveResume = func(s transfers.ResumeState) error { state = s; return nil }
			f.failChunk = true
			if err := c.CreateFile(ctx, base, upload); transfers.Kind(err) != transfers.Temporary {
				t.Fatalf("missing interruption: %v", err)
			}
			var decoded chunkState
			if state.Version != 1 || json.Unmarshal(state.Data, &decoded) != nil || decoded.Offset != 10<<20 {
				t.Fatalf("resume state %#v", state)
			}
			f.mu.Lock()
			f.raceMove = race
			before := len(f.methods)
			f.mu.Unlock()
			upload.Resume = state
			err := c.CreateFile(ctx, base, upload)
			if race {
				if transfers.Kind(err) != transfers.Conflict {
					t.Fatalf("move overwrite %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			for _, m := range f.methods[before:] {
				if m == "MKCOL" {
					t.Fatal("did not resume existing chunks")
				}
			}
			got := f.files[userRoot+"/large/movie.mp4"]
			if race {
				if string(got) != "concurrent original" {
					t.Fatal("overwrote competing upload")
				}
			} else if !bytes.Equal(got, data) {
				t.Fatal("chunks assembled incorrectly")
			}
		})
	}
}
func TestResponseErrorsAndInvalidListings(t *testing.T) {
	for code, want := range map[int]transfers.ErrorKind{401: transfers.Authentication, 403: transfers.Permission, 409: transfers.Conflict, 412: transfers.Conflict, 429: transfers.Throttled, 503: transfers.Temporary, 507: transfers.Quota} {
		r := &http.Response{StatusCode: code, Header: http.Header{"Retry-After": []string{"12"}}}
		err := responseError(r)
		if transfers.Kind(err) != want || strings.Contains(err.Error(), "secret") {
			t.Fatalf("status %d: %v", code, err)
		}
		if err.(*transfers.Error).RetryAfter != 12*time.Second {
			t.Fatal("retry-after ignored")
		}
	}
	for _, response := range []string{`<d:multistatus xmlns:d="DAV:">`, `<html></html>`, `<d:multistatus xmlns:d="DAV:"><d:response><d:href>/remote.php/dav/files/other/a</d:href></d:response></d:multistatus>`} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(207); io.WriteString(w, response) }))
		p := &Provider{HTTP: server.Client()}
		config, _ := json.Marshal(Config{Server: server.URL})
		c, err := p.Connect(context.Background(), config, json.RawMessage(`{"login":"a","password":"b","user_id":"user-id"}`))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = c.Locations(context.Background(), "/"); err == nil {
			t.Fatalf("accepted %s", response)
		}
		server.Close()
	}
}
