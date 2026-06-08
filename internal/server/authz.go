// Datei prueft Berechtigungen und Sichtbarkeit fuer geschuetzte Dokument- und Fotozugriffe.
package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"bearstack/internal/config"

	"golang.org/x/crypto/bcrypt"
)

const (
	authBasicCacheTTL        = 5 * time.Minute
	authBasicCacheMaxEntries = 256
)

type authCapabilities uint64

const (
	authCapDocumentsRead authCapabilities = 1 << iota
	authCapDocumentsWebDAVRead
	authCapDocumentsUpload
	authCapDocumentsEdit
	authCapDocumentsDelete
	authCapDocumentsStructure
	authCapPhotosRead
	authCapPhotosEdit
	authCapPhotosManage
	authCapSystemManage
	authCapSystemAudit
)

const authCapAll = authCapDocumentsRead |
	authCapDocumentsWebDAVRead |
	authCapDocumentsUpload |
	authCapDocumentsEdit |
	authCapDocumentsDelete |
	authCapDocumentsStructure |
	authCapPhotosRead |
	authCapPhotosEdit |
	authCapPhotosManage |
	authCapSystemManage |
	authCapSystemAudit

var authPermissionByName = map[string]authCapabilities{
	"documents.read":        authCapDocumentsRead,
	"documents.webdav.read": authCapDocumentsWebDAVRead,
	"documents.upload":      authCapDocumentsUpload,
	"documents.edit":        authCapDocumentsEdit,
	"documents.delete":      authCapDocumentsDelete,
	"documents.structure":   authCapDocumentsStructure,
	"photos.read":           authCapPhotosRead,
	"photos.edit":           authCapPhotosEdit,
	"photos.manage":         authCapPhotosManage,
	"system.manage":         authCapSystemManage,
	"system.audit":          authCapSystemAudit,
}

var authRoleCapabilities = map[string]authCapabilities{
	"admin": authCapAll,
	"documents_read": authCapDocumentsRead |
		authCapDocumentsWebDAVRead,
	"documents_editor": authCapDocumentsRead |
		authCapDocumentsWebDAVRead |
		authCapDocumentsUpload |
		authCapDocumentsEdit,
	"documents_manager": authCapDocumentsRead |
		authCapDocumentsWebDAVRead |
		authCapDocumentsUpload |
		authCapDocumentsEdit |
		authCapDocumentsDelete |
		authCapDocumentsStructure,
	"photos_read": authCapPhotosRead,
	"photos_editor": authCapPhotosRead |
		authCapPhotosEdit,
	"photos_manager": authCapPhotosRead |
		authCapPhotosEdit |
		authCapPhotosManage,
	"api_uploader": authCapDocumentsUpload,
}

type authState struct {
	enabled     bool
	realm       string
	credentials map[string]*authCredential
	cache       *authBasicCache
}

type authCredential struct {
	username     string
	password     string
	passwordHash []byte
	role         string
	capabilities authCapabilities
}

type authPrincipal struct {
	Username     string
	Role         string
	capabilities authCapabilities
}

type AuthPermissions struct {
	Authenticated         bool
	Username              string
	Role                  string
	CanDocumentsRead      bool
	CanDocumentsWebDAV    bool
	CanDocumentsUpload    bool
	CanDocumentsEdit      bool
	CanDocumentsDelete    bool
	CanDocumentsStructure bool
	CanPhotosRead         bool
	CanPhotosEdit         bool
	CanPhotosManage       bool
	CanSystemManage       bool
	CanSystemAudit        bool
}

type authBasicCache struct {
	mu      sync.Mutex
	entries map[[sha256.Size]byte]authBasicCacheEntry
}

type authBasicCacheEntry struct {
	principal authPrincipal
	expires   time.Time
}

func newAuthState(cfg config.AuthConfig) (*authState, error) {
	realm := cleanAuthRealm(cfg.Realm)
	state := &authState{
		realm:       realm,
		credentials: map[string]*authCredential{},
		cache:       &authBasicCache{entries: map[[sha256.Size]byte]authBasicCacheEntry{}},
	}
	if len(cfg.Credentials) > 0 {
		for i, item := range cfg.Credentials {
			credential, err := compileAuthCredential(item, fmt.Sprintf("auth credential %d", i+1))
			if err != nil {
				return nil, err
			}
			if _, exists := state.credentials[credential.username]; exists {
				return nil, fmt.Errorf("duplicate auth credential username %q", credential.username)
			}
			state.credentials[credential.username] = credential
		}
		state.enabled = true
		return state, nil
	}
	if cfg.Username == "" && cfg.Password == "" && cfg.PasswordHash == "" {
		return state, nil
	}
	credential, err := compileAuthCredential(config.AuthCredential{
		Username:     cfg.Username,
		Password:     cfg.Password,
		PasswordHash: cfg.PasswordHash,
		Role:         "admin",
	}, "auth")
	if err != nil {
		return nil, err
	}
	state.credentials[credential.username] = credential
	state.enabled = true
	return state, nil
}

func compileAuthCredential(item config.AuthCredential, label string) (*authCredential, error) {
	username := strings.TrimSpace(item.Username)
	if username == "" {
		return nil, fmt.Errorf("%s username is required", label)
	}
	if item.PasswordHash == "" && item.Password == "" {
		return nil, fmt.Errorf("%s password or password_hash is required", label)
	}
	if item.PasswordHash != "" {
		if _, err := bcrypt.Cost([]byte(item.PasswordHash)); err != nil {
			return nil, fmt.Errorf("invalid %s password hash: %w", label, err)
		}
	}
	capabilities, role, err := authCapabilitiesForConfig(item.Role, item.Permissions)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	return &authCredential{
		username:     username,
		password:     item.Password,
		passwordHash: []byte(item.PasswordHash),
		role:         role,
		capabilities: capabilities,
	}, nil
}

func authCapabilitiesForConfig(role string, permissions []string) (authCapabilities, string, error) {
	role = strings.TrimSpace(role)
	if role == "" && len(permissions) == 0 {
		role = "admin"
	}
	var capabilities authCapabilities
	if role != "" {
		roleCapabilities, ok := authRoleCapabilities[role]
		if !ok {
			return 0, "", fmt.Errorf("unknown auth role %q", role)
		}
		capabilities |= roleCapabilities
	}
	for _, permission := range permissions {
		permission = strings.TrimSpace(permission)
		if permission == "" {
			continue
		}
		permissionCapability, ok := authPermissionByName[permission]
		if !ok {
			return 0, "", fmt.Errorf("unknown auth permission %q", permission)
		}
		capabilities |= permissionCapability
	}
	if capabilities == 0 {
		return 0, "", errors.New("at least one auth permission is required")
	}
	if role == "" {
		role = "custom"
	}
	return capabilities, role, nil
}

func (s *Server) authEnabled() bool {
	if s == nil {
		return false
	}
	if s.auth == nil && s.cfg.Auth.Enabled() {
		auth, err := newAuthState(s.cfg.Auth)
		if err != nil {
			return false
		}
		s.auth = auth
	}
	return s.auth != nil && s.auth.enabled
}

func (s *Server) authenticateBasic(user, password string) (authPrincipal, bool) {
	if !s.authEnabled() {
		return authPrincipal{}, false
	}
	key := authBasicCacheKey(s.authKey, user, password)
	if principal, ok := s.auth.cache.get(key); ok {
		return principal, true
	}
	credential, ok := s.auth.credentials[user]
	if !ok || !credential.passwordOK(password) {
		return authPrincipal{}, false
	}
	principal := credential.principal()
	s.auth.cache.set(key, principal)
	return principal, true
}

func (c *authCredential) passwordOK(password string) bool {
	if c == nil {
		return false
	}
	if len(c.passwordHash) > 0 {
		return bcrypt.CompareHashAndPassword(c.passwordHash, []byte(password)) == nil
	}
	expected := sha256.Sum256([]byte(c.password))
	actual := sha256.Sum256([]byte(password))
	return subtle.ConstantTimeCompare(actual[:], expected[:]) == 1
}

func (c *authCredential) principal() authPrincipal {
	if c == nil {
		return authPrincipal{}
	}
	return authPrincipal{
		Username:     c.username,
		Role:         c.role,
		capabilities: c.capabilities,
	}
}

func (p authPrincipal) hasAll(capabilities authCapabilities) bool {
	return capabilities == 0 || p.capabilities&capabilities == capabilities
}

func (p authPrincipal) hasAny(capabilities authCapabilities) bool {
	return capabilities == 0 || p.capabilities&capabilities != 0
}

func (c *authBasicCache) get(key [sha256.Size]byte) (authPrincipal, bool) {
	if c == nil {
		return authPrincipal{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return authPrincipal{}, false
	}
	if time.Now().After(entry.expires) {
		delete(c.entries, key)
		return authPrincipal{}, false
	}
	return entry.principal, true
}

func (c *authBasicCache) set(key [sha256.Size]byte, principal authPrincipal) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for itemKey, entry := range c.entries {
		if now.After(entry.expires) {
			delete(c.entries, itemKey)
		}
	}
	for len(c.entries) >= authBasicCacheMaxEntries {
		for itemKey := range c.entries {
			delete(c.entries, itemKey)
			break
		}
	}
	c.entries[key] = authBasicCacheEntry{
		principal: principal,
		expires:   now.Add(authBasicCacheTTL),
	}
}

func authBasicCacheKey(key []byte, user, password string) [sha256.Size]byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(user))
	mac.Write([]byte{0})
	mac.Write([]byte(password))
	var sum [sha256.Size]byte
	copy(sum[:], mac.Sum(nil))
	return sum
}

func authPermissionsForRequest(s *Server, r *http.Request) AuthPermissions {
	if s == nil || !s.authEnabled() {
		return authPermissionsFromCapabilities(authCapAll, authPrincipal{})
	}
	principal, ok := authPrincipalFromContext(r.Context())
	if !ok {
		return AuthPermissions{}
	}
	return authPermissionsFromCapabilities(principal.capabilities, principal)
}

func authPermissionsFromCapabilities(capabilities authCapabilities, principal authPrincipal) AuthPermissions {
	return AuthPermissions{
		Authenticated:         capabilities != 0,
		Username:              principal.Username,
		Role:                  principal.Role,
		CanDocumentsRead:      capabilities&authCapDocumentsRead != 0,
		CanDocumentsWebDAV:    capabilities&authCapDocumentsWebDAVRead != 0,
		CanDocumentsUpload:    capabilities&authCapDocumentsUpload != 0,
		CanDocumentsEdit:      capabilities&authCapDocumentsEdit != 0,
		CanDocumentsDelete:    capabilities&authCapDocumentsDelete != 0,
		CanDocumentsStructure: capabilities&authCapDocumentsStructure != 0,
		CanPhotosRead:         capabilities&authCapPhotosRead != 0,
		CanPhotosEdit:         capabilities&authCapPhotosEdit != 0,
		CanPhotosManage:       capabilities&authCapPhotosManage != 0,
		CanSystemManage:       capabilities&authCapSystemManage != 0,
		CanSystemAudit:        capabilities&authCapSystemAudit != 0,
	}
}

func (s *Server) require(capabilities authCapabilities, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.requestHasCapabilities(r, capabilities) {
			next(w, r)
			return
		}
		s.renderForbidden(w, r)
	}
}

func (s *Server) requireAny(capabilities authCapabilities, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.requestHasAnyCapability(r, capabilities) {
			next(w, r)
			return
		}
		s.renderForbidden(w, r)
	}
}

func (s *Server) requestHasCapabilities(r *http.Request, capabilities authCapabilities) bool {
	if capabilities == 0 {
		return true
	}
	if !s.authEnabled() {
		return true
	}
	principal, ok := authPrincipalFromContext(r.Context())
	return ok && principal.hasAll(capabilities)
}

func (s *Server) requestHasAnyCapability(r *http.Request, capabilities authCapabilities) bool {
	if capabilities == 0 {
		return true
	}
	if !s.authEnabled() {
		return true
	}
	principal, ok := authPrincipalFromContext(r.Context())
	return ok && principal.hasAny(capabilities)
}

func (s *Server) renderForbidden(w http.ResponseWriter, r *http.Request) {
	if wantsJSON(r) || wantsJSONResponse(r) {
		s.renderJSONError(w, http.StatusForbidden, "Zugriff verweigert")
		return
	}
	returnURL := "/"
	if principal, ok := authPrincipalFromContext(r.Context()); ok {
		returnURL = defaultAuthLandingURL(principal)
	}
	w.WriteHeader(http.StatusForbidden)
	s.render(w, r, "error.html", PageData{
		Title:     "Fehler",
		Error:     publicErrorMessage(http.StatusForbidden, "Zugriff verweigert"),
		Active:    "",
		ReturnURL: returnURL,
	})
}
