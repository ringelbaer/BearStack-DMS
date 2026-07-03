// Datei parst Filterparameter aus HTTP-Anfragen und wandelt sie in Repository-Abfragen um.
package server

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"bearstack/internal/document"
)

func filterFromRequest(r *http.Request, trash bool, perPage int) document.ListFilter {
	q := r.URL.Query()
	perPage = normalizeDocumentPageSize(perPage)
	page := parsePositiveInt(q.Get("page"), 1)
	query := strings.TrimSpace(q.Get("q"))
	if len([]rune(query)) < 3 {
		query = ""
	}
	listSort := document.NormalizeListSort(q.Get("sort"))
	filter := document.ListFilter{
		Query:        query,
		Tags:         normalizeTagValues(q["tags"], ""),
		CustomFields: customFieldFiltersFromQuery(q),
		Sort:         listSort,
		Direction:    document.NormalizeListDirection(q.Get("dir"), listSort),
		Trash:        trash,
		Limit:        perPage,
		Offset:       (page - 1) * perPage,
		Page:         page,
	}
	if year := parseFilterYear(q.Get("year")); year > 0 {
		filter.Year = year
		month := parseFilterMonth(q.Get("month"))
		filter.Month = month
		fromMonth := time.January
		toMonth := time.December
		toDay := 31
		if month > 0 {
			fromMonth = time.Month(month)
			toMonth = time.Month(month)
			toDay = time.Date(year, toMonth+1, 0, 0, 0, 0, 0, time.UTC).Day()
		}
		from := time.Date(year, fromMonth, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(year, toMonth, toDay, 0, 0, 0, 0, time.UTC)
		filter.From = &from
		filter.To = &to
	} else {
		if from := parseDateQuery(q.Get("from")); from != nil {
			filter.From = from
		}
		if to := parseDateQuery(q.Get("to")); to != nil {
			filter.To = to
		}
	}
	return filter
}

func customFieldFiltersFromQuery(values map[string][]string) []document.CustomFieldFilter {
	filters := make([]document.CustomFieldFilter, 0)
	for key, rawValues := range values {
		if !strings.HasPrefix(key, "field_") {
			continue
		}
		fieldID, err := strconv.ParseInt(strings.TrimPrefix(key, "field_"), 10, 64)
		if err != nil || fieldID <= 0 {
			continue
		}
		value := firstCustomFieldFilterValue(rawValues)
		if value == "" {
			continue
		}
		filters = append(filters, document.CustomFieldFilter{
			FieldID: fieldID,
			Value:   value,
		})
	}
	sort.Slice(filters, func(i, j int) bool {
		return filters[i].FieldID < filters[j].FieldID
	})
	return filters
}

func firstCustomFieldFilterValue(values []string) string {
	for _, value := range values {
		value = document.CleanCustomFieldFilterValue(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func documentSortLinks(r *http.Request, filter document.ListFilter) map[string]SortLink {
	currentSort := document.NormalizeListSort(filter.Sort)
	currentDirection := document.NormalizeListDirection(filter.Direction, currentSort)
	keys := []string{
		document.ListSortName,
		document.ListSortTitle,
		document.ListSortDate,
		document.ListSortUploadDate,
		document.ListSortSize,
		document.ListSortDeletedAt,
	}
	links := make(map[string]SortLink, len(keys))
	for _, key := range keys {
		active := key == currentSort
		nextDirection := document.DefaultListDirection(key)
		direction := document.NormalizeListDirection("", key)
		if active {
			direction = currentDirection
			nextDirection = document.ToggleListDirection(currentDirection)
		}
		links[key] = SortLink{
			URL:       sortURL(r, key, nextDirection),
			Active:    active,
			Direction: direction,
			AriaSort:  sortAria(active, direction),
		}
	}
	return links
}

func sortURL(r *http.Request, sort, direction string) string {
	q := clearQueryKeys(r.URL.Query(), "notice", "highlight", "page")
	q.Set("sort", document.NormalizeListSort(sort))
	q.Set("dir", document.NormalizeListDirection(direction, sort))
	return pathWithQuery(r.URL.Path, q)
}

func sortAria(active bool, direction string) string {
	if !active {
		return "none"
	}
	if direction == document.ListDirectionAscending {
		return "ascending"
	}
	return "descending"
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

func parseFilterYear(value string) int {
	year, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || year < 1900 || year > 2100 {
		return 0
	}
	return year
}

func parseFilterMonth(value string) int {
	month, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || month < 1 || month > 12 {
		return 0
	}
	return month
}

func documentYearLinks(r *http.Request, years []int, filter document.ListFilter) ([]DateLink, []DateLink) {
	links := make([]DateLink, 0, len(years))
	for _, year := range years {
		links = append(links, DateLink{
			Label:  strconv.Itoa(year),
			URL:    dateFilterURL(r, year, 0),
			Active: filter.Year == year,
		})
	}
	if len(links) <= 3 {
		return links, nil
	}
	visible := append([]DateLink(nil), links[:3]...)
	overflow := append([]DateLink(nil), links[3:]...)
	for i, link := range overflow {
		if !link.Active {
			continue
		}
		visible = append(visible, link)
		overflow = append(overflow[:i], overflow[i+1:]...)
		break
	}
	return visible, overflow
}

func documentMonthLinks(r *http.Request, months []int, filter document.ListFilter) []DateLink {
	if filter.Year == 0 {
		return nil
	}
	links := make([]DateLink, 0, len(months))
	for _, month := range months {
		links = append(links, DateLink{
			Label:  germanMonthShort(time.Month(month)),
			URL:    dateFilterURL(r, filter.Year, month),
			Active: filter.Month == month,
		})
	}
	return links
}

func dateFilterURL(r *http.Request, year, month int) string {
	q := clearQueryKeys(r.URL.Query(), "notice", "highlight", "page", "from", "to")
	q.Set("year", strconv.Itoa(year))
	if month > 0 {
		q.Set("month", strconv.Itoa(month))
	} else {
		q.Del("month")
	}
	return pathWithQuery(r.URL.Path, q)
}

func dateFilterResetURL(r *http.Request) string {
	q := clearQueryKeys(r.URL.Query(), "notice", "highlight", "page", "from", "to", "year", "month")
	return pathWithQuery(r.URL.Path, q)
}

func documentFilterActive(filter document.ListFilter) bool {
	if filter.Query != "" || filter.From != nil || filter.To != nil {
		return true
	}
	if len(normalizeTagValues(filter.Tags, "")) > 0 {
		return true
	}
	for _, customField := range filter.CustomFields {
		if customField.FieldID > 0 && document.CleanCustomFieldFilterValue(customField.Value) != "" {
			return true
		}
	}
	return false
}

func germanMonthShort(month time.Month) string {
	names := [...]string{"Jan", "Feb", "Mrz", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"}
	if month < time.January || month > time.December {
		return ""
	}
	return names[month-1]
}

func datesForFilter(filter document.ListFilter) filterDates {
	var dates filterDates
	if filter.From != nil {
		dates.From = filter.From.Format("2006-01-02")
	}
	if filter.To != nil {
		dates.To = filter.To.Format("2006-01-02")
	}
	return dates
}

func parseDateQuery(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil
	}
	return &parsed
}
