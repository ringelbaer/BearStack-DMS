// Datei definiert HTTP-Middleware fuer Authentifizierung, Logging, Sicherheit und Kontext.
package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	Version             int    `json:"v"`
	Source              string `json:"s"`
	Subject             string `json:"sub"`
	Revision            string `json:"r"`
	User                string `json:"u"`
	Expires             int64  `json:"e"`
	PhotoAdminOnlyShown bool   `json:"pao,omitempty"`
}

func (s *Server) basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Authentication can become enabled after the first loopback bootstrap.
		// Consult the current snapshot for every request instead of permanently
		// bypassing middleware based on the state at Handler construction.
		if !s.authEnabled() {
			next.ServeHTTP(w, r)
			return
		}
		state := s.ensureAuthState()
		realm := state.realm

		if isPublicAssetPath(r.URL.Path) || isWebDAVWellKnownPath(r.URL.Path) || isLoginPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		if isWebDAVPath(r.URL.Path, s.webDAVPath()) {
			if principal, ok := s.authSessionPrincipal(r); ok {
				next.ServeHTTP(w, withAuthPrincipal(r, principal))
				return
			}

			user, pass, provided := r.BasicAuth()
			if !provided {
				writeBasicAuthUnauthorized(w, realm)
				return
			}
			principal, authOK, retryAfter := s.authenticateBasicCheck(user, pass)
			if retryAfter > 0 {
				writeBasicAuthRateLimited(w, realm, retryAfter)
				return
			}
			if !authOK {
				writeBasicAuthUnauthorized(w, realm)
				return
			}
			if !s.setAuthSessionForPrincipal(w, r, principal, authSessionDuration) {
				writeBasicAuthUnauthorized(w, realm)
				return
			}
			next.ServeHTTP(w, withAuthPrincipal(r, principal))
			return
		}

		if principal, ok := s.authSessionPrincipal(r); ok {
			next.ServeHTTP(w, withAuthPrincipal(r, principal))
			return
		}

		user, pass, provided := r.BasicAuth()
		if !provided {
			if isHTMLRequest(r) {
				s.renderLogin(w, r, http.StatusUnauthorized, "", safeReturnURL(r.URL.RequestURI()))
				return
			}
			writeBasicAuthUnauthorized(w, realm)
			return
		}
		principal, authOK, retryAfter := s.authenticateBasicCheck(user, pass)
		if retryAfter > 0 {
			writeBasicAuthRateLimited(w, realm, retryAfter)
			return
		}
		if !authOK {
			if isHTMLRequest(r) {
				s.renderLogin(w, r, http.StatusUnauthorized, "", safeReturnURL(r.URL.RequestURI()))
				return
			}
			writeBasicAuthUnauthorized(w, realm)
			return
		}
		if !s.setAuthSessionForPrincipal(w, r, principal, authSessionDuration) {
			writeBasicAuthUnauthorized(w, realm)
			return
		}
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

func writeBasicAuthRateLimited(w http.ResponseWriter, realm string, retryAfter time.Duration) {
	w.Header().Set("WWW-Authenticate", basicAuthChallenge(realm))
	w.Header().Set("Retry-After", strconv.FormatInt(retryAfterSeconds(retryAfter), 10))
	http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
}

func retryAfterSeconds(duration time.Duration) int64 {
	seconds := int64((duration + time.Second - 1) / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
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

func (s *Server) setAuthSessionForPrincipal(w http.ResponseWriter, r *http.Request, principal authPrincipal, duration time.Duration) bool {
	credential := s.authCredentialForPrincipal(principal)
	if credential == nil {
		return false
	}
	if len(s.authKey) == 0 {
		return true
	}
	if duration <= 0 {
		duration = authSessionDuration
	}
	s.writeAuthSessionCookie(w, r, authSessionPayloadForCredential(credential, time.Now().Add(duration)))
	return true
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
	snapshot := s.authSnapshot()
	if snapshot == nil {
		return authPrincipal{}, false
	}
	credential, ok := snapshot.bySubject[authSubjectKey(payload.Source, payload.Subject)]
	if !ok || !credential.enabled || credential.revision != payload.Revision || credential.username != payload.User {
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
	if !validAuthSessionPayload(payload) {
		return "", errors.New("invalid authentication session payload")
	}
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
	if !validAuthSessionPayload(payload) {
		return authSessionPayload{}, false
	}
	return payload, true
}

func validAuthSessionPayload(payload authSessionPayload) bool {
	return payload.Version == 2 &&
		(payload.Source == authSourceConfig || payload.Source == authSourceDatabase) &&
		payload.Subject != "" && payload.Revision != "" && payload.User != "" && payload.Expires > 0
}

func authSessionPayloadForCredential(credential *authCredential, expires time.Time) authSessionPayload {
	return authSessionPayload{
		Version: 2, Source: credential.source, Subject: credential.subject,
		Revision: credential.revision, User: credential.username, Expires: expires.Unix(),
	}
}

func (s *Server) authCredentialForPrincipal(principal authPrincipal) *authCredential {
	snapshot := s.authSnapshot()
	if snapshot == nil {
		return nil
	}
	credential := snapshot.bySubject[authSubjectKey(principal.Source, principal.Subject)]
	if credential == nil || !credential.enabled || credential.username != principal.Username || credential.revision != principal.Revision {
		return nil
	}
	return credential
}

func (s *Server) authPrincipalForDatabaseRevision(id int64, username string, revision int64) (authPrincipal, bool) {
	if id <= 0 || revision < 1 {
		return authPrincipal{}, false
	}
	snapshot := s.authSnapshot()
	if snapshot == nil {
		return authPrincipal{}, false
	}
	credential := snapshot.bySubject[authSubjectKey(authSourceDatabase, strconv.FormatInt(id, 10))]
	if credential == nil || !credential.enabled || credential.username != username || credential.revision != strconv.FormatInt(revision, 10) {
		return authPrincipal{}, false
	}
	return credential.principal(), true
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
