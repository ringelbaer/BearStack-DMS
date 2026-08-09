// Datei prueft Berechtigungen und haelt den unveraenderlichen Authentifizierungs-Snapshot.
package server

import (
	"container/list"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"bearstack/internal/account"
	"bearstack/internal/config"
	"bearstack/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

const (
	authBasicCacheTTL         = 5 * time.Minute
	authBasicCacheMaxEntries  = 256
	authFailureLimit          = 5
	authFailureWindow         = 15 * time.Minute
	authFailureMaxEntries     = 4096
	authSnapshotReloadTimeout = 5 * time.Second
	authBcryptBusyRetryAfter  = time.Second

	authSourceConfig   = "config"
	authSourceDatabase = "database"
)

type authCapabilities = account.Capabilities

const (
	authCapDocumentsRead       = account.CapabilityDocumentsRead
	authCapDocumentsWebDAVRead = account.CapabilityDocumentsWebDAVRead
	authCapDocumentsUpload     = account.CapabilityDocumentsUpload
	authCapDocumentsEdit       = account.CapabilityDocumentsEdit
	authCapDocumentsDelete     = account.CapabilityDocumentsDelete
	authCapDocumentsStructure  = account.CapabilityDocumentsStructure
	authCapPhotosRead          = account.CapabilityPhotosRead
	authCapPhotosEdit          = account.CapabilityPhotosEdit
	authCapPhotosManage        = account.CapabilityPhotosManage
	authCapSystemManage        = account.CapabilitySystemManage
	authCapSystemAudit         = account.CapabilitySystemAudit
	authCapSystemUsersManage   = account.CapabilitySystemUsersManage
	authCapAll                 = account.AllCapabilities
)

// authState owns process-local security controls. Only snapshot is replaced;
// every published authSnapshot and all values reachable from it are immutable.
type authState struct {
	realm string

	snapshot atomic.Pointer[authSnapshot]
	cache    *authBasicCache
	limiter  *authFailureLimiter
	bcrypt   chan struct{}
}

type authSnapshot struct {
	enabled           bool
	recordCount       int
	activeCredentials int
	byUsername        map[string]*authCredential
	bySubject         map[string]*authCredential
}

type authCredential struct {
	username     string
	password     string
	passwordHash []byte
	role         string
	permissions  []string
	capabilities authCapabilities
	enabled      bool
	source       string
	subject      string
	revision     string
	accountID    int64
}

type authPrincipal struct {
	Username     string
	Role         string
	Source       string
	Subject      string
	Revision     string
	AccountID    int64
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
	CanSystemUsersManage  bool
}

// authAccountView deliberately contains no password material. It is used to
// merge read-only configuration accounts into the administration UI.
type authAccountView struct {
	Username     string
	Role         string
	Permissions  []string
	Capabilities authCapabilities
	Enabled      bool
	Source       string
	Subject      string
}

type authBasicCache struct {
	mu      sync.Mutex
	entries map[[sha256.Size]byte]authBasicCacheEntry
}

type authBasicCacheEntry struct {
	principal authPrincipal
	expires   time.Time
}

type authFailureLimiter struct {
	mu      sync.Mutex
	entries map[[sha256.Size]byte]*authFailureEntry
	lru     *list.List
	now     func() time.Time
}

type authFailureEntry struct {
	key         [sha256.Size]byte
	windowStart time.Time
	failures    int
	element     *list.Element
}

var (
	authDummyHashOnce     sync.Once
	authDummyHash         []byte
	authDummyHashErr      error
	errAuthPrincipalStale = errors.New("authentication principal is no longer current")
	errAuthBcryptBusy     = errors.New("password hashing capacity is busy")
)

func newAuthState(ctx context.Context, cfg config.AuthConfig, repo *repository.Repository, authKey []byte) (*authState, error) {
	snapshot, err := buildAuthSnapshot(ctx, cfg, repo, authKey)
	if err != nil {
		return nil, err
	}
	concurrency := min(4, runtime.GOMAXPROCS(0))
	if concurrency < 1 {
		concurrency = 1
	}
	state := &authState{
		realm:   cleanAuthRealm(cfg.Realm),
		cache:   &authBasicCache{entries: make(map[[sha256.Size]byte]authBasicCacheEntry)},
		limiter: newAuthFailureLimiter(),
		bcrypt:  make(chan struct{}, concurrency),
	}
	state.snapshot.Store(snapshot)
	if snapshot.enabled {
		if _, err := dummyPasswordHash(); err != nil {
			return nil, err
		}
	}
	return state, nil
}

func buildAuthSnapshot(ctx context.Context, cfg config.AuthConfig, repo *repository.Repository, authKey []byte) (*authSnapshot, error) {
	snapshot := &authSnapshot{
		byUsername: make(map[string]*authCredential),
		bySubject:  make(map[string]*authCredential),
	}
	configured, err := compileConfigCredentials(cfg, authKey)
	if err != nil {
		return nil, err
	}
	for _, credential := range configured {
		if err := addAuthCredential(snapshot, credential); err != nil {
			return nil, err
		}
	}

	if repo != nil {
		records, err := repo.ListAuthenticationAccounts(ctx)
		if err != nil {
			return nil, fmt.Errorf("load authentication accounts: %w", err)
		}
		snapshot.recordCount = len(records)
		for _, record := range records {
			credential, err := compileDatabaseCredential(record)
			if err != nil {
				return nil, err
			}
			if err := addAuthCredential(snapshot, credential); err != nil {
				return nil, err
			}
		}
	}

	snapshot.enabled = len(snapshot.byUsername) > 0 || snapshot.recordCount > 0
	return snapshot, nil
}

func compileConfigCredentials(cfg config.AuthConfig, authKey []byte) ([]*authCredential, error) {
	items := cfg.Credentials
	labels := make([]string, len(items))
	for i := range labels {
		labels[i] = fmt.Sprintf("auth credential %d", i+1)
	}
	if len(items) == 0 && (cfg.Username != "" || cfg.Password != "" || cfg.PasswordHash != "") {
		items = []config.AuthCredential{{
			Username:     cfg.Username,
			Password:     cfg.Password,
			PasswordHash: cfg.PasswordHash,
			Role:         account.RoleAdmin,
		}}
		labels = []string{"auth"}
	}
	credentials := make([]*authCredential, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for i, item := range items {
		credential, err := compileConfigCredential(item, labels[i], authKey)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[credential.username]; exists {
			return nil, fmt.Errorf("duplicate auth credential username %q", credential.username)
		}
		seen[credential.username] = struct{}{}
		credentials = append(credentials, credential)
	}
	return credentials, nil
}

func compileConfigCredential(item config.AuthCredential, label string, authKey []byte) (*authCredential, error) {
	username, err := account.NormalizeUsername(item.Username)
	if err != nil {
		return nil, fmt.Errorf("%s username is invalid: %w", label, err)
	}
	if item.PasswordHash == "" && item.Password == "" {
		return nil, fmt.Errorf("%s password or password_hash is required", label)
	}
	if item.PasswordHash != "" {
		if err := account.ValidatePasswordHash(item.PasswordHash); err != nil {
			return nil, fmt.Errorf("invalid %s password hash: %w", label, err)
		}
	}
	capabilities, role, err := authCapabilitiesForConfig(item.Role, item.Permissions)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	permissions := normalizedPermissionNames(item.Permissions)
	credential := &authCredential{
		username:     username,
		password:     item.Password,
		passwordHash: []byte(item.PasswordHash),
		role:         role,
		permissions:  permissions,
		capabilities: capabilities,
		enabled:      true,
		source:       authSourceConfig,
		subject:      username,
	}
	credential.revision, err = configCredentialRevision(authKey, credential, item.Password, item.PasswordHash)
	if err != nil {
		return nil, err
	}
	return credential, nil
}

func compileDatabaseCredential(record account.AuthenticationRecord) (*authCredential, error) {
	username, err := account.NormalizeUsername(record.Username)
	if err != nil {
		return nil, fmt.Errorf("invalid database account %d username: %w", record.ID, err)
	}
	if record.ID <= 0 {
		return nil, fmt.Errorf("invalid database account %q id", username)
	}
	if err := account.ValidatePasswordHash(record.PasswordHash); err != nil {
		return nil, fmt.Errorf("invalid database account %q password hash: %w", username, err)
	}
	capabilities, err := account.CapabilitiesFor(record.Role, record.Permissions)
	if err != nil {
		return nil, fmt.Errorf("invalid database account %q access: %w", username, err)
	}
	if record.SessionVersion < 1 {
		return nil, fmt.Errorf("invalid database account %q session version", username)
	}
	return &authCredential{
		username:     username,
		passwordHash: []byte(record.PasswordHash),
		role:         strings.TrimSpace(record.Role),
		permissions:  normalizedPermissionNames(record.Permissions),
		capabilities: capabilities,
		enabled:      record.Enabled,
		source:       authSourceDatabase,
		subject:      strconv.FormatInt(record.ID, 10),
		revision:     strconv.FormatInt(record.SessionVersion, 10),
		accountID:    record.ID,
	}, nil
}

func addAuthCredential(snapshot *authSnapshot, credential *authCredential) error {
	if previous, exists := snapshot.byUsername[credential.username]; exists {
		return fmt.Errorf("authentication username %q is defined by both %s and %s accounts", credential.username, previous.source, credential.source)
	}
	subjectKey := authSubjectKey(credential.source, credential.subject)
	if _, exists := snapshot.bySubject[subjectKey]; exists {
		return fmt.Errorf("duplicate %s authentication subject %q", credential.source, credential.subject)
	}
	snapshot.byUsername[credential.username] = credential
	snapshot.bySubject[subjectKey] = credential
	if credential.enabled {
		snapshot.activeCredentials++
	}
	return nil
}

func authCapabilitiesForConfig(role string, permissions []string) (authCapabilities, string, error) {
	capabilities, normalizedRole, err := account.ConfigCapabilitiesFor(role, permissions)
	if err == nil {
		return capabilities, normalizedRole, nil
	}
	message := strings.ReplaceAll(err.Error(), "account role", "auth role")
	message = strings.ReplaceAll(message, "account permission", "auth permission")
	return 0, "", errors.New(message)
}

func configCredentialRevision(key []byte, credential *authCredential, password, passwordHash string) (string, error) {
	material := struct {
		Username     string   `json:"username"`
		Password     string   `json:"password,omitempty"`
		PasswordHash string   `json:"password_hash,omitempty"`
		Role         string   `json:"role"`
		Permissions  []string `json:"permissions,omitempty"`
	}{credential.username, password, passwordHash, credential.role, credential.permissions}
	encoded, err := json.Marshal(material)
	if err != nil {
		return "", fmt.Errorf("encode auth credential revision: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(encoded)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func normalizedPermissionNames(values []string) []string {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			unique[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func authSubjectKey(source, subject string) string { return source + "\x00" + subject }

func (s *Server) ensureAuthState() *authState {
	if s == nil {
		return nil
	}
	s.authInitMu.Lock()
	defer s.authInitMu.Unlock()
	if s.auth != nil {
		return s.auth
	}
	state, err := newAuthState(context.Background(), s.cfg.Auth, s.repo, s.authKey)
	if err != nil {
		if s.log != nil {
			s.log.Error("initialize authentication", "error", err)
		}
		return nil
	}
	s.auth = state
	return state
}

func (s *Server) authSnapshot() *authSnapshot {
	state := s.ensureAuthState()
	if state == nil {
		return nil
	}
	return state.snapshot.Load()
}

func (s *Server) authEnabled() bool {
	snapshot := s.authSnapshot()
	return snapshot != nil && snapshot.enabled
}

// AuthEnabled reports the effective hybrid configuration/database state.
func (s *Server) AuthEnabled() bool { return s.authEnabled() }

// ReloadAuthSnapshot publishes committed account changes atomically and clears
// all positive Basic-Auth entries. Requests never observe a partial update.
func (s *Server) ReloadAuthSnapshot(ctx context.Context) error {
	state := s.ensureAuthState()
	if state == nil {
		return errors.New("authentication state is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// A user mutation is already committed when handlers reach this method.
	// Client cancellation must not leave the process authenticating a stale
	// snapshot, so detach cancellation but retain a strict internal deadline.
	reloadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), authSnapshotReloadTimeout)
	defer cancel()
	snapshot, err := buildAuthSnapshot(reloadCtx, s.cfg.Auth, s.repo, s.authKey)
	if err != nil {
		s.publishAuthFailClosed(state, err)
		return err
	}
	if snapshot.enabled {
		if _, err := dummyPasswordHash(); err != nil {
			s.publishAuthFailClosed(state, err)
			return err
		}
	}
	if config.AddrRequiresAuth(s.cfg.Addr) && snapshot.activeCredentials == 0 {
		err := errors.New("at least one active authentication account is required when addr listens on non-loopback interfaces")
		s.publishAuthFailClosed(state, err)
		return err
	}
	state.cache.clear()
	state.snapshot.Store(snapshot)
	return nil
}

func (s *Server) publishAuthFailClosed(state *authState, cause error) {
	if state == nil {
		return
	}
	state.cache.clear()
	state.snapshot.Store(&authSnapshot{
		enabled:    true,
		byUsername: make(map[string]*authCredential),
		bySubject:  make(map[string]*authCredential),
	})
	if s != nil && s.log != nil {
		s.log.Error("authentication snapshot reload failed; authentication is fail-closed", "error", cause)
	}
}

// reloadAuthSnapshot keeps package-local handlers concise while the exported
// form is available to application wiring and focused integration tests.
func (s *Server) reloadAuthSnapshot(ctx context.Context) error {
	return s.ReloadAuthSnapshot(ctx)
}

func (s *Server) withAuthWrite(ctx context.Context, change func() error) error {
	if s == nil || change == nil {
		return errors.New("authentication write is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.authWriteMu.Lock()
	defer s.authWriteMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot := s.authSnapshot()
	principal, authenticated := authPrincipalFromContext(ctx)
	if !authenticated {
		// The only unauthenticated auth write is the first loopback bootstrap.
		// Rechecking after the mutex prevents two concurrent first-admin creates.
		if snapshot == nil || snapshot.enabled {
			return errAuthPrincipalStale
		}
	} else {
		if snapshot == nil || principal.Source == "" || principal.Subject == "" || principal.Revision == "" {
			return errAuthPrincipalStale
		}
		credential := snapshot.bySubject[authSubjectKey(principal.Source, principal.Subject)]
		if credential == nil || !credential.enabled || credential.username != principal.Username || credential.revision != principal.Revision {
			return errAuthPrincipalStale
		}
	}
	return change()
}

func (s *Server) authenticateBasic(user, password string) (authPrincipal, bool) {
	principal, ok, _ := s.authenticateBasicCheck(user, password)
	return principal, ok
}

// authenticateBasicCheck returns a positive retry duration when the exact
// entered username is currently blocked.
func (s *Server) authenticateBasicCheck(user, password string) (authPrincipal, bool, time.Duration) {
	state := s.ensureAuthState()
	if state == nil {
		return authPrincipal{}, false, 0
	}
	snapshot := state.snapshot.Load()
	if snapshot == nil || !snapshot.enabled {
		return authPrincipal{}, false, 0
	}
	if retryAfter := state.limiter.retryAfter(user); retryAfter > 0 {
		return authPrincipal{}, false, retryAfter
	}
	credential, found := snapshot.byUsername[user]
	if found && credential.enabled {
		key := authBasicCacheKey(s.authKey, user, password, credential.source, credential.subject, credential.revision)
		if principal, ok := state.cache.get(key); ok {
			if !authCredentialCurrent(state.snapshot.Load(), credential) {
				return authPrincipal{}, false, 0
			}
			state.limiter.success(user)
			return principal, true, 0
		}
		passwordMatches, busy := state.passwordOK(credential, password)
		if busy {
			return authPrincipal{}, false, authBcryptBusyRetryAfter
		}
		if passwordMatches {
			if !authCredentialCurrent(state.snapshot.Load(), credential) {
				return authPrincipal{}, false, 0
			}
			principal := credential.principal()
			state.cache.set(key, principal)
			state.limiter.success(user)
			return principal, true, 0
		}
	} else {
		if busy := state.compareDummy(password); busy {
			return authPrincipal{}, false, authBcryptBusyRetryAfter
		}
	}
	state.limiter.failure(user)
	return authPrincipal{}, false, 0
}

func authCredentialCurrent(snapshot *authSnapshot, credential *authCredential) bool {
	if snapshot == nil || credential == nil {
		return false
	}
	current := snapshot.bySubject[authSubjectKey(credential.source, credential.subject)]
	return current != nil && current.enabled && current.username == credential.username && current.revision == credential.revision
}

func (s *Server) authPasswordCheck(r *http.Request, password string) (bool, time.Duration) {
	principal, ok := authPrincipalFromContext(r.Context())
	if !ok {
		return false, 0
	}
	verified, authenticated, retryAfter := s.authenticateBasicCheck(principal.Username, password)
	if !authenticated {
		return false, retryAfter
	}
	return verified.Username == principal.Username &&
		verified.Source == principal.Source &&
		verified.Subject == principal.Subject &&
		verified.Revision == principal.Revision, 0
}

func (state *authState) passwordOK(credential *authCredential, password string) (matches, busy bool) {
	if credential == nil {
		return false, state.compareDummy(password)
	}
	if len(credential.passwordHash) > 0 {
		return state.compareBcrypt(credential.passwordHash, password)
	}
	// Clear-text configuration remains backwards compatible. A dummy bcrypt
	// comparison keeps its timing in the same class as unknown accounts.
	if state.compareDummy(password) {
		return false, true
	}
	expected := sha256.Sum256([]byte(credential.password))
	actual := sha256.Sum256([]byte(password))
	return subtle.ConstantTimeCompare(actual[:], expected[:]) == 1, false
}

func (state *authState) compareDummy(password string) (busy bool) {
	hash, err := dummyPasswordHash()
	if err != nil {
		return false
	}
	_, busy = state.compareBcrypt(hash, password)
	return busy
}

func (state *authState) compareBcrypt(hash []byte, password string) (matches, busy bool) {
	select {
	case state.bcrypt <- struct{}{}:
	default:
		return false, true
	}
	err := bcrypt.CompareHashAndPassword(hash, []byte(password))
	<-state.bcrypt
	return err == nil, false
}

func (s *Server) hashAuthPassword(ctx context.Context, password string) (string, error) {
	state := s.ensureAuthState()
	if state == nil {
		return "", errors.New("authentication state is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	select {
	case state.bcrypt <- struct{}{}:
	default:
		return "", errAuthBcryptBusy
	}
	defer func() { <-state.bcrypt }()
	hash, err := account.HashPassword(password)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return hash, nil
}

func dummyPasswordHash() ([]byte, error) {
	authDummyHashOnce.Do(func() {
		authDummyHash, authDummyHashErr = bcrypt.GenerateFromPassword([]byte("bearstack-dummy-password"), account.PasswordHashCost)
		if authDummyHashErr != nil {
			authDummyHashErr = fmt.Errorf("create dummy password hash: %w", authDummyHashErr)
		}
	})
	return authDummyHash, authDummyHashErr
}

func (credential *authCredential) principal() authPrincipal {
	if credential == nil || !credential.enabled {
		return authPrincipal{}
	}
	return authPrincipal{
		Username:     credential.username,
		Role:         credential.role,
		Source:       credential.source,
		Subject:      credential.subject,
		Revision:     credential.revision,
		AccountID:    credential.accountID,
		capabilities: credential.capabilities,
	}
}

func (p authPrincipal) hasAll(capabilities authCapabilities) bool {
	return p.capabilities.HasAll(capabilities)
}
func (p authPrincipal) hasAny(capabilities authCapabilities) bool {
	return p.capabilities.HasAny(capabilities)
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
	c.entries[key] = authBasicCacheEntry{principal: principal, expires: now.Add(authBasicCacheTTL)}
}

func (c *authBasicCache) clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entries = make(map[[sha256.Size]byte]authBasicCacheEntry)
	c.mu.Unlock()
}

func authBasicCacheKey(key []byte, values ...string) [sha256.Size]byte {
	mac := hmac.New(sha256.New, key)
	for _, value := range values {
		_, _ = mac.Write([]byte(value))
		_, _ = mac.Write([]byte{0})
	}
	var sum [sha256.Size]byte
	copy(sum[:], mac.Sum(nil))
	return sum
}

func newAuthFailureLimiter() *authFailureLimiter {
	return &authFailureLimiter{entries: make(map[[sha256.Size]byte]*authFailureEntry), lru: list.New(), now: time.Now}
}

func (limiter *authFailureLimiter) retryAfter(username string) time.Duration {
	if limiter == nil {
		return 0
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	now := limiter.now()
	key := sha256.Sum256([]byte(username))
	entry := limiter.entries[key]
	if entry != nil && !now.Before(entry.windowStart.Add(authFailureWindow)) {
		limiter.remove(entry)
		entry = nil
	}
	if entry == nil || entry.failures < authFailureLimit {
		return 0
	}
	limiter.lru.MoveToFront(entry.element)
	remaining := entry.windowStart.Add(authFailureWindow).Sub(now)
	if remaining <= 0 {
		limiter.remove(entry)
		return 0
	}
	return remaining
}

func (limiter *authFailureLimiter) failure(username string) {
	if limiter == nil {
		return
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	now := limiter.now()
	key := sha256.Sum256([]byte(username))
	entry := limiter.entries[key]
	if entry != nil && !now.Before(entry.windowStart.Add(authFailureWindow)) {
		limiter.remove(entry)
		entry = nil
	}
	if entry == nil {
		for len(limiter.entries) >= authFailureMaxEntries {
			oldest, _ := limiter.lru.Back().Value.(*authFailureEntry)
			limiter.remove(oldest)
		}
		entry = &authFailureEntry{key: key, windowStart: now}
		entry.element = limiter.lru.PushFront(entry)
		limiter.entries[key] = entry
	} else {
		limiter.lru.MoveToFront(entry.element)
	}
	entry.failures++
}

func (limiter *authFailureLimiter) success(username string) {
	if limiter == nil {
		return
	}
	limiter.mu.Lock()
	key := sha256.Sum256([]byte(username))
	if entry := limiter.entries[key]; entry != nil {
		limiter.remove(entry)
	}
	limiter.mu.Unlock()
}

func (limiter *authFailureLimiter) remove(entry *authFailureEntry) {
	if entry == nil {
		return
	}
	delete(limiter.entries, entry.key)
	if entry.element != nil {
		limiter.lru.Remove(entry.element)
		entry.element = nil
	}
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
		CanDocumentsRead:      capabilities.HasAll(authCapDocumentsRead),
		CanDocumentsWebDAV:    capabilities.HasAll(authCapDocumentsWebDAVRead),
		CanDocumentsUpload:    capabilities.HasAll(authCapDocumentsUpload),
		CanDocumentsEdit:      capabilities.HasAll(authCapDocumentsEdit),
		CanDocumentsDelete:    capabilities.HasAll(authCapDocumentsDelete),
		CanDocumentsStructure: capabilities.HasAll(authCapDocumentsStructure),
		CanPhotosRead:         capabilities.HasAll(authCapPhotosRead),
		CanPhotosEdit:         capabilities.HasAll(authCapPhotosEdit),
		CanPhotosManage:       capabilities.HasAll(authCapPhotosManage),
		CanSystemManage:       capabilities.HasAll(authCapSystemManage),
		CanSystemAudit:        capabilities.HasAll(authCapSystemAudit),
		CanSystemUsersManage:  capabilities.HasAll(authCapSystemUsersManage),
	}
}

func (s *Server) authConfigAccountViews() []authAccountView {
	snapshot := s.authSnapshot()
	if snapshot == nil {
		return nil
	}
	result := make([]authAccountView, 0)
	for _, credential := range snapshot.byUsername {
		if credential.source != authSourceConfig {
			continue
		}
		result = append(result, authAccountView{
			Username: credential.username, Role: credential.role,
			Permissions:  append([]string(nil), credential.permissions...),
			Capabilities: credential.capabilities, Enabled: credential.enabled,
			Source: credential.source, Subject: credential.subject,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Username < result[j].Username })
	return result
}

func (s *Server) configActiveUserManagerCount() int {
	count := 0
	for _, item := range s.authConfigAccountViews() {
		if item.Enabled && item.Capabilities.HasAll(authCapSystemUsersManage) {
			count++
		}
	}
	return count
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
	if capabilities == 0 || !s.authEnabled() {
		return true
	}
	principal, ok := authPrincipalFromContext(r.Context())
	return ok && principal.hasAll(capabilities)
}

func (s *Server) requestHasAnyCapability(r *http.Request, capabilities authCapabilities) bool {
	if capabilities == 0 || !s.authEnabled() {
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
	s.render(w, r, "error.html", PageData{Title: "Fehler", Error: publicErrorMessage(http.StatusForbidden, "Zugriff verweigert"), ReturnURL: returnURL})
}
