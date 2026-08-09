// Datei behandelt Verwaltung und Bearbeitung benutzerdefinierter Dokumentfelder.
package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"bearstack/internal/document"
	"bearstack/internal/repository"
)

func (s *Server) handleFields(w http.ResponseWriter, r *http.Request) {
	fields, err := s.repo.ListCustomFields(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	s.render(w, r, "fields.html", PageData{
		Title:        "Felder",
		Active:       "fields",
		CustomFields: fields,
		Notice:       r.URL.Query().Get("notice"),
	})
}

func (s *Server) handleSaveField(w http.ResponseWriter, r *http.Request) {
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	label := strings.TrimSpace(r.FormValue("label"))
	if label == "" {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("Feldname fehlt"), "/fields")
		return
	}
	autocompleteEnabled := r.FormValue("autocomplete_enabled") == "1"
	valueFolderMinDocuments := customFieldValueFolderMinDocumentsFromForm(r.FormValue("value_folder_min_documents"))
	if err := s.repo.SaveCustomField(r.Context(), label, autocompleteEnabled, valueFolderMinDocuments); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	s.invalidateDocumentCountCache()
	redirectWithNotice(w, r, "/fields", "Feld gespeichert.")
}

func (s *Server) handleDeleteField(w http.ResponseWriter, r *http.Request) {
	id, err := positiveIDFromPath(r, "id")
	if err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("ungültige Feld-ID"), "/fields")
		return
	}
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	if status, confirmationErr := s.passwordConfirmationFailure(w, r, r.FormValue("password")); confirmationErr != nil {
		s.renderErrorWithReturn(w, r, status, confirmationErr, "/fields")
		return
	}
	if err := s.repo.DeleteCustomField(r.Context(), id); err != nil {
		s.renderFieldHTTPError(w, r, err)
		return
	}
	s.invalidateDocumentCountCache()
	redirectWithNotice(w, r, "/fields", "Feld gelöscht. Alle Werte dieses Felds wurden entfernt und die Suche wurde aktualisiert.")
}

func (s *Server) handleUpdateField(w http.ResponseWriter, r *http.Request) {
	id, err := positiveIDFromPath(r, "id")
	if err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("ungültige Feld-ID"), "/fields")
		return
	}
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	label := strings.TrimSpace(r.FormValue("label"))
	if label == "" {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("Feldname fehlt"), "/fields")
		return
	}
	autocompleteEnabled := r.FormValue("autocomplete_enabled") == "1"
	valueFolderMinDocuments := customFieldValueFolderMinDocumentsFromForm(r.FormValue("value_folder_min_documents"))
	if err := s.repo.UpdateCustomField(r.Context(), id, label, autocompleteEnabled, valueFolderMinDocuments); err != nil {
		if errors.Is(err, repository.ErrCustomFieldLabelExists) {
			s.renderErrorWithReturn(w, r, http.StatusBadRequest, err, "/fields")
			return
		}
		s.renderFieldHTTPError(w, r, err)
		return
	}
	setAuditTarget(r, namedAuditTarget("Feld", id, label))
	s.invalidateDocumentCountCache()
	redirectWithNotice(w, r, "/fields", "Feld gespeichert.")
}

func (s *Server) handleFieldAutocomplete(w http.ResponseWriter, r *http.Request) {
	id, err := positiveIDFromPath(r, "id")
	if err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("ungültige Feld-ID"), "/fields")
		return
	}
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	enabled := r.FormValue("autocomplete_enabled") == "1"
	if err := s.repo.UpdateCustomFieldAutocomplete(r.Context(), id, enabled); err != nil {
		s.renderFieldHTTPError(w, r, err)
		return
	}
	notice := "Auto-Vervollständigung deaktiviert."
	if enabled {
		notice = "Auto-Vervollständigung aktiviert."
	}
	redirectWithNotice(w, r, "/fields", notice)
}

func customFieldValueFolderMinDocumentsFromForm(value string) int {
	switch strings.TrimSpace(value) {
	case "always", "1":
		return document.CustomFieldValueFolderAlways
	case "5":
		return 5
	case "10":
		return 10
	case "20":
		return 20
	case "50":
		return 50
	default:
		return document.CustomFieldValueFolderNever
	}
}

func (s *Server) handleFieldValueSuggestions(w http.ResponseWriter, r *http.Request) {
	id, err := positiveIDFromPath(r, "id")
	if err != nil {
		if wantsJSONResponse(r) {
			s.renderJSONError(w, http.StatusBadRequest, "ungültige Feld-ID")
		} else {
			s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("ungültige Feld-ID"), "/fields")
		}
		return
	}
	if !wantsJSONResponse(r) {
		if !s.requestHasCapabilities(r, authCapDocumentsStructure) {
			s.renderForbidden(w, r)
			return
		}
		s.handleFieldValues(w, r, id)
		return
	}
	values, err := s.repo.CustomFieldValueSuggestions(r.Context(), id, r.URL.Query().Get("q"), 20)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.renderJSONError(w, http.StatusNotFound, "Feld nicht gefunden")
			return
		}
		s.renderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if values == nil {
		values = []string{}
	}
	if err := writeJSON(w, http.StatusOK, struct {
		Values []string `json:"values"`
	}{Values: values}); err != nil {
		s.log.Warn("custom field value suggestions response failed", "field_id", id, "error", err)
	}
}

func (s *Server) renderFieldHTTPError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		s.renderErrorWithReturn(w, r, http.StatusNotFound, errors.New("Feld nicht gefunden"), "/fields")
		return
	}
	s.renderHTTPError(w, r, err)
}

func (s *Server) handleFieldValues(w http.ResponseWriter, r *http.Request, id int64) {
	field, err := s.repo.GetCustomField(r.Context(), id)
	if err != nil {
		s.renderFieldHTTPError(w, r, err)
		return
	}
	values, err := s.repo.CustomFieldValues(r.Context(), id)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	s.render(w, r, "field_values.html", PageData{
		Title:                  "Feldwerte",
		Active:                 "fields",
		CustomField:            field,
		CustomFieldValues:      values,
		CustomFieldSuggestions: similarCustomFieldValues(values),
		Notice:                 r.URL.Query().Get("notice"),
	})
}

func (s *Server) handleUpdateFieldValue(w http.ResponseWriter, r *http.Request) {
	id, err := positiveIDFromPath(r, "id")
	if err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("ungültige Feld-ID"), "/fields")
		return
	}
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	oldValue := strings.TrimSpace(r.FormValue("old_value"))
	newValue := strings.TrimSpace(r.FormValue("new_value"))
	if oldValue == "" || newValue == "" {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("alter und neuer Wert werden benötigt"), fieldValuesReturnURL(id))
		return
	}
	updated, err := s.repo.RenameCustomFieldValue(r.Context(), id, oldValue, newValue)
	if err != nil {
		s.renderFieldHTTPError(w, r, err)
		return
	}
	s.invalidateDocumentCountCache()
	setAuditTarget(r, namedAuditTarget("Feld", id, oldValue+" -> "+newValue))
	notice := "Kein Datensatz geändert."
	if updated == 1 {
		notice = "1 Datensatz aktualisiert."
	} else if updated > 1 {
		notice = strconv.Itoa(updated) + " Datensätze aktualisiert."
	}
	redirectWithNotice(w, r, "/fields/"+strconv.FormatInt(id, 10)+"/values", notice)
}

func fieldValuesReturnURL(id int64) string {
	return "/fields/" + strconv.FormatInt(id, 10) + "/values"
}

func similarCustomFieldValues(values []document.CustomFieldValue) []document.CustomFieldValueSuggestion {
	type normalizedValue struct {
		value      string
		comparable string
	}
	normalized := make([]normalizedValue, 0, len(values))
	for _, value := range values {
		comparable := comparableCustomFieldValue(value.Value)
		if comparable == "" {
			continue
		}
		normalized = append(normalized, normalizedValue{value: value.Value, comparable: comparable})
	}
	seen := map[string]struct{}{}
	var suggestions []document.CustomFieldValueSuggestion
	for i := 0; i < len(normalized); i++ {
		group := []string{normalized[i].value}
		reason := ""
		for j := i + 1; j < len(normalized); j++ {
			distance := levenshteinDistance(normalized[i].comparable, normalized[j].comparable)
			if normalized[i].comparable == normalized[j].comparable {
				reason = "Unterschied nur bei Großschreibung, Leerzeichen oder Satzzeichen"
				group = append(group, normalized[j].value)
			} else if distance <= 1 || (len(normalized[i].comparable) >= 6 && len(normalized[j].comparable) >= 6 && distance <= 2) {
				if reason == "" {
					reason = "Sehr ähnliche Schreibweise"
				}
				group = append(group, normalized[j].value)
			}
		}
		if len(group) < 2 {
			continue
		}
		sort.Strings(group)
		key := strings.Join(group, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		suggestions = append(suggestions, document.CustomFieldValueSuggestion{
			Value:   group[0],
			Similar: group[1:],
			Reason:  reason,
		})
	}
	return suggestions
}

func comparableCustomFieldValue(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func levenshteinDistance(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 0
			if ar[i-1] != br[j-1] {
				cost = 1
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(br)]
}

func (s *Server) handleSaveColumns(w http.ResponseWriter, r *http.Request) {
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	fields, err := s.repo.ListCustomFields(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	settings := storedDocumentColumnSettings{
		Order:                 normalizeColumnOrder(r.Form["column_order"], fields),
		Visible:               normalizeColumnSelection(r.Form["columns"], fields),
		DesktopDateUnderTitle: r.FormValue("desktop_date_under_title") == "1",
	}
	payload, err := json.Marshal(settings)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	if err := s.repo.SaveSetting(r.Context(), documentColumnsSettingKey, string(payload)); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	redirect(w, r, formReturnURL(r))
}
