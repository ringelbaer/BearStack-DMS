// Datei definiert den Server-Typ und verdrahtet Repository, Services, Templates und Konfiguration.
package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"bearstack/internal/config"
	"bearstack/internal/photos"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
	"bearstack/internal/tagutil"
)

//go:embed templates/*.html static/*
var webFS embed.FS

type Server struct {
	cfg         config.Config
	repo        *repository.Repository
	store       *storage.Store
	photos      *photos.Library
	log         *slog.Logger
	templates   *template.Template
	static      http.Handler
	authKey     []byte
	auth        *authState
	authInitMu  sync.Mutex
	authWriteMu sync.Mutex
	earlyAudit  auditRejectionLimiter
	apps        serverApplications
	jobCtxMu    sync.RWMutex
	jobCtx      context.Context
}

const pageSize = 100
const defaultDocumentPageSize = 100
const authSessionKeyFileName = "auth-session.key"
const authSessionKeyBytes = 32
const defaultAppName = "BearStack"
const appNameSettingKey = "app_name"
const documentColumnsSettingKey = "document_columns"
const documentPageSizeSettingKey = "document_page_size"
const homePageSettingKey = "home_page"
const homePageDocuments = "documents"
const homePageFolders = "folders"
const homePageCloud = "cloud"
const homePagePhotos = "photos"
const documentCloudEnabledSettingKey = "document_cloud_enabled"
const folderTagMinDocumentsSettingKey = "folder_tag_min_documents"
const desktopPreviewModeSettingKey = "desktop_preview_mode"
const desktopPreviewModeModal = "modal"
const desktopPreviewModeInline = "inline"
const themeModeSettingKey = "theme_mode"
const themeModeDefault = "default"
const themeModeDesign2 = "design2"
const tagDisplayModeSettingKey = "tag_display_mode"
const tagDisplayModeLower = tagutil.DisplayModeLower
const tagDisplayModeUpper = tagutil.DisplayModeUpper
const tagDisplayModeFirst = tagutil.DisplayModeFirst
const trashRetentionDaysSettingKey = "trash_retention_days"
const photoPageSizeSettingKey = "photo_page_size"
const photoFolderPreviewCountSettingKey = "photo_folder_preview_count"
const photoFolderThumbnailSizeSettingKey = "photo_folder_thumbnail_size"
const photoThumbnailSizeSettingKey = "photo_thumbnail_size"
const photoPreviewSizeSettingKey = "photo_preview_size"
const photoLargePreviewSizeSettingKey = "photo_large_preview_size"
const photoSlideshowSecondsSettingKey = "photo_slideshow_seconds"
const photoFrameSecondsSettingKey = "photo_frame_seconds"
const photoPreloadAdjacentSettingKey = "photo_preload_adjacent"
const photoMapTrackResolutionSettingKey = "photo_map_track_resolution_meters"
const photoIndexWorkerEnabledSettingKey = "photo_index_worker_enabled"
const photoIndexWorkerIntervalSettingKey = "photo_index_worker_interval_minutes"
const photoIndexWorkerDelaySettingKey = "photo_index_worker_delay_millis"
const photoThumbnailWorkerEnabledSettingKey = "photo_thumbnail_worker_enabled"
const photoThumbnailWorkerIntervalSettingKey = "photo_thumbnail_worker_interval_minutes"
const photoThumbnailWorkerBatchSettingKey = "photo_thumbnail_worker_batch_size"
const photoThumbnailConcurrencySettingKey = "photo_thumbnail_concurrency"

var defaultDocumentColumns = []string{"name", "title", "tags", "document_date", "upload_date", "size", "actions"}
var documentPageSizeOptions = []int{25, 50, 100, 200, 500}

func New(cfg config.Config, repo *repository.Repository, store *storage.Store, logger *slog.Logger) (*Server, error) {
	webDAVPath, err := config.NormalizeWebDAVPath(cfg.WebDAV.Path)
	if err != nil {
		return nil, err
	}
	cfg.WebDAV.Path = webDAVPath

	authKey, err := authSessionKey(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	authState, err := newAuthState(context.Background(), cfg.Auth, repo, authKey)
	if err != nil {
		return nil, err
	}
	snapshot := authState.snapshot.Load()
	if config.AddrRequiresAuth(cfg.Addr) && (snapshot == nil || snapshot.activeCredentials == 0) {
		return nil, errors.New("at least one active authentication account is required when addr listens on non-loopback interfaces")
	}

	tmpl, err := parseTemplates()
	if err != nil {
		return nil, err
	}
	staticFS, err := fs.Sub(webFS, "static")
	if err != nil {
		return nil, err
	}
	var photoLibrary *photos.Library
	if cfg.Photos.Active() {
		photoLibrary, err = photos.New(cfg.Photos.RootDir, cfg.Photos.CacheDir, cfg.Photos.DBPath, cfg.Photos.PageSize)
		if err != nil {
			return nil, fmt.Errorf("configure photos: %w", err)
		}
	}
	s := &Server{
		cfg:       cfg,
		repo:      repo,
		store:     store,
		photos:    photoLibrary,
		log:       logger,
		templates: tmpl,
		static:    cacheStaticAssets(staticFS),
		authKey:   authKey,
		auth:      authState,
	}
	s.apps.photo.jobs = make(chan struct{}, 1)
	s.apps.documents.thumbnails = newThumbnailService(repo, store, logger, make(chan struct{}, 1))
	s.apps.documents.ocr = newOCRService(repo, store, logger, make(chan struct{}, 1), s.invalidateDocumentCountCache, s.recordAuditLog)
	s.apps.documents.postImport = newDocumentPostProcessor(repo, store, s.apps.documents.thumbnails, logger, s.invalidateDocumentCountCache)
	s.apps.documents.importer = newDocumentImporter(repo, store, logger, s.afterDocumentCreate)
	s.apps.mail.importer = newMailImportService(cfg.MaxUploadBytes, repo, store, logger, s.apps.documents.importer, s.recordAuditLog)
	s.apps.documents.trash = newTrashService(repo, store, logger, s.trashRetentionDays, s.invalidateDocumentCountCache)
	if photoLibrary != nil {
		if settings, err := s.photoSettings(context.Background()); err == nil {
			s.configurePhotoThumbnailer(settings)
		} else if logger != nil {
			logger.Warn("photo thumbnail settings failed", "error", err)
		}
	}
	return s, nil
}

type precompressedStaticAsset struct {
	body        []byte
	contentType string
}

func cacheStaticAssets(staticFS fs.FS) http.Handler {
	next := http.FileServer(http.FS(staticFS))
	gzipped := precompressStaticAssets(staticFS)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") != "" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=300")
		}
		assetName := staticAssetName(r.URL.Path)
		if assetName != "" && isCompressibleStaticPath(assetName) {
			w.Header().Add("Vary", "Accept-Encoding")
		}
		if shouldGzipStaticAsset(r) && assetName != "" {
			if asset, ok := gzipped[assetName]; ok {
				servePrecompressedStaticAsset(w, r, asset)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func precompressStaticAssets(staticFS fs.FS) map[string]precompressedStaticAsset {
	assets := map[string]precompressedStaticAsset{}
	_ = fs.WalkDir(staticFS, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !isCompressibleStaticPath(name) {
			return nil
		}
		data, err := fs.ReadFile(staticFS, name)
		if err != nil {
			return nil
		}
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(data); err != nil {
			_ = gz.Close()
			return nil
		}
		if err := gz.Close(); err != nil {
			return nil
		}
		assets[name] = precompressedStaticAsset{
			body:        buf.Bytes(),
			contentType: staticAssetContentType(name, data),
		}
		return nil
	})
	return assets
}

func staticAssetContentType(name string, data []byte) string {
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		return contentType
	}
	return http.DetectContentType(data)
}

func staticAssetName(requestPath string) string {
	name := strings.TrimPrefix(requestPath, "/")
	if name == "" {
		return ""
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return ""
		}
	}
	return name
}

func servePrecompressedStaticAsset(w http.ResponseWriter, r *http.Request, asset precompressedStaticAsset) {
	w.Header().Set("Content-Type", asset.contentType)
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Length", strconv.Itoa(len(asset.body)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(asset.body)
}

func shouldGzipStaticAsset(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if r.Header.Get("Range") != "" || !acceptsGzip(r.Header.Get("Accept-Encoding")) {
		return false
	}
	return isCompressibleStaticPath(r.URL.Path)
}

func acceptsGzip(value string) bool {
	for _, part := range strings.Split(value, ",") {
		token, params, _ := strings.Cut(strings.TrimSpace(strings.ToLower(part)), ";")
		if token != "gzip" {
			continue
		}
		if params != "" {
			if q, ok := acceptEncodingQuality(params); ok && q <= 0 {
				return false
			}
		}
		return true
	}
	return false
}

func acceptEncodingQuality(params string) (float64, bool) {
	for _, param := range strings.Split(params, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
		if !ok || strings.TrimSpace(key) != "q" {
			continue
		}
		q, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return q, err == nil
	}
	return 1, false
}

func isCompressibleStaticPath(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".css", ".js", ".html", ".json", ".svg", ".txt", ".xml":
		return true
	default:
		return false
	}
}

func randomAuthKey() ([]byte, error) {
	key := make([]byte, authSessionKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate auth session key: %w", err)
	}
	return key, nil
}

func authSessionKey(dataDir string) ([]byte, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return randomAuthKey()
	}
	keyPath := filepath.Join(dataDir, authSessionKeyFileName)
	key, err := readAuthSessionKey(keyPath)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return createAuthSessionKey(keyPath)
}

func readAuthSessionKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(key) != authSessionKeyBytes {
		return nil, fmt.Errorf("invalid auth session key file %s", path)
	}
	return key, nil
}

func createAuthSessionKey(path string) ([]byte, error) {
	key, err := randomAuthKey()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create auth session key directory: %w", err)
	}
	data := make([]byte, hex.EncodedLen(len(key))+1)
	hex.Encode(data, key)
	data[len(data)-1] = '\n'
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return readAuthSessionKey(path)
	}
	if err != nil {
		return nil, fmt.Errorf("write auth session key: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("write auth session key: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("write auth session key: %w", err)
	}
	return key, nil
}
