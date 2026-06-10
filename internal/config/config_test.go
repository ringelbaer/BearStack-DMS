package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clearBearStackEnv(t *testing.T) {
	for _, key := range []string{
		"BEARSTACK_CONFIG",
		"BEARSTACK_ADDR",
		"BEARSTACK_DATA_DIR",
		"BEARSTACK_STORAGE_DIR",
		"BEARSTACK_DB_PATH",
		"BEARSTACK_MAX_UPLOAD_MB",
		"BEARSTACK_MAX_UPLOAD_BYTES",
		"BEARSTACK_AUTH_USER",
		"BEARSTACK_AUTH_PASSWORD",
		"BEARSTACK_AUTH_PASSWORD_HASH",
		"BEARSTACK_AUTH_REALM",
		"BEARSTACK_TLS_ENABLED",
		"BEARSTACK_TLS_CERT_FILE",
		"BEARSTACK_TLS_KEY_FILE",
		"BEARSTACK_TLS_AUTO_CERT",
		"BEARSTACK_PHOTOS_ENABLED",
		"BEARSTACK_PHOTOS_DIR",
		"BEARSTACK_PHOTOS_DATA_DIR",
		"BEARSTACK_PHOTOS_CACHE_DIR",
		"BEARSTACK_PHOTOS_DB_PATH",
		"BEARSTACK_PHOTOS_PAGE_SIZE",
		"BEARSTACK_WEBDAV_PATH",
		"BEARSTACK_ENV_FILE",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadReadsWebDAVPathFromConfigFile(t *testing.T) {
	clearBearStackEnv(t)
	path := filepath.Join(t.TempDir(), "bearstack.json")
	if err := os.WriteFile(path, []byte(`{
		"webdav": {
			"path": "/dav/"
		}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BEARSTACK_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.WebDAV.Path != "/dav" {
		t.Fatalf("webdav path = %q", cfg.WebDAV.Path)
	}
}

func TestLoadReadsWebDAVPathFromEnv(t *testing.T) {
	clearBearStackEnv(t)
	path := filepath.Join(t.TempDir(), "bearstack.json")
	if err := os.WriteFile(path, []byte(`{
		"webdav": {
			"path": "/from-config"
		}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BEARSTACK_CONFIG", path)
	t.Setenv("BEARSTACK_WEBDAV_PATH", "/from-env")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.WebDAV.Path != "/from-env" {
		t.Fatalf("webdav path = %q", cfg.WebDAV.Path)
	}
}

func TestLoadRejectsInvalidWebDAVPath(t *testing.T) {
	clearBearStackEnv(t)
	t.Setenv("BEARSTACK_WEBDAV_PATH", "/")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "webdav path") {
		t.Fatalf("Load() err = %v", err)
	}
}

func TestLoadReadsAuthPasswordHashFromEnv(t *testing.T) {
	const passwordHash = "$2a$04$012345678901234567890e6z6A1gXzYwmxRswbXQiwI5ctv8Hw3kW"

	clearBearStackEnv(t)
	t.Setenv("BEARSTACK_AUTH_USER", "admin")
	t.Setenv("BEARSTACK_AUTH_PASSWORD_HASH", passwordHash)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Auth.Username != "admin" {
		t.Fatalf("username = %q", cfg.Auth.Username)
	}
	if cfg.Auth.PasswordHash != passwordHash {
		t.Fatalf("password hash = %q", cfg.Auth.PasswordHash)
	}
	if cfg.Auth.Password != "" {
		t.Fatalf("password = %q", cfg.Auth.Password)
	}
	if !cfg.Auth.Enabled() {
		t.Fatal("auth should be enabled with username and password hash")
	}
}

func TestLoadReadsAuthPasswordHashFromConfigFile(t *testing.T) {
	clearBearStackEnv(t)
	path := filepath.Join(t.TempDir(), "bearstack.json")
	if err := os.WriteFile(path, []byte(`{
		"auth": {
			"username": "admin",
			"password_hash": "$2a$04$012345678901234567890e6z6A1gXzYwmxRswbXQiwI5ctv8Hw3kW"
		}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BEARSTACK_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Auth.Username != "admin" {
		t.Fatalf("username = %q", cfg.Auth.Username)
	}
	if cfg.Auth.PasswordHash == "" {
		t.Fatal("password hash should be set")
	}
	if !cfg.Auth.Enabled() {
		t.Fatal("auth should be enabled with config password hash")
	}
}

func TestLoadReadsAuthCredentialsFromConfigFile(t *testing.T) {
	clearBearStackEnv(t)
	path := filepath.Join(t.TempDir(), "bearstack.json")
	if err := os.WriteFile(path, []byte(`{
		"auth": {
			"username": "legacy",
			"password": "ignored",
			"credentials": [
				{"username": "dav", "password_hash": "$2a$04$012345678901234567890e6z6A1gXzYwmxRswbXQiwI5ctv8Hw3kW", "role": "documents_read"},
				{"username": "photos", "password": "secret", "role": "photos_read"}
			]
		}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BEARSTACK_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Auth.Credentials) != 2 {
		t.Fatalf("credentials = %#v", cfg.Auth.Credentials)
	}
	if !cfg.Auth.Enabled() {
		t.Fatal("auth should be enabled with credential list")
	}
	if cfg.Auth.Credentials[0].Username != "dav" || cfg.Auth.Credentials[0].Role != "documents_read" {
		t.Fatalf("first credential = %#v", cfg.Auth.Credentials[0])
	}
}

func TestLoadReadsTLSFromEnv(t *testing.T) {
	clearBearStackEnv(t)
	t.Setenv("BEARSTACK_TLS_ENABLED", "true")
	t.Setenv("BEARSTACK_TLS_CERT_FILE", "/tmp/bearstack.crt")
	t.Setenv("BEARSTACK_TLS_KEY_FILE", "/tmp/bearstack.key")
	t.Setenv("BEARSTACK_TLS_AUTO_CERT", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.TLS.Enabled {
		t.Fatal("tls should be enabled")
	}
	if cfg.TLS.CertFile != "/tmp/bearstack.crt" || cfg.TLS.KeyFile != "/tmp/bearstack.key" {
		t.Fatalf("tls files = %q %q", cfg.TLS.CertFile, cfg.TLS.KeyFile)
	}
	if cfg.TLS.AutoCert {
		t.Fatal("tls auto cert should be disabled")
	}
}

func TestLoadDefaultsTLSAutoCertWhenEnabledInConfig(t *testing.T) {
	clearBearStackEnv(t)
	path := filepath.Join(t.TempDir(), "bearstack.json")
	if err := os.WriteFile(path, []byte(`{
		"tls": {
			"enabled": true
		}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BEARSTACK_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.TLS.Enabled {
		t.Fatal("tls should be enabled")
	}
	if !cfg.TLS.AutoCert {
		t.Fatal("auto cert should default to true")
	}
}

func TestLoadRejectsIncompleteTLSFiles(t *testing.T) {
	clearBearStackEnv(t)
	path := filepath.Join(t.TempDir(), "bearstack.json")
	if err := os.WriteFile(path, []byte(`{
		"tls": {
			"enabled": true,
			"cert_file": "/tmp/bearstack.crt",
			"auto_cert": false
		}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BEARSTACK_CONFIG", path)

	if _, err := Load(); err == nil {
		t.Fatal("expected incomplete tls files to be rejected")
	}
}

func TestLoadIgnoresUnknownConfigFields(t *testing.T) {
	clearBearStackEnv(t)
	path := filepath.Join(t.TempDir(), "bearstack.json")
	if err := os.WriteFile(path, []byte(`{
		"addr": "127.0.0.1:8080",
		"surprise": true,
		"auth": {
			"username": "admin",
			"password": "secret",
			"legacy_token": "ignored"
		}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BEARSTACK_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:8080" {
		t.Fatalf("addr = %q", cfg.Addr)
	}
	if !cfg.Auth.Enabled() {
		t.Fatal("auth should still be loaded")
	}
}

func TestLoadRejectsTrailingConfigJSON(t *testing.T) {
	clearBearStackEnv(t)
	path := filepath.Join(t.TempDir(), "bearstack.json")
	data := []byte(`{"addr":"127.0.0.1:8080"}{"auth":{"username":"admin","password":"secret"}}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BEARSTACK_CONFIG", path)

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "multiple JSON documents") {
		t.Fatalf("Load() err = %v", err)
	}
}

func TestLoadRejectsNonPositiveMaxUploadBytes(t *testing.T) {
	clearBearStackEnv(t)
	t.Setenv("BEARSTACK_MAX_UPLOAD_BYTES", "0")

	if _, err := Load(); err == nil {
		t.Fatal("expected non-positive upload limit to be rejected")
	}
}

func TestLoadRejectsPublicBindWithoutAuth(t *testing.T) {
	clearBearStackEnv(t)
	t.Setenv("BEARSTACK_ADDR", "0.0.0.0:8080")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "auth username") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadAllowsLoopbackBindWithoutAuth(t *testing.T) {
	clearBearStackEnv(t)
	t.Setenv("BEARSTACK_ADDR", "localhost:8080")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.Enabled() {
		t.Fatal("auth should not be required for loopback bind")
	}
}

func TestLoadAllowsPublicBindWithAuth(t *testing.T) {
	clearBearStackEnv(t)
	t.Setenv("BEARSTACK_ADDR", ":8080")
	t.Setenv("BEARSTACK_AUTH_USER", "admin")
	t.Setenv("BEARSTACK_AUTH_PASSWORD", "secret")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Auth.Enabled() {
		t.Fatal("auth should be enabled")
	}
}

func TestLoadAllowsPublicBindWithAuthCredentials(t *testing.T) {
	clearBearStackEnv(t)
	path := filepath.Join(t.TempDir(), "bearstack.json")
	if err := os.WriteFile(path, []byte(`{
		"addr": ":8080",
		"auth": {
			"credentials": [
				{"username": "admin", "password": "secret", "role": "admin"}
			]
		}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BEARSTACK_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Auth.Enabled() {
		t.Fatal("auth should be enabled")
	}
}

func TestLoadReadsPhotoModuleFromEnv(t *testing.T) {
	clearBearStackEnv(t)
	t.Setenv("BEARSTACK_PHOTOS_ENABLED", "true")
	t.Setenv("BEARSTACK_PHOTOS_DIR", "/srv/photos")
	t.Setenv("BEARSTACK_PHOTOS_DATA_DIR", "/var/lib/bearstack/photos")
	t.Setenv("BEARSTACK_PHOTOS_PAGE_SIZE", "42")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.Photos.Enabled || !cfg.Photos.Active() {
		t.Fatal("photos should be enabled")
	}
	if cfg.Photos.RootDir != "/srv/photos" {
		t.Fatalf("photo root = %q", cfg.Photos.RootDir)
	}
	if cfg.Photos.DataDir != "/var/lib/bearstack/photos" {
		t.Fatalf("photo data = %q", cfg.Photos.DataDir)
	}
	if cfg.Photos.CacheDir != "/var/lib/bearstack/photos/thumbnails" {
		t.Fatalf("photo cache = %q", cfg.Photos.CacheDir)
	}
	if cfg.Photos.DBPath != "/var/lib/bearstack/photos/photos.db" {
		t.Fatalf("photo db = %q", cfg.Photos.DBPath)
	}
	if cfg.Photos.PageSize != 42 {
		t.Fatalf("photo page size = %d", cfg.Photos.PageSize)
	}
}

func TestLoadAllowsExplicitPhotoCacheAndDBPaths(t *testing.T) {
	clearBearStackEnv(t)
	t.Setenv("BEARSTACK_PHOTOS_ENABLED", "true")
	t.Setenv("BEARSTACK_PHOTOS_CACHE_DIR", "/tmp/photo-thumbs")
	t.Setenv("BEARSTACK_PHOTOS_DB_PATH", "/tmp/photo-index.sqlite")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Photos.CacheDir != "/tmp/photo-thumbs" {
		t.Fatalf("photo cache = %q", cfg.Photos.CacheDir)
	}
	if cfg.Photos.DBPath != "/tmp/photo-index.sqlite" {
		t.Fatalf("photo db = %q", cfg.Photos.DBPath)
	}
}

func TestParseEnvLine(t *testing.T) {
	tests := []struct {
		line  string
		key   string
		value string
		ok    bool
	}{
		{line: "", ok: false},
		{line: "# comment", ok: false},
		{line: "BEARSTACK_PHOTOS_ENABLED=true", key: "BEARSTACK_PHOTOS_ENABLED", value: "true", ok: true},
		{line: "export BEARSTACK_AUTH_PASSWORD='change me'", key: "BEARSTACK_AUTH_PASSWORD", value: "change me", ok: true},
		{line: `BEARSTACK_AUTH_REALM="Bear \"Stack\""`, key: "BEARSTACK_AUTH_REALM", value: `Bear "Stack"`, ok: true},
		{line: "BEARSTACK_ADDR=127.0.0.1:8080 # local", key: "BEARSTACK_ADDR", value: "127.0.0.1:8080", ok: true},
	}

	for _, tt := range tests {
		key, value, ok := parseEnvLine(tt.line)
		if ok != tt.ok || key != tt.key || value != tt.value {
			t.Fatalf("parseEnvLine(%q) = %q %q %v", tt.line, key, value, ok)
		}
	}
}
