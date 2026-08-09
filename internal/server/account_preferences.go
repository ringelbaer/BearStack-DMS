// Datei haelt UI-Praeferenzen aller Kontoquellen performant im Arbeitsspeicher.
package server

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"

	"bearstack/internal/account"
	"bearstack/internal/repository"
)

type accountPreferenceSnapshot struct {
	byAccount map[string]repository.AccountPreference
}

type accountPreferenceState struct {
	snapshot atomic.Pointer[accountPreferenceSnapshot]
}

func newAccountPreferenceState(ctx context.Context, repo *repository.Repository) (*accountPreferenceState, error) {
	state := &accountPreferenceState{}
	if repo == nil {
		state.snapshot.Store(&accountPreferenceSnapshot{byAccount: map[string]repository.AccountPreference{}})
		return state, nil
	}
	preferences, err := repo.ListAccountPreferences(ctx)
	if err != nil {
		return nil, err
	}
	snapshot := &accountPreferenceSnapshot{byAccount: make(map[string]repository.AccountPreference, len(preferences))}
	for _, preference := range preferences {
		snapshot.byAccount[accountPreferenceKey(preference.Source, preference.Subject)] = preference
	}
	state.snapshot.Store(snapshot)
	return state, nil
}

func accountPreferenceKey(source, subject string) string {
	return strings.TrimSpace(source) + "\x00" + strings.TrimSpace(subject)
}

func (s *Server) accountPreference(source, subject string) repository.AccountPreference {
	preference := repository.AccountPreference{Source: source, Subject: subject}
	if s == nil || s.preferences == nil {
		return preference
	}
	snapshot := s.preferences.snapshot.Load()
	if snapshot == nil {
		return preference
	}
	if stored, ok := snapshot.byAccount[accountPreferenceKey(source, subject)]; ok {
		return stored
	}
	return preference
}

func (s *Server) saveAccountPreference(ctx context.Context, params repository.SaveAccountPreferenceParams) (repository.AccountPreference, error) {
	s.preferenceWriteMu.Lock()
	defer s.preferenceWriteMu.Unlock()
	current := s.accountPreference(params.Source, params.Subject)
	if current.RowVersion != params.ExpectedRowVersion {
		return repository.AccountPreference{}, repository.ErrAccountPreferenceConflict
	}
	if params.Source == authSourceDatabase {
		id, err := strconv.ParseInt(params.Subject, 10, 64)
		if err != nil || id < 1 {
			return repository.AccountPreference{}, sql.ErrNoRows
		}
		if _, err := s.repo.UserByID(ctx, id); err != nil {
			return repository.AccountPreference{}, err
		}
	}
	updated, err := s.repo.SaveAccountPreference(ctx, params)
	if err != nil {
		return repository.AccountPreference{}, err
	}
	previous := s.preferences.snapshot.Load()
	next := &accountPreferenceSnapshot{byAccount: make(map[string]repository.AccountPreference, len(previous.byAccount)+1)}
	for key, preference := range previous.byAccount {
		next.byAccount[key] = preference
	}
	next.byAccount[accountPreferenceKey(updated.Source, updated.Subject)] = updated
	s.preferences.snapshot.Store(next)
	return updated, nil
}

// removeAccountPreferenceSnapshotLocked muss unter preferenceWriteMu laufen.
func (s *Server) removeAccountPreferenceSnapshotLocked(source, subject string) {
	previous := s.preferences.snapshot.Load()
	next := &accountPreferenceSnapshot{byAccount: make(map[string]repository.AccountPreference, len(previous.byAccount))}
	removedKey := accountPreferenceKey(source, subject)
	for key, preference := range previous.byAccount {
		if key != removedKey {
			next.byAccount[key] = preference
		}
	}
	s.preferences.snapshot.Store(next)
}

func (s *Server) customPDFPreviewForRequest(rPrincipal authPrincipal) bool {
	if rPrincipal.Source == "" || rPrincipal.Subject == "" {
		return false
	}
	return s.accountPreference(rPrincipal.Source, rPrincipal.Subject).CustomPDFPreviewEnabled
}

func (s *Server) preferenceTarget(ctx context.Context, source, subject string) (string, account.User, authAccountView, error) {
	source = strings.TrimSpace(source)
	subject = strings.TrimSpace(subject)
	switch source {
	case authSourceDatabase:
		id, err := strconv.ParseInt(subject, 10, 64)
		if err != nil || id < 1 || strconv.FormatInt(id, 10) != subject {
			return "", account.User{}, authAccountView{}, errors.New("ungueltiges Datenbankkonto")
		}
		user, err := s.repo.UserByID(ctx, id)
		return user.Username, user, authAccountView{}, err
	case authSourceConfig:
		for _, configured := range s.authConfigAccountViews() {
			if configured.Subject == subject {
				return configured.Username, account.User{}, configured, nil
			}
		}
		return "", account.User{}, authAccountView{}, errors.New("Konfigurationskonto nicht gefunden")
	default:
		return "", account.User{}, authAccountView{}, errors.New("ungueltige Kontoquelle")
	}
}

func actorCanManageConfigPreference(actor authPrincipal, target authAccountView) bool {
	if actor.Username == "" {
		return false
	}
	if actor.Source == target.Source && actor.Subject == target.Subject {
		return true
	}
	if account.IsAdministratorRole(actor.Role) {
		return true
	}
	if account.IsAdministratorRole(target.Role) || target.Capabilities.HasAll(authCapSystemUsersManage) {
		return false
	}
	return target.Capabilities&^actor.capabilities == 0
}
