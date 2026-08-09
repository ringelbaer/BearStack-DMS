// Datei implementiert die servergerenderte Nutzerverwaltung und den Konto-Selbstservice.
package server

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"bearstack/internal/account"
	"bearstack/internal/repository"
)

const (
	userFormActionAccess   = "access"
	userFormActionPassword = "password"
	userFormActionStatus   = "status"
	userFormActionDelete   = "delete"
)

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.repo.ListUsers(r.Context())
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	principal, _ := authPrincipalFromContext(r.Context())
	view := s.userManagementListView(users, principal)
	view.Bootstrap = s.userBootstrapAllowed(r, users)
	view.CanCreate = view.Bootstrap || view.CanCreate
	s.render(w, r, "users.html", PageData{
		Title:          "Nutzerverwaltung",
		Active:         "settings",
		SettingsTab:    "users",
		Notice:         r.URL.Query().Get("notice"),
		UserManagement: view,
	})
}

func (s *Server) handleNewUser(w http.ResponseWriter, r *http.Request) {
	users, err := s.repo.ListUsers(r.Context())
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	bootstrap := s.userBootstrapAllowed(r, users)
	principal, hasPrincipal := authPrincipalFromContext(r.Context())
	if !bootstrap && !hasPrincipal {
		s.renderForbidden(w, r)
		return
	}
	if !bootstrap && !actorCanCreateUser(principal) {
		s.renderForbidden(w, r)
		return
	}
	role := defaultAssignableUserRole(principal, bootstrap)
	form := ManagedUserFormView{
		Role:                role,
		Active:              true,
		Editable:            true,
		CanEditAccess:       true,
		SelectedPermissions: map[string]bool{},
		Action:              userFormActionAccess,
	}
	s.renderUserForm(w, r, http.StatusOK, form, true, bootstrap, "")
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderError(w, r, http.StatusBadRequest, errors.New("Ungültige Formulardaten."))
		return
	}
	users, err := s.repo.ListUsers(r.Context())
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	bootstrap := s.userBootstrapAllowed(r, users)
	principal, hasPrincipal := authPrincipalFromContext(r.Context())
	if !bootstrap && (!hasPrincipal || !actorCanCreateUser(principal)) {
		s.renderForbidden(w, r)
		return
	}

	form := managedUserFormFromRequest(r, true)
	form.Action = userFormActionAccess
	fieldErrors := map[string]string{}
	username, usernameErr := account.NormalizeUsername(form.Username)
	if usernameErr != nil {
		fieldErrors["username"] = friendlyUsernameError(usernameErr)
	} else {
		form.Username = username
		setAuditTarget(r, "Benutzer:"+username)
	}
	selectedPermissions := additionalPermissionsForRole(form.Role, selectedPermissionNames(form.SelectedPermissions))
	role, permissions, _, accessErr := account.NormalizeAccess(form.Role, selectedPermissions)
	if bootstrap {
		role = account.RoleAdmin
		permissions = nil
		accessErr = nil
		form.Role = role
		form.SelectedPermissions = map[string]bool{}
	} else if accessErr != nil {
		fieldErrors["permissions"] = friendlyAccessError(accessErr)
	} else if err := validateDelegatedAccess(principal, role, permissions); err != nil {
		fieldErrors["permissions"] = err.Error()
	}
	password := r.FormValue("new_password")
	passwordConfirmation := r.FormValue("new_password_confirmation")
	if password != passwordConfirmation {
		fieldErrors["new_password_confirmation"] = "Die Passwörter stimmen nicht überein."
	}
	if err := account.ValidatePassword(password); err != nil {
		fieldErrors["new_password"] = friendlyPasswordError(err)
	}
	confirmationStatus := 0
	if !bootstrap {
		confirmationStatus = s.currentPasswordFormStatus(w, r, fieldErrors)
	}
	if usernameErr == nil && s.configUsernameExists(username) {
		fieldErrors["username"] = "Dieser Benutzername wird bereits durch die Konfiguration verwendet."
	}
	if len(fieldErrors) > 0 {
		form.FieldErrors = fieldErrors
		status := http.StatusBadRequest
		if confirmationStatus != 0 {
			status = confirmationStatus
		}
		s.renderUserForm(w, r, status, form, true, bootstrap, "Bitte korrigieren Sie die markierten Felder.")
		return
	}

	passwordHash, err := s.hashAuthPassword(r.Context(), password)
	if err != nil {
		if errors.Is(err, errAuthBcryptBusy) {
			setAuthBusyRetryAfter(w)
			s.renderUserForm(w, r, http.StatusTooManyRequests, form, true, bootstrap, "Die Passwortverarbeitung ist ausgelastet. Bitte versuchen Sie es gleich erneut.")
			return
		}
		s.renderUserForm(w, r, http.StatusBadRequest, form, true, bootstrap, friendlyPasswordError(err))
		return
	}
	params := account.CreateUserParams{
		Username:     username,
		PasswordHash: passwordHash,
		Role:         role,
		Permissions:  permissions,
		Enabled:      true,
	}
	var created account.User
	err = s.withAuthWrite(r.Context(), func() error {
		var createErr error
		created, createErr = s.repo.CreateUser(r.Context(), params)
		if createErr != nil {
			return createErr
		}
		if reloadErr := s.reloadAuthSnapshot(r.Context()); reloadErr != nil {
			return reloadErr
		}
		if bootstrap {
			createdPrincipal, current := s.authPrincipalForDatabaseRevision(created.ID, created.Username, created.SessionVersion)
			if !current || !s.setAuthSessionForPrincipal(w, r, createdPrincipal, authSessionDuration) {
				return errAuthPrincipalStale
			}
		}
		return nil
	})
	setAuditTarget(r, "Benutzer:"+username)
	if err != nil {
		if errors.Is(err, repository.ErrUserAlreadyExists) {
			form.FieldErrors = map[string]string{"username": "Dieser Benutzername ist bereits vergeben."}
			s.renderUserForm(w, r, http.StatusBadRequest, form, true, bootstrap, "Bitte wählen Sie einen anderen Benutzernamen.")
			return
		}
		s.renderUserMutationError(w, r, err, "/settings/users")
		return
	}
	if bootstrap {
		setAuditActor(r, created.Username)
	}
	redirectWithNotice(w, r, "/settings/users", "Nutzer "+created.Username+" wurde angelegt.")
}

func (s *Server) handleEditUser(w http.ResponseWriter, r *http.Request) {
	user, principal, ok := s.editableUserFromRequest(w, r)
	if !ok {
		return
	}
	form := managedUserFormFromAccount(user)
	form.Action = userFormActionAccess
	s.renderUserForm(w, r, http.StatusOK, form, false, false, "")
	_ = principal
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	user, principal, ok := s.editableUserFromRequest(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("Ungültige Formulardaten."), userEditURL(user.ID))
		return
	}
	form := managedUserFormFromRequest(r, false)
	form.ID = user.ID
	form.Username = user.Username
	form.Active = user.Enabled
	form.Editable = true
	form.CanEditAccess = true
	form.CanResetPassword = true
	form.CanChangeStatus = true
	form.CanDelete = true
	form.Action = userFormActionAccess
	fieldErrors := map[string]string{}
	if form.Version < 1 {
		fieldErrors["version"] = "Das Formular ist veraltet. Bitte laden Sie die Seite neu."
	}
	selectedPermissions := additionalPermissionsForRole(form.Role, selectedPermissionNames(form.SelectedPermissions))
	role, permissions, _, err := account.NormalizeAccess(form.Role, selectedPermissions)
	if err != nil {
		fieldErrors["permissions"] = friendlyAccessError(err)
	} else if err := validateDelegatedAccess(principal, role, permissions); err != nil {
		fieldErrors["permissions"] = err.Error()
	}
	confirmationStatus := s.currentPasswordFormStatus(w, r, fieldErrors)
	if len(fieldErrors) > 0 {
		form.FieldErrors = fieldErrors
		status := http.StatusBadRequest
		errorText := "Bitte korrigieren Sie die markierten Felder."
		if confirmationStatus != 0 {
			status = confirmationStatus
		} else if form.Version < 1 {
			status = http.StatusConflict
			errorText = fieldErrors["version"]
		}
		s.renderUserForm(w, r, status, form, false, false, errorText)
		return
	}

	externalManagers := s.configActiveUserManagerCount()
	err = s.withAuthWrite(r.Context(), func() error {
		_, updateErr := s.repo.UpdateUserAccess(r.Context(), user.ID, account.UpdateUserAccessParams{
			Role:               role,
			Permissions:        permissions,
			ExpectedRowVersion: form.Version,
		}, externalManagers)
		if updateErr != nil {
			return updateErr
		}
		return s.reloadAuthSnapshot(r.Context())
	})
	setAuditTarget(r, "Benutzer:"+user.Username)
	if err != nil {
		if errors.Is(err, repository.ErrUserConflict) || errors.Is(err, repository.ErrLastActiveUserManager) {
			form.FieldErrors = map[string]string{"version": friendlyUserMutationError(err)}
			s.renderUserForm(w, r, http.StatusConflict, form, false, false, friendlyUserMutationError(err))
			return
		}
		s.renderUserMutationError(w, r, err, userEditURL(user.ID))
		return
	}
	redirectWithNotice(w, r, userEditURL(user.ID), "Zugriffsrechte wurden gespeichert. Bestehende Sitzungen des Kontos wurden beendet.")
}

func (s *Server) handleResetUserPassword(w http.ResponseWriter, r *http.Request) {
	user, _, ok := s.editableUserFromRequest(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("Ungültige Formulardaten."), userEditURL(user.ID))
		return
	}
	form := managedUserFormFromAccount(user)
	form.Action = userFormActionPassword
	form.Version = formVersionFromRequest(r)
	fieldErrors := map[string]string{}
	password := r.FormValue("new_password")
	if err := account.ValidatePassword(password); err != nil {
		fieldErrors["new_password"] = friendlyPasswordError(err)
	}
	if password != r.FormValue("new_password_confirmation") {
		fieldErrors["new_password_confirmation"] = "Die Passwörter stimmen nicht überein."
	}
	confirmationStatus := s.currentPasswordFormStatus(w, r, fieldErrors)
	if form.Version < 1 {
		fieldErrors["version"] = "Das Formular ist veraltet. Bitte laden Sie die Seite neu."
	}
	if len(fieldErrors) > 0 {
		form.FieldErrors = fieldErrors
		status := http.StatusBadRequest
		errorText := "Bitte korrigieren Sie die markierten Felder."
		if confirmationStatus != 0 {
			status = confirmationStatus
		} else if form.Version < 1 {
			status = http.StatusConflict
			errorText = fieldErrors["version"]
		}
		s.renderUserForm(w, r, status, form, false, false, errorText)
		return
	}
	passwordHash, err := s.hashAuthPassword(r.Context(), password)
	if err != nil {
		if errors.Is(err, errAuthBcryptBusy) {
			setAuthBusyRetryAfter(w)
			s.renderUserForm(w, r, http.StatusTooManyRequests, form, false, false, "Die Passwortverarbeitung ist ausgelastet. Bitte versuchen Sie es gleich erneut.")
			return
		}
		s.renderUserForm(w, r, http.StatusBadRequest, form, false, false, friendlyPasswordError(err))
		return
	}
	err = s.withAuthWrite(r.Context(), func() error {
		_, updateErr := s.repo.UpdateUserPassword(r.Context(), user.ID, account.UpdateUserPasswordParams{
			PasswordHash:       passwordHash,
			ExpectedRowVersion: form.Version,
		})
		if updateErr != nil {
			return updateErr
		}
		return s.reloadAuthSnapshot(r.Context())
	})
	setAuditTarget(r, "Benutzer:"+user.Username)
	if err != nil {
		s.renderUserMutationError(w, r, err, userEditURL(user.ID))
		return
	}
	redirectWithNotice(w, r, userEditURL(user.ID), "Passwort wurde zurückgesetzt. Bestehende Sitzungen des Kontos wurden beendet.")
}

func (s *Server) handleEnableUser(w http.ResponseWriter, r *http.Request) {
	s.handleSetUserEnabled(w, r, true)
}

func (s *Server) handleDisableUser(w http.ResponseWriter, r *http.Request) {
	s.handleSetUserEnabled(w, r, false)
}

func (s *Server) handleSetUserEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	user, _, ok := s.editableUserFromRequest(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("Ungültige Formulardaten."), userEditURL(user.ID))
		return
	}
	form := managedUserFormFromAccount(user)
	form.Action = userFormActionStatus
	form.Version = formVersionFromRequest(r)
	fieldErrors := map[string]string{}
	if status := s.currentPasswordFormStatus(w, r, fieldErrors); status != 0 {
		form.FieldErrors = fieldErrors
		s.renderUserForm(w, r, status, form, false, false, "Die Passwortbestätigung ist fehlgeschlagen.")
		return
	}
	if form.Version < 1 {
		s.renderUserForm(w, r, http.StatusConflict, form, false, false, "Das Formular ist veraltet. Bitte laden Sie die Seite neu.")
		return
	}
	externalManagers := s.configActiveUserManagerCount()
	err := s.withAuthWrite(r.Context(), func() error {
		_, updateErr := s.repo.SetUserEnabled(r.Context(), user.ID, enabled, form.Version, externalManagers)
		if updateErr != nil {
			return updateErr
		}
		return s.reloadAuthSnapshot(r.Context())
	})
	setAuditTarget(r, "Benutzer:"+user.Username)
	if err != nil {
		s.renderUserMutationError(w, r, err, userEditURL(user.ID))
		return
	}
	notice := "Konto wurde aktiviert."
	if !enabled {
		notice = "Konto wurde deaktiviert. Bestehende Sitzungen wurden beendet."
	}
	redirectWithNotice(w, r, userEditURL(user.ID), notice)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	user, _, ok := s.editableUserFromRequest(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("Ungültige Formulardaten."), userEditURL(user.ID))
		return
	}
	form := managedUserFormFromAccount(user)
	form.Action = userFormActionDelete
	form.Version = formVersionFromRequest(r)
	fieldErrors := map[string]string{}
	if status := s.currentPasswordFormStatus(w, r, fieldErrors); status != 0 {
		form.FieldErrors = fieldErrors
		s.renderUserForm(w, r, status, form, false, false, "Die Passwortbestätigung ist fehlgeschlagen.")
		return
	}
	if form.Version < 1 {
		s.renderUserForm(w, r, http.StatusConflict, form, false, false, "Das Formular ist veraltet. Bitte laden Sie die Seite neu.")
		return
	}
	externalManagers := s.configActiveUserManagerCount()
	err := s.withAuthWrite(r.Context(), func() error {
		s.preferenceWriteMu.Lock()
		defer s.preferenceWriteMu.Unlock()
		if _, deleteErr := s.repo.DeleteUser(r.Context(), user.ID, form.Version, externalManagers); deleteErr != nil {
			return deleteErr
		}
		s.removeAccountPreferenceSnapshotLocked(authSourceDatabase, strconv.FormatInt(user.ID, 10))
		return s.reloadAuthSnapshot(r.Context())
	})
	setAuditTarget(r, "Benutzer:"+user.Username)
	if err != nil {
		s.renderUserMutationError(w, r, err, userEditURL(user.ID))
		return
	}
	redirectWithNotice(w, r, "/settings/users", "Konto "+user.Username+" wurde gelöscht.")
}

func (s *Server) handleAccount(w http.ResponseWriter, r *http.Request) {
	principal, ok := authPrincipalFromContext(r.Context())
	if !ok {
		s.renderForbidden(w, r)
		return
	}
	view, err := s.accountViewForPrincipal(r, principal, nil)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	s.render(w, r, "account.html", PageData{
		Title:   "Mein Konto",
		Active:  "account",
		Notice:  r.URL.Query().Get("notice"),
		Account: view,
	})
}

func (s *Server) handleChangeOwnPreferences(w http.ResponseWriter, r *http.Request) {
	principal, ok := authPrincipalFromContext(r.Context())
	if !ok {
		s.renderForbidden(w, r)
		return
	}
	setAuditTarget(r, "Benutzer:"+principal.Username)
	if err := r.ParseForm(); err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("Ungültige Formulardaten."), "/account")
		return
	}
	version, err := accountPreferenceVersionFromRequest(r)
	if err != nil {
		view, viewErr := s.accountViewForPrincipal(r, principal, map[string]string{"preferences": "Das Formular ist ungültig. Bitte laden Sie die Seite neu."})
		if viewErr != nil {
			s.renderHTTPError(w, r, viewErr)
			return
		}
		s.renderAccountForm(w, r, http.StatusConflict, view, "Das Formular ist veraltet. Bitte laden Sie die Seite neu.")
		return
	}
	err = s.withAuthWrite(r.Context(), func() error {
		_, saveErr := s.saveAccountPreference(r.Context(), repository.SaveAccountPreferenceParams{
			Source: principal.Source, Subject: principal.Subject,
			CustomPDFPreviewEnabled: preferenceEnabledFromRequest(r),
			ExpectedRowVersion:      version,
		})
		return saveErr
	})
	if err != nil {
		status := http.StatusInternalServerError
		message := "Die Vorschau-Einstellung konnte nicht gespeichert werden."
		fieldErrors := map[string]string{"preferences": message}
		if errors.Is(err, repository.ErrAccountPreferenceConflict) || errors.Is(err, errAuthPrincipalStale) {
			status = http.StatusConflict
			message = "Die Einstellung wurde zwischenzeitlich geändert. Bitte laden Sie die Seite neu."
			fieldErrors["preferences"] = message
		}
		view, viewErr := s.accountViewForPrincipal(r, principal, fieldErrors)
		if viewErr != nil {
			s.renderHTTPError(w, r, viewErr)
			return
		}
		s.renderAccountForm(w, r, status, view, message)
		return
	}
	redirectWithNotice(w, r, "/account", "PDF-Vorschau-Einstellung wurde gespeichert.")
}

func (s *Server) handleChangeUserPreferences(w http.ResponseWriter, r *http.Request) {
	principal, ok := authPrincipalFromContext(r.Context())
	if !ok {
		s.renderForbidden(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderUsersWithError(w, r, http.StatusBadRequest, "Ungültige Formulardaten.")
		return
	}
	source := strings.TrimSpace(r.FormValue("account_source"))
	subject := strings.TrimSpace(r.FormValue("account_subject"))
	username, databaseUser, configuredUser, err := s.preferenceTarget(r.Context(), source, subject)
	if err != nil {
		s.renderUsersWithError(w, r, http.StatusNotFound, "Nutzer nicht gefunden.")
		return
	}
	setAuditTarget(r, "Benutzer:"+username)
	allowed := false
	if source == authSourceDatabase {
		allowed = principal.Source == authSourceDatabase && principal.AccountID == databaseUser.ID
		allowed = allowed || actorCanManageUser(principal, databaseUser)
	} else {
		allowed = actorCanManageConfigPreference(principal, configuredUser)
	}
	if !allowed {
		s.renderForbidden(w, r)
		return
	}
	version, err := accountPreferenceVersionFromRequest(r)
	if err != nil {
		s.renderUsersPreferenceError(w, r, http.StatusConflict, "Das Formular ist veraltet. Bitte laden Sie die Seite neu.", source, subject, preferenceEnabledFromRequest(r))
		return
	}
	err = s.withAuthWrite(r.Context(), func() error {
		_, currentDatabaseUser, currentConfiguredUser, targetErr := s.preferenceTarget(r.Context(), source, subject)
		if targetErr != nil {
			return repository.ErrAccountPreferenceConflict
		}
		currentAllowed := false
		if source == authSourceDatabase {
			currentAllowed = principal.Source == authSourceDatabase && principal.AccountID == currentDatabaseUser.ID
			currentAllowed = currentAllowed || actorCanManageUser(principal, currentDatabaseUser)
		} else {
			currentAllowed = actorCanManageConfigPreference(principal, currentConfiguredUser)
		}
		if !currentAllowed {
			return errAuthPrincipalStale
		}
		_, saveErr := s.saveAccountPreference(r.Context(), repository.SaveAccountPreferenceParams{
			Source: source, Subject: subject,
			CustomPDFPreviewEnabled: preferenceEnabledFromRequest(r),
			ExpectedRowVersion:      version,
		})
		return saveErr
	})
	if err != nil {
		if errors.Is(err, repository.ErrAccountPreferenceConflict) || errors.Is(err, errAuthPrincipalStale) {
			s.renderUsersPreferenceError(w, r, http.StatusConflict, "Die Einstellung wurde zwischenzeitlich geändert. Bitte laden Sie die Seite neu.", source, subject, preferenceEnabledFromRequest(r))
			return
		}
		s.renderUsersPreferenceError(w, r, http.StatusInternalServerError, "Die Vorschau-Einstellung konnte nicht gespeichert werden.", source, subject, preferenceEnabledFromRequest(r))
		return
	}
	redirectWithNotice(w, r, "/settings/users", "PDF-Vorschau für "+username+" wurde gespeichert.")
}

func accountPreferenceVersionFromRequest(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.FormValue("preference_version"))
	version, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || version < 0 || strconv.FormatInt(version, 10) != raw {
		return 0, repository.ErrAccountPreferenceConflict
	}
	return version, nil
}

func preferenceEnabledFromRequest(r *http.Request) bool {
	for _, value := range r.Form["custom_pdf_preview_enabled"] {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true", "on", "yes":
			return true
		}
	}
	return false
}

func (s *Server) renderUsersWithError(w http.ResponseWriter, r *http.Request, status int, message string) {
	s.renderUsersPreferenceError(w, r, status, message, "", "", false)
}

func (s *Server) renderUsersPreferenceError(w http.ResponseWriter, r *http.Request, status int, message, source, subject string, attemptedEnabled bool) {
	users, err := s.repo.ListUsers(r.Context())
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	principal, _ := authPrincipalFromContext(r.Context())
	view := s.userManagementListView(users, principal)
	for index := range view.Users {
		if view.Users[index].Source == source && view.Users[index].Subject == subject {
			view.Users[index].CustomPDFPreviewEnabled = attemptedEnabled
			view.Users[index].PreferenceError = message
		}
	}
	view.Bootstrap = s.userBootstrapAllowed(r, users)
	view.CanCreate = view.Bootstrap || view.CanCreate
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	s.render(w, r, "users.html", PageData{
		Title: "Nutzerverwaltung", Active: "settings", SettingsTab: "users",
		Error: message, UserManagement: view,
	})
}

func (s *Server) handleChangeOwnPassword(w http.ResponseWriter, r *http.Request) {
	principal, ok := authPrincipalFromContext(r.Context())
	if !ok {
		s.renderForbidden(w, r)
		return
	}
	setAuditTarget(r, "Benutzer:"+principal.Username)
	if err := r.ParseForm(); err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("Ungültige Formulardaten."), "/account")
		return
	}
	user, found, err := s.databaseUserForPrincipal(r, principal)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	if !found {
		view, viewErr := s.accountViewForPrincipal(r, principal, nil)
		if viewErr != nil {
			s.renderHTTPError(w, r, viewErr)
			return
		}
		s.renderAccountForm(w, r, http.StatusForbidden, view, "Dieses Konto wird durch die Konfiguration verwaltet.")
		return
	}
	fieldErrors := map[string]string{}
	password := r.FormValue("new_password")
	if err := account.ValidatePassword(password); err != nil {
		fieldErrors["new_password"] = friendlyPasswordError(err)
	}
	if password != r.FormValue("new_password_confirmation") {
		fieldErrors["new_password_confirmation"] = "Die Passwörter stimmen nicht überein."
	}
	confirmationStatus := s.currentPasswordFormStatus(w, r, fieldErrors)
	view, err := s.accountViewForPrincipal(r, principal, fieldErrors)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	if len(fieldErrors) > 0 {
		status := http.StatusBadRequest
		if confirmationStatus != 0 {
			status = confirmationStatus
		}
		s.renderAccountForm(w, r, status, view, "Bitte korrigieren Sie die markierten Felder.")
		return
	}
	passwordHash, err := s.hashAuthPassword(r.Context(), password)
	if err != nil {
		if errors.Is(err, errAuthBcryptBusy) {
			setAuthBusyRetryAfter(w)
			s.renderAccountForm(w, r, http.StatusTooManyRequests, view, "Die Passwortverarbeitung ist ausgelastet. Bitte versuchen Sie es gleich erneut.")
			return
		}
		s.renderAccountForm(w, r, http.StatusBadRequest, view, friendlyPasswordError(err))
		return
	}
	var updated account.User
	err = s.withAuthWrite(r.Context(), func() error {
		var updateErr error
		updated, updateErr = s.repo.UpdateUserPassword(r.Context(), user.ID, account.UpdateUserPasswordParams{
			PasswordHash:       passwordHash,
			ExpectedRowVersion: user.RowVersion,
		})
		if updateErr != nil {
			return updateErr
		}
		if reloadErr := s.reloadAuthSnapshot(r.Context()); reloadErr != nil {
			return reloadErr
		}
		refreshedPrincipal, current := s.authPrincipalForDatabaseRevision(updated.ID, updated.Username, updated.SessionVersion)
		if !current || !s.setAuthSessionForPrincipal(w, r, refreshedPrincipal, authSessionDuration) {
			return errAuthPrincipalStale
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, repository.ErrUserConflict) {
			s.renderAccountForm(w, r, http.StatusConflict, view, "Das Konto wurde zwischenzeitlich geändert. Bitte versuchen Sie es erneut.")
			return
		}
		s.renderUserMutationError(w, r, err, "/account")
		return
	}
	redirectWithNotice(w, r, "/account", "Passwort wurde geändert. Andere Sitzungen wurden beendet.")
}

func (s *Server) editableUserFromRequest(w http.ResponseWriter, r *http.Request) (account.User, authPrincipal, bool) {
	id, err := idFromRequest(r)
	if err != nil {
		s.renderErrorWithReturn(w, r, http.StatusNotFound, errors.New("Nutzer nicht gefunden."), "/settings/users")
		return account.User{}, authPrincipal{}, false
	}
	user, err := s.repo.UserByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.renderErrorWithReturn(w, r, http.StatusNotFound, errors.New("Nutzer nicht gefunden."), "/settings/users")
		} else {
			s.renderHTTPError(w, r, err)
		}
		return account.User{}, authPrincipal{}, false
	}
	setAuditTarget(r, "Benutzer:"+user.Username)
	principal, ok := authPrincipalFromContext(r.Context())
	if !ok || !actorCanManageUser(principal, user) {
		s.renderForbidden(w, r)
		return account.User{}, authPrincipal{}, false
	}
	return user, principal, true
}

func (s *Server) renderUserForm(w http.ResponseWriter, r *http.Request, status int, form ManagedUserFormView, creating, bootstrap bool, errorText string) {
	principal, _ := authPrincipalFromContext(r.Context())
	view := userManagementFormView(principal, form, creating, bootstrap)
	data := PageData{
		Title:          map[bool]string{true: "Nutzer anlegen", false: "Nutzer bearbeiten"}[creating],
		Active:         "settings",
		SettingsTab:    "users",
		Error:          errorText,
		Notice:         r.URL.Query().Get("notice"),
		UserManagement: view,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	s.render(w, r, "user_form.html", data)
}

func (s *Server) renderAccountForm(w http.ResponseWriter, r *http.Request, status int, view AccountView, errorText string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	s.render(w, r, "account.html", PageData{
		Title:   "Mein Konto",
		Active:  "account",
		Error:   errorText,
		Account: view,
	})
}

func (s *Server) renderUserMutationError(w http.ResponseWriter, r *http.Request, err error, returnURL string) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, sql.ErrNoRows):
		status = http.StatusNotFound
	case errors.Is(err, repository.ErrUserConflict), errors.Is(err, repository.ErrLastActiveUserManager), errors.Is(err, errAuthPrincipalStale):
		status = http.StatusConflict
	case errors.Is(err, repository.ErrUserAlreadyExists):
		status = http.StatusBadRequest
	}
	s.renderErrorWithReturn(w, r, status, errors.New(friendlyUserMutationError(err)), returnURL)
}

func (s *Server) currentPasswordFormStatus(w http.ResponseWriter, r *http.Request, fieldErrors map[string]string) int {
	ok, retryAfter := s.authPasswordCheck(r, r.FormValue("current_password"))
	if ok {
		return 0
	}
	if retryAfter > 0 {
		seconds := int((retryAfter + time.Second - 1) / time.Second)
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		fieldErrors["current_password"] = "Zu viele fehlgeschlagene Versuche. Bitte versuchen Sie es später erneut."
		return http.StatusTooManyRequests
	}
	fieldErrors["current_password"] = "Das aktuelle Passwort ist nicht korrekt."
	return http.StatusForbidden
}

func setAuthBusyRetryAfter(w http.ResponseWriter) {
	w.Header().Set("Retry-After", strconv.FormatInt(retryAfterSeconds(authBcryptBusyRetryAfter), 10))
}

func friendlyUserMutationError(err error) string {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "Nutzer nicht gefunden."
	case errors.Is(err, repository.ErrUserConflict):
		return "Das Konto wurde zwischenzeitlich geändert. Bitte laden Sie die Seite neu."
	case errors.Is(err, errAuthPrincipalStale):
		return "Ihre Sitzung oder Berechtigung wurde zwischenzeitlich geändert. Bitte laden Sie die Seite neu und melden Sie sich gegebenenfalls erneut an."
	case errors.Is(err, repository.ErrLastActiveUserManager):
		return "Der letzte aktive Nutzerverwalter darf nicht deaktiviert, gelöscht oder herabgestuft werden."
	case errors.Is(err, repository.ErrUserAlreadyExists):
		return "Dieser Benutzername ist bereits vergeben."
	default:
		return "Die Kontoänderung konnte nicht gespeichert werden."
	}
}

func managedUserFormFromRequest(r *http.Request, creating bool) ManagedUserFormView {
	selected := make(map[string]bool, len(r.Form["permissions"]))
	for _, permission := range r.Form["permissions"] {
		permission = strings.TrimSpace(permission)
		if permission != "" {
			selected[permission] = true
		}
	}
	form := ManagedUserFormView{
		Username:            strings.TrimSpace(r.FormValue("username")),
		Role:                strings.TrimSpace(r.FormValue("role")),
		Active:              true,
		Version:             formVersionFromRequest(r),
		SelectedPermissions: selected,
		Editable:            true,
		CanEditAccess:       true,
	}
	if !creating {
		form.CanResetPassword = true
		form.CanChangeStatus = true
		form.CanDelete = true
	}
	return form
}

func managedUserFormFromAccount(user account.User) ManagedUserFormView {
	selected := make(map[string]bool, len(user.Permissions))
	for _, permission := range user.Permissions {
		selected[permission] = true
	}
	return ManagedUserFormView{
		ID:                  user.ID,
		Username:            user.Username,
		Role:                user.Role,
		Active:              user.Enabled,
		Version:             user.RowVersion,
		SelectedPermissions: selected,
		Editable:            true,
		CanEditAccess:       true,
		CanResetPassword:    true,
		CanChangeStatus:     true,
		CanDelete:           true,
	}
}

func formVersionFromRequest(r *http.Request) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("version")), 10, 64)
	if err != nil || value < 1 {
		return 0
	}
	return value
}

func selectedPermissionNames(selected map[string]bool) []string {
	permissions := make([]string, 0, len(selected))
	for permission, checked := range selected {
		if checked {
			permissions = append(permissions, permission)
		}
	}
	sort.Strings(permissions)
	return permissions
}

func additionalPermissionsForRole(role string, selected []string) []string {
	granted := map[string]bool{}
	for _, descriptor := range account.RoleDescriptors() {
		if descriptor.Name != strings.TrimSpace(role) {
			continue
		}
		for _, permission := range descriptor.Permissions {
			granted[permission] = true
		}
		break
	}
	additional := make([]string, 0, len(selected))
	for _, permission := range selected {
		if !granted[permission] {
			additional = append(additional, permission)
		}
	}
	return additional
}

func friendlyUsernameError(err error) string {
	switch {
	case errors.Is(err, account.ErrUsernameRequired):
		return "Bitte geben Sie einen Benutzernamen ein."
	case errors.Is(err, account.ErrUsernameTooLong):
		return "Der Benutzername darf höchstens 64 Zeichen lang sein."
	case errors.Is(err, account.ErrUsernameInvalid):
		return "Der Benutzername enthält unzulässige Zeichen. Steuerzeichen und Doppelpunkt sind nicht erlaubt."
	default:
		return "Der Benutzername ist ungültig."
	}
}

func friendlyPasswordError(err error) string {
	switch {
	case errors.Is(err, account.ErrPasswordTooShort):
		return "Das Passwort muss mindestens 12 Zeichen lang sein."
	case errors.Is(err, account.ErrPasswordTooLong):
		return "Das Passwort darf höchstens 72 UTF-8-Bytes lang sein."
	case errors.Is(err, account.ErrPasswordInvalid):
		return "Das Passwort enthält ungültige Zeichen."
	default:
		return "Das Passwort ist ungültig."
	}
}

func friendlyAccessError(err error) string {
	switch {
	case errors.Is(err, account.ErrUnknownRole):
		return "Die ausgewählte Rolle ist ungültig."
	case errors.Is(err, account.ErrUnknownPermission):
		return "Mindestens ein ausgewähltes Einzelrecht ist ungültig."
	case errors.Is(err, account.ErrAccessRequired):
		return "Wählen Sie eine Rolle oder mindestens ein Einzelrecht aus."
	default:
		return "Die ausgewählten Zugriffsrechte sind ungültig."
	}
}

func validateDelegatedAccess(actor authPrincipal, role string, permissions []string) error {
	if account.IsAdministratorRole(actor.Role) {
		return nil
	}
	if account.IsAdministratorRole(role) || account.IsUserManager(role, permissions) {
		return errors.New("Nur Administratoren dürfen Administratoren oder weitere Nutzerverwalter anlegen und verwalten.")
	}
	requested, err := account.CapabilitiesFor(role, permissions)
	if err != nil {
		return err
	}
	actorCapabilities := account.Capabilities(actor.capabilities)
	if requested&^actorCapabilities != 0 {
		return errors.New("Sie dürfen nur Rechte vergeben, die Sie selbst besitzen.")
	}
	return nil
}

func actorCanCreateUser(actor authPrincipal) bool {
	if account.IsAdministratorRole(actor.Role) {
		return true
	}
	capabilities := account.Capabilities(actor.capabilities) &^ account.CapabilitySystemUsersManage
	return capabilities != 0
}

func actorCanManageUser(actor authPrincipal, target account.User) bool {
	if actor.Username == "" || actor.Username == target.Username {
		return false
	}
	if account.IsAdministratorRole(actor.Role) {
		return true
	}
	if account.IsAdministratorRole(target.Role) || account.IsUserManager(target.Role, target.Permissions) {
		return false
	}
	targetCapabilities, err := account.CapabilitiesFor(target.Role, target.Permissions)
	if err != nil {
		return false
	}
	return targetCapabilities&^account.Capabilities(actor.capabilities) == 0
}

func defaultAssignableUserRole(actor authPrincipal, bootstrap bool) string {
	if bootstrap {
		return account.RoleAdmin
	}
	_ = actor
	return account.RoleCustom
}

func userEditURL(id int64) string {
	return fmt.Sprintf("/settings/users/%d", id)
}

func requestIsLoopback(r *http.Request) bool {
	return loopbackHost(r.RemoteAddr) && loopbackHost(r.Host)
}

func loopbackHost(hostPort string) bool {
	host := strings.TrimSpace(hostPort)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(strings.TrimSuffix(host, "."), "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) userBootstrapAllowed(r *http.Request, users []account.User) bool {
	return len(users) == 0 && !s.cfg.Auth.Enabled() && requestIsLoopback(r)
}

func (s *Server) configUsernameExists(username string) bool {
	for _, user := range s.authConfigAccountViews() {
		if user.Username == username {
			return true
		}
	}
	return false
}
