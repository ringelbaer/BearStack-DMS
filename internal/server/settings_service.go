// Datei zentralisiert Anwendungseinstellungen, Normalisierung, Defaults und Cache.
package server

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"bearstack/internal/photos"
	"bearstack/internal/tagutil"
)

const (
	photoSettingsCacheTTL       = 30 * time.Second
	renderSettingsCacheTTL      = 30 * time.Second
	folderTagMinDocumentsMax    = 100000
	photoFolderThumbnailMaxSize = 640
	photoWorkerThumbnailMaxSize = 1200
	photoPreviewMinSize         = 640
	photoPreviewMaxSize         = 1920
)

type appSettingsState struct {
	mu      sync.RWMutex
	appName appNameCacheEntry
	favicon customFaviconCacheEntry
	render  renderSettingsCacheEntry
}

type appNameCacheEntry struct {
	value  string
	loaded bool
}

type renderSettingsCacheEntry struct {
	value     renderSettingsSnapshot
	expiresAt time.Time
}

type renderSettingsSnapshot struct {
	TagDisplayMode       string
	ThemeMode            string
	HomePage             string
	DocumentCloudEnabled bool
}

type photoSettingsState struct {
	mu    sync.RWMutex
	cache photoSettingsCacheEntry
}

type photoSettingsCacheEntry struct {
	value     PhotoSettings
	expiresAt time.Time
}

type PhotoSettings struct {
	PageSize                       int
	FolderPreviewCount             int
	FolderThumbnailSize            int
	ThumbnailSize                  int
	PreviewSize                    int
	LargePreviewSize               int
	SlideshowSeconds               int
	FrameSeconds                   int
	PreloadAdjacent                bool
	MapTrackResolutionMeters       int
	IndexWorkerEnabled             bool
	IndexWorkerIntervalMinutes     int
	IndexWorkerDelayMillis         int
	ThumbnailWorkerEnabled         bool
	ThumbnailWorkerIntervalMinutes int
	ThumbnailWorkerBatchSize       int
	ThumbnailConcurrency           int
}

type settingsStore interface {
	settingReader
	settingWriter
}

type settingsService struct {
	store settingsStore
	app   *appSettingsState
	photo *photoSettingsState
}

func (s *Server) settingsService() settingsService {
	if s == nil {
		return settingsService{}
	}
	svc := settingsService{
		app:   &s.apps.settings,
		photo: &s.apps.photo.settings,
	}
	if s.repo != nil {
		svc.store = s.repo
	}
	return svc
}

func (s *Server) desktopPreviewMode(ctx context.Context) (string, error) {
	return s.settingsService().DesktopPreviewMode(ctx)
}

func (s *Server) appName(ctx context.Context) (string, error) {
	return s.settingsService().AppName(ctx)
}

func (s *Server) cacheAppName(value string) {
	s.settingsService().CacheAppName(value)
}

func (s *Server) tagDisplayMode(ctx context.Context) (string, error) {
	return s.settingsService().TagDisplayMode(ctx)
}

func (s *Server) themeMode(ctx context.Context) (string, error) {
	return s.settingsService().ThemeMode(ctx)
}

func (s *Server) homePage(ctx context.Context) (string, error) {
	return s.settingsService().HomePage(ctx)
}

func (s *Server) documentCloudEnabled(ctx context.Context) (bool, error) {
	return s.settingsService().DocumentCloudEnabled(ctx)
}

func (s *Server) renderSettings(ctx context.Context) (renderSettingsSnapshot, error) {
	return s.settingsService().RenderSettings(ctx)
}

func (s *Server) trashRetentionDays(ctx context.Context) (int, error) {
	return s.settingsService().TrashRetentionDays(ctx)
}

func (s *Server) folderTagMinDocuments(ctx context.Context) (int, error) {
	return s.settingsService().FolderTagMinDocuments(ctx)
}

func (s *Server) photoSettings(ctx context.Context) (PhotoSettings, error) {
	return s.settingsService().PhotoSettings(ctx)
}

func (s *Server) savePhotoSettings(ctx context.Context, settings PhotoSettings) error {
	return s.settingsService().SavePhotoSettings(ctx, settings)
}

func (svc settingsService) AppName(ctx context.Context) (string, error) {
	if svc.store == nil {
		return defaultAppName, nil
	}
	if svc.app != nil {
		svc.app.mu.RLock()
		if svc.app.appName.loaded {
			value := svc.app.appName.value
			svc.app.mu.RUnlock()
			return value, nil
		}
		svc.app.mu.RUnlock()
	}

	value, err := appName(ctx, svc.store)
	if err != nil {
		return "", err
	}
	svc.CacheAppName(value)
	return value, nil
}

func (svc settingsService) CacheAppName(value string) {
	if svc.app == nil {
		return
	}
	svc.app.mu.Lock()
	svc.app.appName = appNameCacheEntry{value: normalizeAppName(value), loaded: true}
	svc.app.mu.Unlock()
}

func (svc settingsService) DesktopPreviewMode(ctx context.Context) (string, error) {
	if svc.store == nil {
		return desktopPreviewModeModal, nil
	}
	return desktopPreviewMode(ctx, svc.store)
}

func (svc settingsService) TagDisplayMode(ctx context.Context) (string, error) {
	if svc.store == nil {
		return tagDisplayModeLower, nil
	}
	return tagDisplayMode(ctx, svc.store)
}

func (svc settingsService) ThemeMode(ctx context.Context) (string, error) {
	if svc.store == nil {
		return themeModeDefault, nil
	}
	return themeMode(ctx, svc.store)
}

func (svc settingsService) HomePage(ctx context.Context) (string, error) {
	if svc.store == nil {
		return homePageDocuments, nil
	}
	return homePage(ctx, svc.store)
}

func (svc settingsService) DocumentCloudEnabled(ctx context.Context) (bool, error) {
	if svc.store == nil {
		return false, nil
	}
	return documentCloudEnabled(ctx, svc.store)
}

func (svc settingsService) RenderSettings(ctx context.Context) (renderSettingsSnapshot, error) {
	if svc.store == nil {
		return defaultRenderSettingsSnapshot(), nil
	}
	now := time.Now()
	if svc.app != nil {
		svc.app.mu.RLock()
		entry := svc.app.render
		if now.Before(entry.expiresAt) {
			svc.app.mu.RUnlock()
			return entry.value, nil
		}
		svc.app.mu.RUnlock()
	}

	return svc.ReloadRenderSettings(ctx)
}

// ReloadRenderSettings preserves values belonging to other settings forms when
// refreshing the shared rendering cache after a save.
func (svc settingsService) ReloadRenderSettings(ctx context.Context) (renderSettingsSnapshot, error) {
	if svc.store == nil {
		return defaultRenderSettingsSnapshot(), nil
	}
	settings := defaultRenderSettingsSnapshot()
	var err error
	if settings.TagDisplayMode, err = tagDisplayMode(ctx, svc.store); err != nil {
		return renderSettingsSnapshot{}, err
	}
	if settings.ThemeMode, err = themeMode(ctx, svc.store); err != nil {
		return renderSettingsSnapshot{}, err
	}
	if settings.HomePage, err = homePage(ctx, svc.store); err != nil {
		return renderSettingsSnapshot{}, err
	}
	if settings.DocumentCloudEnabled, err = documentCloudEnabled(ctx, svc.store); err != nil {
		return renderSettingsSnapshot{}, err
	}
	svc.CacheRenderSettings(settings)
	return settings, nil
}

func (svc settingsService) CacheRenderSettings(settings renderSettingsSnapshot) {
	if svc.app == nil {
		return
	}
	settings = normalizeRenderSettingsSnapshot(settings)
	svc.app.mu.Lock()
	svc.app.render = renderSettingsCacheEntry{
		value:     settings,
		expiresAt: time.Now().Add(renderSettingsCacheTTL),
	}
	svc.app.mu.Unlock()
}

func (svc settingsService) TrashRetentionDays(ctx context.Context) (int, error) {
	if svc.store == nil {
		return 0, nil
	}
	return trashRetentionDays(ctx, svc.store)
}

func (svc settingsService) FolderTagMinDocuments(ctx context.Context) (int, error) {
	if svc.store == nil {
		return 0, nil
	}
	return folderTagMinDocuments(ctx, svc.store)
}

func (svc settingsService) PhotoSettings(ctx context.Context) (PhotoSettings, error) {
	if svc.store == nil {
		return defaultPhotoSettings(), nil
	}
	now := time.Now()
	if svc.photo != nil {
		svc.photo.mu.RLock()
		entry := svc.photo.cache
		if now.Before(entry.expiresAt) {
			svc.photo.mu.RUnlock()
			return entry.value, nil
		}
		svc.photo.mu.RUnlock()
	}

	settings, err := photoSettings(ctx, svc.store)
	if err != nil {
		return PhotoSettings{}, err
	}
	svc.cachePhotoSettings(settings)
	return settings, nil
}

func (svc settingsService) SavePhotoSettings(ctx context.Context, settings PhotoSettings) error {
	if svc.store == nil {
		return nil
	}
	if err := savePhotoSettings(ctx, svc.store, settings); err != nil {
		return err
	}
	svc.cachePhotoSettings(settings)
	return nil
}

func (svc settingsService) cachePhotoSettings(settings PhotoSettings) {
	if svc.photo == nil {
		return
	}
	svc.photo.mu.Lock()
	svc.photo.cache = photoSettingsCacheEntry{
		value:     settings,
		expiresAt: time.Now().Add(photoSettingsCacheTTL),
	}
	svc.photo.mu.Unlock()
}

func appName(ctx context.Context, settings settingReader) (string, error) {
	value, ok, err := settings.GetSetting(ctx, appNameSettingKey)
	if err != nil {
		return "", err
	}
	if !ok {
		return defaultAppName, nil
	}
	return normalizeAppName(value), nil
}

func normalizeAppName(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return defaultAppName
	}
	const maxRunes = 80
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return value
}

func desktopPreviewMode(ctx context.Context, settings settingReader) (string, error) {
	value, ok, err := settings.GetSetting(ctx, desktopPreviewModeSettingKey)
	if err != nil {
		return "", err
	}
	if !ok {
		return desktopPreviewModeModal, nil
	}
	return normalizeDesktopPreviewMode(value), nil
}

func normalizeDesktopPreviewMode(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case desktopPreviewModeInline:
		return desktopPreviewModeInline
	default:
		return desktopPreviewModeModal
	}
}

func tagDisplayMode(ctx context.Context, settings settingReader) (string, error) {
	value, ok, err := settings.GetSetting(ctx, tagDisplayModeSettingKey)
	if err != nil {
		return "", err
	}
	if !ok {
		return tagDisplayModeLower, nil
	}
	return normalizeTagDisplayMode(value), nil
}

func normalizeTagDisplayMode(value string) string {
	return tagutil.NormalizeDisplayMode(value)
}

func tagDisplayOptions() []TagDisplayOption {
	return []TagDisplayOption{
		{Value: tagDisplayModeLower, Label: "kleinschreibung"},
		{Value: tagDisplayModeFirst, Label: "Erster Buchstabe Groß"},
		{Value: tagDisplayModeUpper, Label: "GROSSSCHREIBUNG"},
	}
}

func themeMode(ctx context.Context, settings settingReader) (string, error) {
	value, ok, err := settings.GetSetting(ctx, themeModeSettingKey)
	if err != nil {
		return "", err
	}
	if !ok {
		return themeModeDefault, nil
	}
	return normalizeThemeMode(value), nil
}

func normalizeThemeMode(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case themeModeDesign2:
		return themeModeDesign2
	default:
		return themeModeDefault
	}
}

func themeOptions() []ThemeOption {
	return []ThemeOption{
		{Value: themeModeDefault, Label: "Standard", Description: "Kühles BearStack-Design"},
		{Value: themeModeDesign2, Label: "Wüste", Description: "Warme, helle Admin-Oberfläche"},
	}
}

func homePage(ctx context.Context, settings settingReader) (string, error) {
	value, ok, err := settings.GetSetting(ctx, homePageSettingKey)
	if err != nil {
		return "", err
	}
	if !ok {
		return homePageDocuments, nil
	}
	return normalizeHomePage(value), nil
}

func normalizeHomePage(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case homePageFolders:
		return homePageFolders
	case homePageCloud:
		return homePageCloud
	case homePagePhotos:
		return homePagePhotos
	default:
		return homePageDocuments
	}
}

func normalizeAvailableHomePage(value string, photosEnabled, cloudEnabled bool) string {
	page := normalizeHomePage(value)
	if page == homePageCloud && !cloudEnabled {
		return homePageDocuments
	}
	if page == homePagePhotos && !photosEnabled {
		return homePageDocuments
	}
	return page
}

func homePageURL(page string) string {
	switch normalizeHomePage(page) {
	case homePageFolders:
		return "/folders"
	case homePageCloud:
		return "/cloud"
	case homePagePhotos:
		return "/photos"
	default:
		return "/documents"
	}
}

func homeURLForPermissions(page string, auth AuthPermissions, photosEnabled bool) string {
	if auth.CanDocumentsRead {
		return homePageURL(page)
	}
	if photosEnabled && auth.CanPhotosRead {
		return "/photos"
	}
	if auth.CanSystemManage {
		return "/settings"
	}
	if photosEnabled && auth.CanPhotosManage {
		return "/settings/photos"
	}
	if auth.CanSystemUsersManage {
		return "/settings/users"
	}
	if auth.CanSystemAudit {
		return "/log"
	}
	return "/help"
}

func (s *Server) resolvedHomePage(ctx context.Context, auth AuthPermissions) (string, error) {
	page := homePageDocuments
	cloudEnabled := false
	if s != nil {
		configured, err := s.homePage(ctx)
		if err != nil {
			return "", err
		}
		page = configured
		enabled, err := s.documentCloudEnabled(ctx)
		if err != nil {
			return "", err
		}
		cloudEnabled = enabled
	}
	return s.resolveHomePage(page, auth, cloudEnabled), nil
}

func (s *Server) resolveHomePage(page string, auth AuthPermissions, cloudEnabled bool) string {
	page = normalizeAvailableHomePage(page, s != nil && s.photos != nil, cloudEnabled)
	if homePageAllowed(page, auth, s != nil && s.photos != nil, cloudEnabled) {
		return page
	}
	if auth.CanDocumentsRead {
		return homePageDocuments
	}
	if s != nil && s.photos != nil && auth.CanPhotosRead {
		return homePagePhotos
	}
	return homePageDocuments
}

func homePageAllowed(page string, auth AuthPermissions, photosEnabled, cloudEnabled bool) bool {
	switch normalizeHomePage(page) {
	case homePagePhotos:
		return photosEnabled && auth.CanPhotosRead
	case homePageCloud:
		return cloudEnabled && auth.CanDocumentsRead
	case homePageDocuments, homePageFolders:
		return auth.CanDocumentsRead
	default:
		return auth.CanDocumentsRead
	}
}

func homePageOptions(photosEnabled, cloudEnabled bool) []HomePageOption {
	options := []HomePageOption{
		{Value: homePageDocuments, Label: "Dokumente", URL: homePageURL(homePageDocuments)},
		{Value: homePageFolders, Label: "Ordner", URL: homePageURL(homePageFolders)},
	}
	if cloudEnabled {
		options = append(options, HomePageOption{Value: homePageCloud, Label: "Wolke", URL: homePageURL(homePageCloud)})
	}
	if photosEnabled {
		options = append(options, HomePageOption{Value: homePagePhotos, Label: "Fotos", URL: homePageURL(homePagePhotos)})
	}
	return options
}

func defaultRenderSettingsSnapshot() renderSettingsSnapshot {
	return renderSettingsSnapshot{
		TagDisplayMode:       tagDisplayModeLower,
		ThemeMode:            themeModeDefault,
		HomePage:             homePageDocuments,
		DocumentCloudEnabled: false,
	}
}

func normalizeRenderSettingsSnapshot(settings renderSettingsSnapshot) renderSettingsSnapshot {
	settings.TagDisplayMode = normalizeTagDisplayMode(settings.TagDisplayMode)
	settings.ThemeMode = normalizeThemeMode(settings.ThemeMode)
	settings.HomePage = normalizeHomePage(settings.HomePage)
	return settings
}

func documentCloudEnabled(ctx context.Context, settings settingReader) (bool, error) {
	return boolSetting(ctx, settings, documentCloudEnabledSettingKey, false)
}

func trashRetentionDays(ctx context.Context, settings settingReader) (int, error) {
	value, ok, err := settings.GetSetting(ctx, trashRetentionDaysSettingKey)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, nil
	}
	return normalizeTrashRetentionDays(value), nil
}

func normalizeTrashRetentionDays(value string) int {
	days, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	switch days {
	case 30, 60, 90:
		return days
	default:
		return 0
	}
}

func trashRetentionOptions() []TrashRetentionOption {
	return []TrashRetentionOption{
		{Value: 0, Label: "Nie"},
		{Value: 30, Label: "Nach 30 Tagen"},
		{Value: 60, Label: "Nach 60 Tagen"},
		{Value: 90, Label: "Nach 90 Tagen"},
	}
}

func folderTagMinDocuments(ctx context.Context, settings settingReader) (int, error) {
	value, ok, err := settings.GetSetting(ctx, folderTagMinDocumentsSettingKey)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, nil
	}
	return normalizeFolderTagMinDocuments(value), nil
}

func normalizeFolderTagMinDocuments(value string) int {
	return boundedInt(value, 0, 0, folderTagMinDocumentsMax)
}

func defaultPhotoSettings() PhotoSettings {
	return PhotoSettings{
		PageSize:                       120,
		FolderPreviewCount:             photos.MaxFolderPreviewCount,
		FolderThumbnailSize:            320,
		ThumbnailSize:                  photos.DefaultThumbnailSize,
		PreviewSize:                    photoPreviewMaxSize,
		LargePreviewSize:               photos.MaxThumbnailSize,
		SlideshowSeconds:               5,
		FrameSeconds:                   8,
		PreloadAdjacent:                true,
		MapTrackResolutionMeters:       photos.DefaultRouteClusterRadiusMeters,
		IndexWorkerEnabled:             false,
		IndexWorkerIntervalMinutes:     1440,
		IndexWorkerDelayMillis:         250,
		ThumbnailWorkerEnabled:         false,
		ThumbnailWorkerIntervalMinutes: 15,
		ThumbnailWorkerBatchSize:       15,
		ThumbnailConcurrency:           1,
	}
}

func normalizePhotoSettings(settings PhotoSettings) PhotoSettings {
	settings.PageSize = max(1, min(1000, settings.PageSize))
	settings.FolderPreviewCount = max(photos.MinFolderPreviewCount, min(photos.MaxFolderPreviewCount, settings.FolderPreviewCount))
	settings.FolderThumbnailSize = max(photos.MinThumbnailSize, min(photoFolderThumbnailMaxSize, settings.FolderThumbnailSize))
	settings.ThumbnailSize = max(photos.MinThumbnailSize, min(photoWorkerThumbnailMaxSize, settings.ThumbnailSize))
	settings.PreviewSize = max(photoPreviewMinSize, min(photoPreviewMaxSize, settings.PreviewSize))
	settings.LargePreviewSize = max(photoPreviewMaxSize, min(photos.MaxThumbnailSize, settings.LargePreviewSize))
	settings.SlideshowSeconds = max(2, min(60, settings.SlideshowSeconds))
	settings.FrameSeconds = max(3, min(300, settings.FrameSeconds))
	settings.MapTrackResolutionMeters = max(500, min(10000, settings.MapTrackResolutionMeters))
	settings.IndexWorkerIntervalMinutes = max(1, min(10080, settings.IndexWorkerIntervalMinutes))
	settings.IndexWorkerDelayMillis = max(50, min(5000, settings.IndexWorkerDelayMillis))
	settings.ThumbnailWorkerIntervalMinutes = max(1, min(1440, settings.ThumbnailWorkerIntervalMinutes))
	settings.ThumbnailWorkerBatchSize = max(1, min(1000, settings.ThumbnailWorkerBatchSize))
	settings.ThumbnailConcurrency = max(1, min(4, settings.ThumbnailConcurrency))
	settings.MapTrackResolutionMeters = photos.NormalizeRouteClusterRadiusMeters(settings.MapTrackResolutionMeters)
	return settings
}

func photoSettings(ctx context.Context, settings settingReader) (PhotoSettings, error) {
	result := defaultPhotoSettings()
	var err error
	if result.PageSize, err = intSetting(ctx, settings, photoPageSizeSettingKey, result.PageSize); err != nil {
		return PhotoSettings{}, err
	}
	if result.FolderPreviewCount, err = intSetting(ctx, settings, photoFolderPreviewCountSettingKey, result.FolderPreviewCount); err != nil {
		return PhotoSettings{}, err
	}
	if result.FolderThumbnailSize, err = intSetting(ctx, settings, photoFolderThumbnailSizeSettingKey, result.FolderThumbnailSize); err != nil {
		return PhotoSettings{}, err
	}
	if result.ThumbnailSize, err = intSetting(ctx, settings, photoThumbnailSizeSettingKey, result.ThumbnailSize); err != nil {
		return PhotoSettings{}, err
	}
	if result.PreviewSize, err = intSetting(ctx, settings, photoPreviewSizeSettingKey, result.PreviewSize); err != nil {
		return PhotoSettings{}, err
	}
	if result.LargePreviewSize, err = intSetting(ctx, settings, photoLargePreviewSizeSettingKey, result.LargePreviewSize); err != nil {
		return PhotoSettings{}, err
	}
	if result.SlideshowSeconds, err = intSetting(ctx, settings, photoSlideshowSecondsSettingKey, result.SlideshowSeconds); err != nil {
		return PhotoSettings{}, err
	}
	if result.FrameSeconds, err = intSetting(ctx, settings, photoFrameSecondsSettingKey, result.FrameSeconds); err != nil {
		return PhotoSettings{}, err
	}
	if result.PreloadAdjacent, err = boolSetting(ctx, settings, photoPreloadAdjacentSettingKey, result.PreloadAdjacent); err != nil {
		return PhotoSettings{}, err
	}
	if result.MapTrackResolutionMeters, err = intSetting(ctx, settings, photoMapTrackResolutionSettingKey, result.MapTrackResolutionMeters); err != nil {
		return PhotoSettings{}, err
	}
	if result.IndexWorkerEnabled, err = boolSetting(ctx, settings, photoIndexWorkerEnabledSettingKey, result.IndexWorkerEnabled); err != nil {
		return PhotoSettings{}, err
	}
	if result.IndexWorkerIntervalMinutes, err = intSetting(ctx, settings, photoIndexWorkerIntervalSettingKey, result.IndexWorkerIntervalMinutes); err != nil {
		return PhotoSettings{}, err
	}
	if result.IndexWorkerDelayMillis, err = intSetting(ctx, settings, photoIndexWorkerDelaySettingKey, result.IndexWorkerDelayMillis); err != nil {
		return PhotoSettings{}, err
	}
	if result.ThumbnailWorkerEnabled, err = boolSetting(ctx, settings, photoThumbnailWorkerEnabledSettingKey, result.ThumbnailWorkerEnabled); err != nil {
		return PhotoSettings{}, err
	}
	if result.ThumbnailWorkerIntervalMinutes, err = intSetting(ctx, settings, photoThumbnailWorkerIntervalSettingKey, result.ThumbnailWorkerIntervalMinutes); err != nil {
		return PhotoSettings{}, err
	}
	if result.ThumbnailWorkerBatchSize, err = intSetting(ctx, settings, photoThumbnailWorkerBatchSettingKey, result.ThumbnailWorkerBatchSize); err != nil {
		return PhotoSettings{}, err
	}
	if result.ThumbnailConcurrency, err = intSetting(ctx, settings, photoThumbnailConcurrencySettingKey, result.ThumbnailConcurrency); err != nil {
		return PhotoSettings{}, err
	}
	return normalizePhotoSettings(result), nil
}

func savePhotoSettings(ctx context.Context, store settingWriter, settings PhotoSettings) error {
	values := map[string]string{
		photoPageSizeSettingKey:                strconv.Itoa(settings.PageSize),
		photoFolderPreviewCountSettingKey:      strconv.Itoa(settings.FolderPreviewCount),
		photoFolderThumbnailSizeSettingKey:     strconv.Itoa(settings.FolderThumbnailSize),
		photoThumbnailSizeSettingKey:           strconv.Itoa(settings.ThumbnailSize),
		photoPreviewSizeSettingKey:             strconv.Itoa(settings.PreviewSize),
		photoLargePreviewSizeSettingKey:        strconv.Itoa(settings.LargePreviewSize),
		photoSlideshowSecondsSettingKey:        strconv.Itoa(settings.SlideshowSeconds),
		photoFrameSecondsSettingKey:            strconv.Itoa(settings.FrameSeconds),
		photoPreloadAdjacentSettingKey:         boolSettingValue(settings.PreloadAdjacent),
		photoMapTrackResolutionSettingKey:      strconv.Itoa(photos.NormalizeRouteClusterRadiusMeters(settings.MapTrackResolutionMeters)),
		photoIndexWorkerEnabledSettingKey:      boolSettingValue(settings.IndexWorkerEnabled),
		photoIndexWorkerIntervalSettingKey:     strconv.Itoa(settings.IndexWorkerIntervalMinutes),
		photoIndexWorkerDelaySettingKey:        strconv.Itoa(settings.IndexWorkerDelayMillis),
		photoThumbnailWorkerEnabledSettingKey:  boolSettingValue(settings.ThumbnailWorkerEnabled),
		photoThumbnailWorkerIntervalSettingKey: strconv.Itoa(settings.ThumbnailWorkerIntervalMinutes),
		photoThumbnailWorkerBatchSettingKey:    strconv.Itoa(settings.ThumbnailWorkerBatchSize),
		photoThumbnailConcurrencySettingKey:    strconv.Itoa(settings.ThumbnailConcurrency),
	}
	for key, value := range values {
		if err := store.SaveSetting(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}

func intSetting(ctx context.Context, settings settingReader, key string, fallback int) (int, error) {
	value, ok, err := settings.GetSetting(ctx, key)
	if err != nil {
		return 0, err
	}
	if !ok {
		return fallback, nil
	}
	return parseIntOrDefault(value, fallback), nil
}

func boolSetting(ctx context.Context, settings settingReader, key string, fallback bool) (bool, error) {
	value, ok, err := settings.GetSetting(ctx, key)
	if err != nil {
		return false, err
	}
	if !ok {
		return fallback, nil
	}
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return fallback, nil
	}
}

func boundedInt(value string, fallback, min, max int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	if parsed < min {
		return min
	}
	if parsed > max {
		return max
	}
	return parsed
}

func boolSettingValue(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func parseIntOrDefault(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}
