// Datei liest und validiert Formularwerte fuer Dokument-, Foto- und Einstellungsaktionen.
package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/tagutil"
)

func normalizeTagValues(values []string, newTags string) []string {
	var parts []string
	parts = append(parts, values...)
	parts = append(parts, newTags)
	return tagutil.NormalizeString(strings.Join(parts, ","))
}

func firstNormalizedTag(value string) string {
	tags := tagutil.NormalizeString(value)
	if len(tags) == 0 {
		return ""
	}
	return tags[0]
}

func currentReturnURL(r *http.Request) string {
	return requestURLWithoutQueryKeys(r, "notice", "highlight")
}

func safeReturnURL(value string) string {
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return "/"
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return "/"
	}
	if strings.Contains(parsed.Path, "\\") {
		return "/"
	}
	if decodedPath, err := url.PathUnescape(parsed.EscapedPath()); err == nil {
		if strings.HasPrefix(decodedPath, "//") || strings.Contains(decodedPath, "\\") {
			return "/"
		}
	}
	return value
}

func formReturnURL(r *http.Request) string {
	if err := r.ParseForm(); err != nil {
		return ""
	}
	return safeReturnURL(r.FormValue("return"))
}

func (s *Server) parseFormOrRenderError(w http.ResponseWriter, r *http.Request) bool {
	if err := r.ParseForm(); err != nil {
		s.renderError(w, r, http.StatusBadRequest, err)
		return false
	}
	return true
}

func documentURL(id int64, returnURL, notice string) string {
	target := fmt.Sprintf("/documents/%d", id)
	q := url.Values{}
	if notice != "" {
		q.Set("notice", notice)
	}
	returnURL = safeReturnURL(returnURL)
	if returnURL != "/" {
		q.Set("return", returnURL)
	}
	if encoded := q.Encode(); encoded != "" {
		target += "?" + encoded
	}
	return target
}

func documentViewURL(id int64, returnURL, notice string) string {
	target := fmt.Sprintf("/documents/%d/view", id)
	q := url.Values{}
	if notice != "" {
		q.Set("notice", notice)
	}
	returnURL = safeReturnURL(returnURL)
	if returnURL != "/" {
		q.Set("return", returnURL)
	}
	if encoded := q.Encode(); encoded != "" {
		target += "?" + encoded
	}
	return target
}

func withHighlight(target string, id int64) string {
	target = safeReturnURL(target)
	parsed, err := url.Parse(target)
	if err != nil {
		return "/"
	}
	q := parsed.Query()
	q.Set("highlight", strconv.FormatInt(id, 10))
	q.Del("notice")
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

func withNotice(target, notice string) string {
	target = safeReturnURL(target)
	parsed, err := url.Parse(target)
	if err != nil {
		return "/"
	}
	q := parsed.Query()
	q.Set("notice", notice)
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

func redirectWithNotice(w http.ResponseWriter, r *http.Request, target, notice string) {
	redirect(w, r, withNotice(target, notice))
}

func highlightIDFromRequest(r *http.Request) int64 {
	id, err := strconv.ParseInt(r.URL.Query().Get("highlight"), 10, 64)
	if err != nil || id < 1 {
		return 0
	}
	return id
}

func parseOptionalDate(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func customValuesFromForm(r *http.Request, fields []document.CustomField) map[int64]string {
	values := make(map[int64]string, len(fields))
	for _, field := range fields {
		values[field.ID] = r.FormValue(customFieldInputName(field.ID))
	}
	return values
}

func tagRulesFromForm(r *http.Request, tagID int64) ([]document.TagRule, []int64, error) {
	deleteIDs, err := parseIDValues(r.Form["delete_rule"], "ungültige Regel-ID")
	if err != nil {
		return nil, nil, err
	}

	ids := r.Form["rule_id"]
	labels := r.Form["rule_label"]
	scopes := r.Form["rule_scope"]
	matchModes := r.Form["rule_match_mode"]
	keywords := r.Form["rule_keywords"]
	excludes := r.Form["rule_excludes"]
	count := maxLen(ids, labels, scopes, matchModes, keywords, excludes)
	rules := make([]document.TagRule, 0, count)
	for i := 0; i < count; i++ {
		id, err := parseOptionalPositiveInt(formValueAt(ids, i), "ungültige Regel-ID")
		if err != nil {
			return nil, nil, err
		}
		rules = append(rules, document.TagRule{
			ID:        id,
			TagID:     tagID,
			Label:     formValueAt(labels, i),
			Scope:     formValueAt(scopes, i),
			MatchMode: formValueAt(matchModes, i),
			Keywords:  splitRuleKeywordInput(formValueAt(keywords, i)),
			Excludes:  splitRuleKeywordInput(formValueAt(excludes, i)),
			Position:  i,
		})
	}
	return rules, deleteIDs, nil
}

func customFieldInputName(id int64) string {
	return "field_" + strconv.FormatInt(id, 10)
}

func parseIDValues(values []string, message string) ([]int64, error) {
	ids := make([]int64, 0, len(values))
	for _, value := range values {
		id, err := parseOptionalPositiveInt(value, message)
		if err != nil {
			return nil, err
		}
		if id > 0 {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func parseOptionalPositiveInt(value, message string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New(message)
	}
	return id, nil
}

func formValueAt(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return values[index]
}

func splitRuleKeywordInput(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	})
}

func maxLen(values ...[]string) int {
	max := 0
	for _, value := range values {
		if len(value) > max {
			max = len(value)
		}
	}
	return max
}

func exportIDs(r *http.Request) ([]int64, error) {
	values := r.URL.Query()["ids"]
	if len(values) == 0 {
		values = r.URL.Query()["id"]
	}
	return documentIDsFromValues(values, "ungültige Dokument-ID im Export")
}

func formDocumentIDs(r *http.Request) ([]int64, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	return documentIDsFromValues(r.Form["ids"], "ungültige Dokument-ID")
}

func documentIDsFromValues(values []string, errorMessage string) ([]int64, error) {
	var ids []int64
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil || id <= 0 {
				return nil, errors.New(errorMessage)
			}
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func uniqueDocumentIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	unique := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}
