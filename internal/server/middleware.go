// Datei definiert HTTP-Middleware fuer Authentifizierung, Logging, Sicherheit und Kontext.
package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	authSessionCookieName       = "bearstack_session"
	authSessionDuration         = 12 * time.Hour
	authRememberSessionDuration = 30 * 24 * time.Hour
)

type authActorContextKey struct{}
type authPrincipalContextKey struct{}

type authSessionPayload struct {
	User                string `json:"u"`
	Expires             int64  `json:"e"`
	PhotoAdminOnlyShown bool   `json:"pao,omitempty"`
}

func (s *Server) basicAuth(next http.Handler) http.Handler {
	if !s.authEnabled() {
		s.log.Warn("basic auth disabled because auth username and password or password hash are not fully configured")
		return next
	}

	realm := s.auth.realm

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicAssetPath(r.URL.Path) || isWebDAVWellKnownPath(r.URL.Path) || isLoginPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		if isWebDAVPath(r.URL.Path, s.webDAVPath()) {
			if principal, ok := s.authSessionPrincipal(r); ok {
				next.ServeHTTP(w, withAuthPrincipal(r, principal))
				return
			}

			user, pass, ok := r.BasicAuth()
			principal, authOK := s.authenticateBasic(user, pass)
			if !ok || !authOK {
				writeBasicAuthUnauthorized(w, realm)
				return
			}
			s.setAuthSession(w, r, user)
			next.ServeHTTP(w, withAuthPrincipal(r, principal))
			return
		}

		if principal, ok := s.authSessionPrincipal(r); ok {
			next.ServeHTTP(w, withAuthPrincipal(r, principal))
			return
		}

		user, pass, ok := r.BasicAuth()
		principal, authOK := s.authenticateBasic(user, pass)
		if !ok || !authOK {
			if isHTMLRequest(r) {
				s.renderLogin(w, r, http.StatusUnauthorized, "", safeReturnURL(r.URL.RequestURI()))
				return
			}
			writeBasicAuthUnauthorized(w, realm)
			return
		}
		s.setAuthSession(w, r, user)
		next.ServeHTTP(w, withAuthPrincipal(r, principal))
	})
}

func isHTMLRequest(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if isBrowserNavigation(r) {
		return true
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	return strings.Contains(accept, "text/html") || strings.Contains(accept, "application/xhtml+xml")
}

func isBrowserNavigation(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Mode"), "navigate") {
		return true
	}
	return strings.EqualFold(r.Header.Get("Sec-Fetch-Dest"), "document")
}

func isPublicAssetPath(path string) bool {
	if path == "/favicon.ico" || path == "/favicon/custom" {
		return true
	}
	return strings.HasPrefix(path, "/static/")
}

func isLoginPath(path string) bool {
	return path == "/login"
}

func isWebDAVPath(path, webDAVPath string) bool {
	if path == webDAVPath {
		return true
	}
	return strings.HasPrefix(path, webDAVPath+"/")
}

func withAuthPrincipal(r *http.Request, principal authPrincipal) *http.Request {
	if strings.TrimSpace(principal.Username) == "" {
		return r
	}
	ctx := context.WithValue(r.Context(), authPrincipalContextKey{}, principal)
	ctx = context.WithValue(ctx, authActorContextKey{}, principal.Username)
	return r.WithContext(ctx)
}

func authPrincipalFromContext(ctx context.Context) (authPrincipal, bool) {
	principal, ok := ctx.Value(authPrincipalContextKey{}).(authPrincipal)
	if !ok || strings.TrimSpace(principal.Username) == "" {
		return authPrincipal{}, false
	}
	return principal, true
}

func writeBasicAuthUnauthorized(w http.ResponseWriter, realm string) {
	w.Header().Set("WWW-Authenticate", basicAuthChallenge(realm))
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

func basicAuthChallenge(realm string) string {
	return "Basic realm=" + strconv.Quote(cleanAuthRealm(realm)) + `, charset="UTF-8"`
}

func cleanAuthRealm(realm string) string {
	realm = strings.TrimSpace(realm)
	realm = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, realm)
	realm = strings.TrimSpace(realm)
	if realm == "" {
		return "BearStack"
	}
	return realm
}

func (s *Server) authPasswordOK(r *http.Request, password string) bool {
	if !s.authEnabled() {
		return false
	}
	principal, ok := authPrincipalFromContext(r.Context())
	if !ok {
		return false
	}
	credential, ok := s.auth.credentials[principal.Username]
	return ok && credential.passwordOK(password)
}

func (s *Server) setAuthSession(w http.ResponseWriter, r *http.Request, user string) {
	s.setAuthSessionWithDuration(w, r, user, authSessionDuration)
}

func (s *Server) setRememberedAuthSession(w http.ResponseWriter, r *http.Request, user string) {
	s.setAuthSessionWithDuration(w, r, user, authRememberSessionDuration)
}

func (s *Server) setAuthSessionWithDuration(w http.ResponseWriter, r *http.Request, user string, duration time.Duration) {
	if len(s.authKey) == 0 {
		return
	}
	if duration <= 0 {
		duration = authSessionDuration
	}
	expires := time.Now().Add(duration)
	s.writeAuthSessionCookie(w, r, authSessionPayload{
		User:    user,
		Expires: expires.Unix(),
	})
}

func (s *Server) writeAuthSessionCookie(w http.ResponseWriter, r *http.Request, payload authSessionPayload) {
	if len(s.authKey) == 0 {
		return
	}
	expires := time.Unix(payload.Expires, 0)
	if payload.Expires <= 0 {
		expires = time.Now().Add(authSessionDuration)
		payload.Expires = expires.Unix()
	}
	value, err := s.signAuthSession(payload)
	if err != nil {
		s.log.Warn("failed to create auth session", "error", err)
		return
	}
	maxAge := int(time.Until(expires).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(w, &http.Cookie{
		Name:     authSessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestUsesHTTPS(r),
	})
}

func clearAuthSession(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     authSessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestUsesHTTPS(r),
	})
}

func (s *Server) authSessionPrincipal(r *http.Request) (authPrincipal, bool) {
	payload, ok := s.authSessionFromRequest(r)
	if !ok {
		return authPrincipal{}, false
	}
	credential, ok := s.auth.credentials[payload.User]
	if !ok {
		return authPrincipal{}, false
	}
	return credential.principal(), true
}

func (s *Server) authSessionFromRequest(r *http.Request) (authSessionPayload, bool) {
	if len(s.authKey) == 0 {
		return authSessionPayload{}, false
	}
	cookie, err := r.Cookie(authSessionCookieName)
	if err != nil {
		return authSessionPayload{}, false
	}
	payload, ok := s.verifyAuthSession(cookie.Value)
	if !ok || time.Now().Unix() > payload.Expires {
		return authSessionPayload{}, false
	}
	return payload, true
}

func (s *Server) signAuthSession(payload authSessionPayload) (string, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	signature := authSessionSignature(s.authKey, payloadBytes)
	return base64.RawURLEncoding.EncodeToString(payloadBytes) + "." +
		base64.RawURLEncoding.EncodeToString(signature), nil
}

func (s *Server) verifyAuthSession(value string) (authSessionPayload, bool) {
	encodedPayload, encodedSignature, ok := strings.Cut(value, ".")
	if !ok {
		return authSessionPayload{}, false
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return authSessionPayload{}, false
	}
	signature, err := base64.RawURLEncoding.DecodeString(encodedSignature)
	if err != nil {
		return authSessionPayload{}, false
	}
	if !hmac.Equal(signature, authSessionSignature(s.authKey, payloadBytes)) {
		return authSessionPayload{}, false
	}
	var payload authSessionPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return authSessionPayload{}, false
	}
	return payload, true
}

func authSessionSignature(key, payload []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return mac.Sum(nil)
}

func requestUsesHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) sameOriginUnsafeRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isUnsafeRequestMethod(r.Method) && !s.requestHasSameOrigin(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isUnsafeRequestMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace, "PROPFIND":
		return false
	default:
		return true
	}
}

func (s *Server) requestHasSameOrigin(r *http.Request) bool {
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		return originMatchesRequest(r, origin)
	}
	if referer := strings.TrimSpace(r.Header.Get("Referer")); referer != "" {
		return originMatchesRequest(r, referer)
	}
	return s == nil || !s.authEnabled() || requestHasExplicitAuthorization(r)
}

func requestHasExplicitAuthorization(r *http.Request) bool {
	return strings.TrimSpace(r.Header.Get("Authorization")) != ""
}

func originMatchesRequest(r *http.Request, value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}

	requestScheme := originRequestScheme(r)
	requestHost, requestPort, ok := splitOriginHost(r.Host, requestScheme)
	if !ok {
		return false
	}
	originHost, originPort, ok := splitOriginHost(parsed.Host, parsed.Scheme)
	if !ok {
		return false
	}
	return strings.EqualFold(parsed.Scheme, requestScheme) &&
		strings.EqualFold(originHost, requestHost) &&
		originPort == requestPort
}

func originRequestScheme(r *http.Request) string {
	if requestUsesHTTPS(r) {
		return "https"
	}
	return "http"
}

func splitOriginHost(hostPort, scheme string) (string, string, bool) {
	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return "", "", false
	}
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		host = hostPort
		port = defaultOriginPort(scheme)
		if strings.Count(hostPort, ":") == 1 {
			candidateHost, candidatePort, cut := strings.Cut(hostPort, ":")
			if cut {
				host = candidateHost
				port = candidatePort
			}
		}
	}
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	if host == "" || port == "" {
		return "", "", false
	}
	return host, port, true
}

func defaultOriginPort(scheme string) string {
	if strings.EqualFold(scheme, "https") {
		return "443"
	}
	return "80"
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: https://tile.openstreetmap.org; script-src 'self'; style-src 'self' 'unsafe-inline'; frame-src 'self' https://www.openstreetmap.org; object-src 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}
