// Datei definiert die ausschließlich für HTML verwendeten Ansichten der Nutzerverwaltung.
package server

// UserManagementView bündelt Listen- und Formulardaten der Nutzerverwaltung.
// Passwörter sind absichtlich kein Bestandteil dieser Struktur.
type UserManagementView struct {
	Users             []ManagedUserView
	Form              ManagedUserFormView
	Roles             []UserRoleOptionView
	PermissionGroups  []UserPermissionGroupView
	Creating          bool
	Bootstrap         bool
	CanCreate         bool
	CurrentUsername   string
	CurrentUserSource string
	CurrentUserID     int64
}

type ManagedUserView struct {
	ID                   int64
	Username             string
	Source               string
	SourceLabel          string
	Role                 string
	RoleLabel            string
	Active               bool
	Current              bool
	Editable             bool
	Version              int64
	ExtraPermissions     []UserPermissionLabelView
	EffectivePermissions []UserPermissionLabelView
}

type ManagedUserFormView struct {
	ID                  int64
	Username            string
	Role                string
	Active              bool
	Version             int64
	SelectedPermissions map[string]bool
	FieldErrors         map[string]string
	Editable            bool
	CanEditAccess       bool
	CanResetPassword    bool
	CanChangeStatus     bool
	CanDelete           bool
	Current             bool
	Action              string
}

func (f ManagedUserFormView) ActionIs(name string) bool {
	return f.Action == name
}

func (f ManagedUserFormView) FieldError(name string) string {
	if f.FieldErrors == nil {
		return ""
	}
	return f.FieldErrors[name]
}

func (f ManagedUserFormView) PermissionSelected(name string) bool {
	return f.SelectedPermissions != nil && f.SelectedPermissions[name]
}

type UserRoleOptionView struct {
	Value              string
	Label              string
	Description        string
	GrantedPermissions string
	Disabled           bool
}

type UserPermissionGroupView struct {
	Label       string
	Permissions []UserPermissionOptionView
}

type UserPermissionOptionView struct {
	Value       string
	Label       string
	Description string
	RoleGranted bool
	Selected    bool
	Assignable  bool
	Disabled    bool
}

type UserPermissionLabelView struct {
	Value string
	Label string
}

// AccountView enthält die nicht geheimen Informationen der Selbstverwaltung.
type AccountView struct {
	Username             string
	Source               string
	SourceLabel          string
	Role                 string
	RoleLabel            string
	CanChangePassword    bool
	EffectivePermissions []UserPermissionLabelView
	FieldErrors          map[string]string
}

func (a AccountView) FieldError(name string) string {
	if a.FieldErrors == nil {
		return ""
	}
	return a.FieldErrors[name]
}
