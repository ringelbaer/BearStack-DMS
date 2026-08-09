package account

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type Capabilities uint64

const (
	CapabilityDocumentsRead Capabilities = 1 << iota
	CapabilityDocumentsWebDAVRead
	CapabilityDocumentsUpload
	CapabilityDocumentsEdit
	CapabilityDocumentsDelete
	CapabilityDocumentsStructure
	CapabilityPhotosRead
	CapabilityPhotosEdit
	CapabilityPhotosManage
	CapabilitySystemManage
	CapabilitySystemAudit
	CapabilitySystemUsersManage
)

const AllCapabilities = CapabilityDocumentsRead |
	CapabilityDocumentsWebDAVRead |
	CapabilityDocumentsUpload |
	CapabilityDocumentsEdit |
	CapabilityDocumentsDelete |
	CapabilityDocumentsStructure |
	CapabilityPhotosRead |
	CapabilityPhotosEdit |
	CapabilityPhotosManage |
	CapabilitySystemManage |
	CapabilitySystemAudit |
	CapabilitySystemUsersManage

const (
	PermissionDocumentsRead       = "documents.read"
	PermissionDocumentsWebDAVRead = "documents.webdav.read"
	PermissionDocumentsUpload     = "documents.upload"
	PermissionDocumentsEdit       = "documents.edit"
	PermissionDocumentsDelete     = "documents.delete"
	PermissionDocumentsStructure  = "documents.structure"
	PermissionPhotosRead          = "photos.read"
	PermissionPhotosEdit          = "photos.edit"
	PermissionPhotosManage        = "photos.manage"
	PermissionSystemManage        = "system.manage"
	PermissionSystemAudit         = "system.audit"
	PermissionSystemUsersManage   = "system.users.manage"
)

const (
	RoleAdmin            = "admin"
	RoleDocumentsRead    = "documents_read"
	RoleDocumentsEditor  = "documents_editor"
	RoleDocumentsManager = "documents_manager"
	RolePhotosRead       = "photos_read"
	RolePhotosEditor     = "photos_editor"
	RolePhotosManager    = "photos_manager"
	RoleAPIUploader      = "api_uploader"
	RoleCustom           = "custom"
)

var (
	ErrUnknownRole       = errors.New("unknown account role")
	ErrUnknownPermission = errors.New("unknown account permission")
	ErrAccessRequired    = errors.New("at least one account permission is required")
)

type PermissionDescriptor struct {
	Name        string
	Label       string
	Description string
	Group       string
	Capability  Capabilities
}

type RoleDescriptor struct {
	Name        string
	Label       string
	Description string
	Permissions []string
}

var permissionDescriptors = []PermissionDescriptor{
	{Name: PermissionDocumentsRead, Label: "Dokumente lesen", Description: "Dokumente suchen, ansehen und herunterladen", Group: "Dokumente", Capability: CapabilityDocumentsRead},
	{Name: PermissionDocumentsWebDAVRead, Label: "WebDAV lesen", Description: "Dokumente über WebDAV lesen", Group: "Dokumente", Capability: CapabilityDocumentsWebDAVRead},
	{Name: PermissionDocumentsUpload, Label: "Dokumente hochladen", Description: "Neue Dokumente importieren", Group: "Dokumente", Capability: CapabilityDocumentsUpload},
	{Name: PermissionDocumentsEdit, Label: "Dokumente bearbeiten", Description: "Metadaten, OCR und Verknüpfungen bearbeiten", Group: "Dokumente", Capability: CapabilityDocumentsEdit},
	{Name: PermissionDocumentsDelete, Label: "Dokumente löschen", Description: "Dokumente in den Papierkorb verschieben und endgültig löschen", Group: "Dokumente", Capability: CapabilityDocumentsDelete},
	{Name: PermissionDocumentsStructure, Label: "Dokumentstruktur verwalten", Description: "Tags, Felder und Suchfavoriten verwalten", Group: "Dokumente", Capability: CapabilityDocumentsStructure},
	{Name: PermissionPhotosRead, Label: "Fotos lesen", Description: "Fotos und Alben ansehen", Group: "Fotos", Capability: CapabilityPhotosRead},
	{Name: PermissionPhotosEdit, Label: "Fotos bearbeiten", Description: "Foto-Metadaten und Tags bearbeiten", Group: "Fotos", Capability: CapabilityPhotosEdit},
	{Name: PermissionPhotosManage, Label: "Fotobibliothek verwalten", Description: "Fotobibliothek und Blogs verwalten", Group: "Fotos", Capability: CapabilityPhotosManage},
	{Name: PermissionSystemManage, Label: "System verwalten", Description: "Anwendungseinstellungen und Wartung verwalten", Group: "System", Capability: CapabilitySystemManage},
	{Name: PermissionSystemAudit, Label: "Audit-Protokoll lesen", Description: "Audit-Ereignisse einsehen", Group: "System", Capability: CapabilitySystemAudit},
	{Name: PermissionSystemUsersManage, Label: "Benutzer verwalten", Description: "Benutzerkonten und deren Zugriffsrechte verwalten", Group: "System", Capability: CapabilitySystemUsersManage},
}

var permissionCapabilities = func() map[string]Capabilities {
	result := make(map[string]Capabilities, len(permissionDescriptors))
	for _, descriptor := range permissionDescriptors {
		result[descriptor.Name] = descriptor.Capability
	}
	return result
}()

var roleCapabilities = map[string]Capabilities{
	RoleAdmin: AllCapabilities,
	RoleDocumentsRead: CapabilityDocumentsRead |
		CapabilityDocumentsWebDAVRead,
	RoleDocumentsEditor: CapabilityDocumentsRead |
		CapabilityDocumentsWebDAVRead |
		CapabilityDocumentsUpload |
		CapabilityDocumentsEdit,
	RoleDocumentsManager: CapabilityDocumentsRead |
		CapabilityDocumentsWebDAVRead |
		CapabilityDocumentsUpload |
		CapabilityDocumentsEdit |
		CapabilityDocumentsDelete |
		CapabilityDocumentsStructure,
	RolePhotosRead: CapabilityPhotosRead,
	RolePhotosEditor: CapabilityPhotosRead |
		CapabilityPhotosEdit,
	RolePhotosManager: CapabilityPhotosRead |
		CapabilityPhotosEdit |
		CapabilityPhotosManage,
	RoleAPIUploader: CapabilityDocumentsUpload,
	RoleCustom:      0,
}

var roleDescriptors = []RoleDescriptor{
	{Name: RoleAdmin, Label: "Administrator", Description: "Vollzugriff auf alle Bereiche", Permissions: permissionNamesForCapabilities(AllCapabilities)},
	{Name: RoleDocumentsRead, Label: "Dokumente lesen", Description: "Dokumente und WebDAV lesend verwenden", Permissions: permissionNamesForCapabilities(roleCapabilities[RoleDocumentsRead])},
	{Name: RoleDocumentsEditor, Label: "Dokumente bearbeiten", Description: "Dokumente lesen, hochladen und bearbeiten", Permissions: permissionNamesForCapabilities(roleCapabilities[RoleDocumentsEditor])},
	{Name: RoleDocumentsManager, Label: "Dokumente verwalten", Description: "Vollzugriff auf Dokumente und ihre Struktur", Permissions: permissionNamesForCapabilities(roleCapabilities[RoleDocumentsManager])},
	{Name: RolePhotosRead, Label: "Fotos lesen", Description: "Fotobibliothek lesend verwenden", Permissions: permissionNamesForCapabilities(roleCapabilities[RolePhotosRead])},
	{Name: RolePhotosEditor, Label: "Fotos bearbeiten", Description: "Fotos lesen und bearbeiten", Permissions: permissionNamesForCapabilities(roleCapabilities[RolePhotosEditor])},
	{Name: RolePhotosManager, Label: "Fotos verwalten", Description: "Vollzugriff auf die Fotobibliothek", Permissions: permissionNamesForCapabilities(roleCapabilities[RolePhotosManager])},
	{Name: RoleAPIUploader, Label: "API-Uploader", Description: "Dokumente ausschließlich hochladen", Permissions: permissionNamesForCapabilities(roleCapabilities[RoleAPIUploader])},
	{Name: RoleCustom, Label: "Benutzerdefiniert", Description: "Zugriff ausschließlich über zusätzliche Einzelrechte"},
}

func PermissionDescriptors() []PermissionDescriptor {
	return append([]PermissionDescriptor(nil), permissionDescriptors...)
}

func RoleDescriptors() []RoleDescriptor {
	result := make([]RoleDescriptor, len(roleDescriptors))
	copy(result, roleDescriptors)
	for i := range result {
		result[i].Permissions = append([]string(nil), result[i].Permissions...)
	}
	return result
}

func (c Capabilities) HasAll(required Capabilities) bool {
	return required == 0 || c&required == required
}

func (c Capabilities) HasAny(required Capabilities) bool {
	return required == 0 || c&required != 0
}

// NormalizeAccess validates a persisted role and its additive permissions.
func NormalizeAccess(role string, additional []string) (string, []string, Capabilities, error) {
	role = strings.TrimSpace(role)
	if role == "" {
		role = RoleCustom
	}
	base, ok := roleCapabilities[role]
	if !ok {
		return "", nil, 0, fmt.Errorf("%w %q", ErrUnknownRole, role)
	}
	permissions, extraCapabilities, err := normalizePermissions(additional)
	if err != nil {
		return "", nil, 0, err
	}
	capabilities := base | extraCapabilities
	if capabilities == 0 {
		return "", nil, 0, ErrAccessRequired
	}
	return role, permissions, capabilities, nil
}

func CapabilitiesFor(role string, additional []string) (Capabilities, error) {
	_, _, capabilities, err := NormalizeAccess(role, additional)
	return capabilities, err
}

// ConfigCapabilitiesFor preserves the legacy configuration default: an entry
// without a role or individual permissions is an administrator.
func ConfigCapabilitiesFor(role string, additional []string) (Capabilities, string, error) {
	if strings.TrimSpace(role) == "" && !hasNonBlank(additional) {
		role = RoleAdmin
	}
	normalizedRole, _, capabilities, err := NormalizeAccess(role, additional)
	return capabilities, normalizedRole, err
}

func EffectivePermissionNames(role string, additional []string) ([]string, error) {
	capabilities, err := CapabilitiesFor(role, additional)
	if err != nil {
		return nil, err
	}
	return permissionNamesForCapabilities(capabilities), nil
}

func IsAdministratorRole(role string) bool {
	return strings.TrimSpace(role) == RoleAdmin
}

func IsUserManager(role string, additional []string) bool {
	capabilities, err := CapabilitiesFor(role, additional)
	return err == nil && capabilities.HasAll(CapabilitySystemUsersManage)
}

func normalizePermissions(values []string) ([]string, Capabilities, error) {
	unique := make(map[string]struct{}, len(values))
	var capabilities Capabilities
	for _, value := range values {
		name := strings.TrimSpace(value)
		if name == "" {
			continue
		}
		capability, ok := permissionCapabilities[name]
		if !ok {
			return nil, 0, fmt.Errorf("%w %q", ErrUnknownPermission, name)
		}
		unique[name] = struct{}{}
		capabilities |= capability
	}
	names := make([]string, 0, len(unique))
	for name := range unique {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, capabilities, nil
}

func permissionNamesForCapabilities(capabilities Capabilities) []string {
	result := make([]string, 0, len(permissionDescriptors))
	for _, descriptor := range permissionDescriptors {
		if capabilities.HasAll(descriptor.Capability) {
			result = append(result, descriptor.Name)
		}
	}
	return result
}

func hasNonBlank(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}
