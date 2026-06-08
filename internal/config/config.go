// Datei liest, normalisiert und validiert die Laufzeitkonfiguration aus Umgebung und Defaults.
package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"bearstack/internal/boolutil"
)

const (
	defaultAddr          = "127.0.0.1:8080"
	defaultDataDir       = "data"
	defaultMaxUploadMB   = 50
	defaultRealm         = "BearStack"
	defaultPhotoPageSize = 120
	DefaultWebDAVPath    = "/webdav"
)

type Config struct {
	Addr           string       `json:"addr"`
	DataDir        string       `json:"data_dir"`
	StorageDir     string       `json:"storage_dir"`
	DBPath         string       `json:"db_path"`
	MaxUploadBytes int64        `json:"max_upload_bytes"`
	Auth           AuthConfig   `json:"auth"`
	TLS            TLSConfig    `json:"tls"`
	Photos         PhotoConfig  `json:"photos"`
	WebDAV         WebDAVConfig `json:"webdav"`
}

type AuthConfig struct {
	Username     string           `json:"username"`
	Password     string           `json:"password"`
	PasswordHash string           `json:"password_hash"`
	Realm        string           `json:"realm"`
	Credentials  []AuthCredential `json:"credentials"`
}

type AuthCredential struct {
	Username     string   `json:"username"`
	Password     string   `json:"password"`
	PasswordHash string   `json:"password_hash"`
	Role         string   `json:"role"`
	Permissions  []string `json:"permissions"`
}

type TLSConfig struct {
	Enabled  bool   `json:"enabled"`
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
	AutoCert bool   `json:"auto_cert"`
}

type PhotoConfig struct {
	Enabled  bool   `json:"enabled"`
	RootDir  string `json:"root_dir"`
	DataDir  string `json:"data_dir"`
	CacheDir string `json:"cache_dir"`
	DBPath   string `json:"db_path"`
	PageSize int    `json:"page_size"`
}

type WebDAVConfig struct {
	Path string `json:"path"`
}

func (a AuthConfig) Enabled() bool {
	if len(a.Credentials) > 0 {
		return true
	}
	return a.Username != "" && (a.Password != "" || a.PasswordHash != "")
}

func (p PhotoConfig) Active() bool {
	return p.Enabled
}

func Load() (Config, error) {
	loadEnvFiles()

	cfg := Config{
		Addr:           defaultAddr,
		DataDir:        defaultDataDir,
		MaxUploadBytes: defaultMaxUploadMB * 1024 * 1024,
		Auth: AuthConfig{
			Realm: defaultRealm,
		},
		TLS: TLSConfig{
			AutoCert: true,
		},
		Photos: PhotoConfig{
			PageSize: defaultPhotoPageSize,
		},
		WebDAV: WebDAVConfig{
			Path: DefaultWebDAVPath,
		},
	}

	if path := os.Getenv("BEARSTACK_CONFIG"); path != "" {
		if err := readConfigFile(path, &cfg); err != nil {
			return Config{}, err
		}
	}

	applyEnv(&cfg)
	derivePaths(&cfg)

	webDAVPath, err := NormalizeWebDAVPath(cfg.WebDAV.Path)
	if err != nil {
		return Config{}, err
	}
	cfg.WebDAV.Path = webDAVPath

	if cfg.MaxUploadBytes <= 0 {
		return Config{}, errors.New("max_upload_bytes must be greater than zero")
	}
	if cfg.Auth.Realm == "" {
		cfg.Auth.Realm = defaultRealm
	}
	if (cfg.TLS.CertFile == "") != (cfg.TLS.KeyFile == "") {
		return Config{}, errors.New("tls cert_file and key_file must be configured together")
	}
	if cfg.TLS.Enabled && cfg.TLS.CertFile == "" && !cfg.TLS.AutoCert {
		return Config{}, errors.New("tls cert_file and key_file are required when tls is enabled and auto_cert is disabled")
	}
	if cfg.Photos.PageSize <= 0 {
		return Config{}, errors.New("photos page_size must be greater than zero")
	}
	if !cfg.Auth.Enabled() && addrRequiresAuth(cfg.Addr) {
		return Config{}, errors.New("auth username and password or password_hash are required when addr listens on non-loopback interfaces")
	}

	return cfg, nil
}

func loadEnvFiles() {
	loadEnvFile(".env")
	if path := strings.TrimSpace(os.Getenv("BEARSTACK_ENV_FILE")); path != "" && path != ".env" {
		loadEnvFile(path)
	}
}

func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := parseEnvLine(scanner.Text())
		if !ok {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func parseEnvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false
	}
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		quote := value[0]
		if (quote == '\'' || quote == '"') && value[len(value)-1] == quote {
			value = value[1 : len(value)-1]
			if quote == '"' {
				value = strings.ReplaceAll(value, `\n`, "\n")
				value = strings.ReplaceAll(value, `\"`, `"`)
				value = strings.ReplaceAll(value, `\\`, `\`)
			}
			return key, value, true
		}
	}
	if cut := strings.Index(value, " #"); cut >= 0 {
		value = strings.TrimSpace(value[:cut])
	}
	return key, value, true
}

func readConfigFile(path string, cfg *Config) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open config file: %w", err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	if err := dec.Decode(cfg); err != nil {
		return fmt.Errorf("decode config file: %w", err)
	}
	return nil
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("BEARSTACK_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := os.Getenv("BEARSTACK_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("BEARSTACK_STORAGE_DIR"); v != "" {
		cfg.StorageDir = v
	}
	if v := os.Getenv("BEARSTACK_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("BEARSTACK_MAX_UPLOAD_MB"); v != "" {
		if mb, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.MaxUploadBytes = mb * 1024 * 1024
		}
	}
	if v := os.Getenv("BEARSTACK_MAX_UPLOAD_BYTES"); v != "" {
		if bytes, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.MaxUploadBytes = bytes
		}
	}
	if v := os.Getenv("BEARSTACK_AUTH_USER"); v != "" {
		cfg.Auth.Username = v
	}
	if v := os.Getenv("BEARSTACK_AUTH_PASSWORD"); v != "" {
		cfg.Auth.Password = v
	}
	if v := os.Getenv("BEARSTACK_AUTH_PASSWORD_HASH"); v != "" {
		cfg.Auth.PasswordHash = v
	}
	if v := os.Getenv("BEARSTACK_AUTH_REALM"); v != "" {
		cfg.Auth.Realm = v
	}
	if v, ok := envBool("BEARSTACK_TLS_ENABLED"); ok {
		cfg.TLS.Enabled = v
	}
	if v := os.Getenv("BEARSTACK_TLS_CERT_FILE"); v != "" {
		cfg.TLS.CertFile = v
	}
	if v := os.Getenv("BEARSTACK_TLS_KEY_FILE"); v != "" {
		cfg.TLS.KeyFile = v
	}
	if v, ok := envBool("BEARSTACK_TLS_AUTO_CERT"); ok {
		cfg.TLS.AutoCert = v
	}
	if v, ok := envBool("BEARSTACK_PHOTOS_ENABLED"); ok {
		cfg.Photos.Enabled = v
	}
	if v := os.Getenv("BEARSTACK_PHOTOS_DIR"); v != "" {
		cfg.Photos.RootDir = v
	}
	if v := os.Getenv("BEARSTACK_PHOTOS_DATA_DIR"); v != "" {
		cfg.Photos.DataDir = v
	}
	if v := os.Getenv("BEARSTACK_PHOTOS_CACHE_DIR"); v != "" {
		cfg.Photos.CacheDir = v
	}
	if v := os.Getenv("BEARSTACK_PHOTOS_DB_PATH"); v != "" {
		cfg.Photos.DBPath = v
	}
	if v := os.Getenv("BEARSTACK_PHOTOS_PAGE_SIZE"); v != "" {
		if size, err := strconv.Atoi(v); err == nil {
			cfg.Photos.PageSize = size
		}
	}
	if v := os.Getenv("BEARSTACK_WEBDAV_PATH"); v != "" {
		cfg.WebDAV.Path = v
	}
}

func derivePaths(cfg *Config) {
	if cfg.DataDir == "" {
		cfg.DataDir = defaultDataDir
	}
	if cfg.StorageDir == "" {
		cfg.StorageDir = filepath.Join(cfg.DataDir, "documents")
	}
	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(cfg.DataDir, "bearstack.db")
	}
	if cfg.Photos.RootDir == "" {
		cfg.Photos.RootDir = filepath.Join(cfg.DataDir, "photos")
	}
	if cfg.Photos.DataDir == "" {
		cfg.Photos.DataDir = filepath.Join(cfg.DataDir, "photos-data")
	}
	if cfg.Photos.CacheDir == "" {
		cfg.Photos.CacheDir = filepath.Join(cfg.Photos.DataDir, "thumbnails")
	}
	if cfg.Photos.DBPath == "" {
		cfg.Photos.DBPath = filepath.Join(cfg.Photos.DataDir, "photos.db")
	}
	if cfg.Photos.PageSize == 0 {
		cfg.Photos.PageSize = defaultPhotoPageSize
	}
}

func NormalizeWebDAVPath(value string) (string, error) {
	endpoint := strings.TrimSpace(value)
	if endpoint == "" {
		endpoint = DefaultWebDAVPath
	}
	if !strings.HasPrefix(endpoint, "/") {
		return "", errors.New("webdav path must start with /")
	}
	if strings.ContainsAny(endpoint, "?#") {
		return "", errors.New("webdav path must not contain query or fragment")
	}
	if strings.ContainsAny(endpoint, " \t\r\n{}") {
		return "", errors.New("webdav path must not contain whitespace or route wildcards")
	}
	endpoint = path.Clean(endpoint)
	if endpoint == "/" {
		return "", errors.New("webdav path must not be /")
	}
	if endpoint == "/.well-known/webdav" || strings.HasPrefix(endpoint, "/.well-known/webdav/") {
		return "", errors.New("webdav path must not overlap /.well-known/webdav")
	}
	return endpoint, nil
}

func envBool(key string) (bool, bool) {
	return boolutil.Parse(os.Getenv(key))
}

func addrRequiresAuth(addr string) bool {
	host := addrHost(addr)
	if host == "" {
		return true
	}
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	if host == "localhost" || host == "localhost." {
		return false
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback()
}

func addrHost(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return host
	}
	if strings.HasPrefix(addr, ":") {
		return ""
	}
	return addr
}
