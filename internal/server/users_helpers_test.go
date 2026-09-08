package server

import (
	"bytes"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"bearstack/internal/account"
	"bearstack/internal/config"
)

func TestAdditionalPermissionsForRoleRemovesInheritedRights(t *testing.T) {
	got := additionalPermissionsForRole(account.RoleDocumentsRead, []string{
		account.PermissionDocumentsRead,
		account.PermissionDocumentsWebDAVRead,
		account.PermissionDocumentsUpload,
	})
	want := []string{account.PermissionDocumentsUpload}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("additional permissions = %#v, want %#v", got, want)
	}
}

func TestUserPresenterMarksSelfAndConfigurationAccountsReadOnly(t *testing.T) {
	actor := authPrincipal{
		Username:     "admin",
		Role:         account.RoleAdmin,
		capabilities: authCapabilities(account.AllCapabilities),
	}
	server := &Server{cfg: config.Config{Auth: config.AuthConfig{Credentials: []config.AuthCredential{
		{Username: "config-admin", Password: "configuration-secret"},
	}}}}
	view := server.userManagementListView([]account.User{
		{ID: 1, Username: "admin", Role: account.RoleAdmin, Enabled: true, RowVersion: 2},
		{ID: 2, Username: "reader", Role: account.RoleDocumentsRead, Enabled: true, RowVersion: 1},
	}, actor)
	if len(view.Users) != 3 {
		t.Fatalf("presented users = %#v", view.Users)
	}
	byName := map[string]ManagedUserView{}
	for _, user := range view.Users {
		byName[user.Username] = user
	}
	if !byName["admin"].Current || byName["admin"].Editable {
		t.Fatalf("self presentation = %#v", byName["admin"])
	}
	if !byName["reader"].Editable {
		t.Fatalf("ordinary database user is not editable: %#v", byName["reader"])
	}
	configured := byName["config-admin"]
	if configured.Source != "config" || configured.Editable || !configured.Active {
		t.Fatalf("configuration user presentation = %#v", configured)
	}
}

func TestUserViewModelsContainNoPasswordMaterial(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(UserManagementView{}),
		reflect.TypeOf(ManagedUserView{}),
		reflect.TypeOf(ManagedUserFormView{}),
		reflect.TypeOf(AccountView{}),
	}
	for _, typ := range types {
		for i := 0; i < typ.NumField(); i++ {
			if typ.Field(i).Type.Kind() == reflect.Bool {
				continue
			}
			name := strings.ToLower(typ.Field(i).Name)
			if strings.Contains(name, "password") || strings.Contains(name, "hash") || strings.Contains(name, "secret") {
				t.Fatalf("%s exposes sensitive field %q", typ.Name(), typ.Field(i).Name)
			}
		}
	}
}

func TestUserFormTemplateRendersInlineErrorsWithoutPasswordValues(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	actor := authPrincipal{
		Username:     "admin",
		Role:         account.RoleAdmin,
		capabilities: authCapabilities(account.AllCapabilities),
	}
	form := ManagedUserFormView{
		ID:                  42,
		Username:            "Alice <unsafe>",
		Role:                account.RoleDocumentsRead,
		Active:              true,
		Version:             7,
		SelectedPermissions: map[string]bool{},
		FieldErrors: map[string]string{
			"new_password":              "Das neue Passwort ist zu kurz.",
			"new_password_confirmation": "Die Passwörter stimmen nicht überein.",
			"current_password":          "Das aktuelle Passwort ist falsch.",
		},
		Editable:         true,
		CanEditAccess:    true,
		CanResetPassword: true,
		CanChangeStatus:  true,
		CanDelete:        true,
		Action:           userFormActionPassword,
	}
	view := userManagementFormView(actor, form, false, false)
	var output bytes.Buffer
	if err := templates.ExecuteTemplate(&output, "user_form.html", PageData{
		Title:          "Nutzer bearbeiten",
		Auth:           authPermissionsFromCapabilities(actor.capabilities, actor),
		UserManagement: view,
	}); err != nil {
		t.Fatal(err)
	}
	body := output.String()
	for _, want := range []string{
		`id="reset-new-password-error"`,
		`id="reset-new-password-confirmation-error"`,
		`id="reset-current-password-error"`,
		`aria-describedby="reset-current-password-error"`,
		"Das neue Passwort ist zu kurz.",
		"Die Passwörter stimmen nicht überein.",
		"Das aktuelle Passwort ist falsch.",
		"Alice &lt;unsafe&gt;",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered form is missing %q: %s", want, body)
		}
	}
	if count := strings.Count(body, "Das aktuelle Passwort ist falsch."); count != 1 {
		t.Fatalf("current-password error rendered %d times, want exactly once", count)
	}
	if strings.Contains(body, `id="access-current-password-error"`) || strings.Contains(body, `id="status-current-password-error"`) || strings.Contains(body, `id="delete-current-password-error"`) {
		t.Fatalf("password error leaked into an unrelated action form: %s", body)
	}
	assertRenderedPasswordInputsEmpty(t, body)
}

func TestAccountTemplateRendersInlineErrorsWithoutPasswordValues(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	view := AccountView{
		Username:          "Alice",
		SourceLabel:       "BearStack-Datenbank",
		RoleLabel:         "Administrator",
		CanChangePassword: true,
		FieldErrors: map[string]string{
			"current_password":          "Aktuelles Passwort falsch.",
			"new_password":              "Neues Passwort zu kurz.",
			"new_password_confirmation": "Bestätigung falsch.",
		},
	}
	var output bytes.Buffer
	if err := templates.ExecuteTemplate(&output, "account.html", PageData{Title: "Mein Konto", Account: view}); err != nil {
		t.Fatal(err)
	}
	body := output.String()
	for _, want := range []string{
		`id="account-current-password-error"`,
		`id="account-new-password-error"`,
		`id="account-new-password-confirmation-error"`,
		"Aktuelles Passwort falsch.",
		"Neues Passwort zu kurz.",
		"Bestätigung falsch.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered account form is missing %q: %s", want, body)
		}
	}
	assertRenderedPasswordInputsEmpty(t, body)
}

func assertRenderedPasswordInputsEmpty(t *testing.T, body string) {
	t.Helper()
	inputs := regexp.MustCompile(`<input\s+type="password"[^>]*>`).FindAllString(body, -1)
	namedInputs := 0
	for _, input := range inputs {
		if !strings.Contains(input, `name="`) {
			continue
		}
		namedInputs++
		if !strings.Contains(input, `value=""`) {
			t.Fatalf("named password input retains a value: %s", input)
		}
	}
	if namedInputs == 0 {
		t.Fatal("rendered page has no password inputs")
	}
}
