package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"time"
)

func newLoopbackRequest(method, target string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, target, body)
	r.Host = "localhost"
	r.RemoteAddr = "127.0.0.1:12345"
	return r
}

func (c *documentCountCache) countSize() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.counts)
}

func (s *Server) authenticateBasic(user, password string) (authPrincipal, bool) {
	principal, ok, _ := s.authenticateBasicCheck(user, password)
	return principal, ok
}

func (svc settingsService) CacheAppName(value string) {
	if svc.app == nil {
		return
	}
	svc.app.mu.Lock()
	svc.app.appName = appNameCacheEntry{value: normalizeAppName(value), loaded: true}
	svc.app.mu.Unlock()
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
