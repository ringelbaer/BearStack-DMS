// Datei behandelt Speichern, Laden und Loeschen von Suchfavoriten.
package server

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/repository"
)

func (s *Server) handleSearchFavorites(w http.ResponseWriter, r *http.Request) {
	favorites, err := s.repo.ListSearchFavorites(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	tags, err := s.repo.ListTags(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	fields, err := s.repo.ListCustomFields(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	s.render(w, r, "search_favorites.html", PageData{
		Title:               "Suchfavoriten",
		Active:              "search-favorites",
		SearchFavorites:     searchFavoriteViews(favorites, time.Now()),
		SearchFavoriteDates: searchFavoriteDateOptions(),
		Tags:                tags,
		TagDescriptions:     tagDescriptionMap(tags),
		TagStyles:           tagStyleMap(tags),
		CustomFields:        fields,
		Notice:              r.URL.Query().Get("notice"),
	})
}

func (s *Server) handleSaveSearchFavorite(w http.ResponseWriter, r *http.Request) {
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	favorite := searchFavoriteFromForm(r)
	id, err := s.repo.CreateSearchFavorite(r.Context(), favorite)
	if err != nil {
		s.renderSearchFavoriteError(w, r, err)
		return
	}
	setAuditTarget(r, namedAuditTarget("Suchfavorit", id, favorite.Name))
	redirectWithNotice(w, r, "/search-favorites", "Suchfavorit gespeichert.")
}

func (s *Server) handleUpdateSearchFavorite(w http.ResponseWriter, r *http.Request) {
	id, err := positiveIDFromPath(r, "id")
	if err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("ungültige Suchfavorit-ID"), "/search-favorites")
		return
	}
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	favorite := searchFavoriteFromForm(r)
	if err := s.repo.UpdateSearchFavorite(r.Context(), id, favorite); err != nil {
		s.renderSearchFavoriteError(w, r, err)
		return
	}
	setAuditTarget(r, namedAuditTarget("Suchfavorit", id, favorite.Name))
	redirectWithNotice(w, r, "/search-favorites", "Suchfavorit gespeichert.")
}

func (s *Server) handleDeleteSearchFavorite(w http.ResponseWriter, r *http.Request) {
	id, err := positiveIDFromPath(r, "id")
	if err != nil {
		s.renderErrorWithReturn(w, r, http.StatusBadRequest, errors.New("ungültige Suchfavorit-ID"), "/search-favorites")
		return
	}
	deleted, err := s.repo.DeleteSearchFavorite(r.Context(), id)
	if err != nil {
		s.renderSearchFavoriteError(w, r, err)
		return
	}
	setAuditTarget(r, namedAuditTarget("Suchfavorit", deleted.ID, deleted.Name))
	redirectWithNotice(w, r, "/search-favorites", "Suchfavorit gelöscht.")
}

func (s *Server) renderSearchFavoriteError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		s.renderSearchFavoriteErrorPage(w, r, http.StatusNotFound, errors.New("Suchfavorit nicht gefunden"))
		return
	}
	if errors.Is(err, repository.ErrSearchFavoriteNameExists) ||
		errors.Is(err, repository.ErrSearchFavoriteNameMissing) ||
		errors.Is(err, repository.ErrSearchFavoriteQueryTooShort) ||
		errors.Is(err, repository.ErrSearchFavoriteInvalidYear) ||
		errors.Is(err, repository.ErrSearchFavoriteEmptyCriteria) {
		s.renderSearchFavoriteErrorPage(w, r, http.StatusBadRequest, err)
		return
	}
	s.renderHTTPError(w, r, err)
}

func (s *Server) renderSearchFavoriteErrorPage(w http.ResponseWriter, r *http.Request, status int, err error) {
	s.renderErrorWithReturn(w, r, status, err, "/search-favorites")
}

func searchFavoriteFromForm(r *http.Request) document.SearchFavorite {
	year, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("date_year")))
	return document.SearchFavorite{
		Name:         r.FormValue("name"),
		Query:        r.FormValue("query"),
		Tags:         normalizeTagValues(r.Form["tags"], ""),
		CustomFields: customFieldFiltersFromQuery(r.Form),
		DateMode:     r.FormValue("date_mode"),
		DateYear:     year,
	}
}

func searchFavoriteViews(favorites []document.SearchFavorite, now time.Time) []SearchFavoriteView {
	views := make([]SearchFavoriteView, 0, len(favorites))
	for _, favorite := range favorites {
		dateLabel := searchFavoriteDateLabel(favorite)
		views = append(views, SearchFavoriteView{
			SearchFavorite: favorite,
			URL:            searchFavoriteURL(favorite, now),
			DateLabel:      dateLabel,
			Summary:        searchFavoriteSummary(favorite, dateLabel),
		})
	}
	return views
}

func searchFavoriteURL(favorite document.SearchFavorite, now time.Time) string {
	q := url.Values{}
	if favorite.Query != "" {
		q.Set("q", favorite.Query)
	}
	for _, tag := range favorite.Tags {
		q.Add("tags", tag)
	}
	for _, fieldFilter := range favorite.CustomFields {
		value := document.CleanCustomFieldFilterValue(fieldFilter.Value)
		if fieldFilter.FieldID <= 0 || value == "" {
			continue
		}
		q.Set("field_"+strconv.FormatInt(fieldFilter.FieldID, 10), value)
	}
	if favorite.DateMode == document.SearchFavoriteDateYear && favorite.DateYear > 0 {
		q.Set("year", strconv.Itoa(favorite.DateYear))
	} else if from, to := searchFavoriteDynamicRange(favorite.DateMode, now); from != nil && to != nil {
		q.Set("from", from.Format("2006-01-02"))
		q.Set("to", to.Format("2006-01-02"))
	}
	if encoded := q.Encode(); encoded != "" {
		return "/?" + encoded
	}
	return "/"
}

func searchFavoriteDateLabel(favorite document.SearchFavorite) string {
	if favorite.DateMode == document.SearchFavoriteDateYear {
		if favorite.DateYear > 0 {
			return strconv.Itoa(favorite.DateYear)
		}
	}
	for _, option := range searchFavoriteDateOptions() {
		if option.Value == favorite.DateMode {
			return option.Label
		}
	}
	return ""
}

func searchFavoriteDateOptions() []SearchFavoriteDateOption {
	return []SearchFavoriteDateOption{
		{Value: document.SearchFavoriteDateNone, Label: "Kein Zeitraum"},
		{Value: document.SearchFavoriteDateYear, Label: "Festes Jahr"},
		{Value: document.SearchFavoriteDateThisMonth, Label: "Dieser Monat"},
		{Value: document.SearchFavoriteDateLastMonth, Label: "Letzter Monat"},
		{Value: document.SearchFavoriteDateThisQuarter, Label: "Dieses Quartal"},
		{Value: document.SearchFavoriteDateLastQuarter, Label: "Letztes Quartal"},
		{Value: document.SearchFavoriteDateThisHalf, Label: "Dieses Halbjahr"},
		{Value: document.SearchFavoriteDateLastHalf, Label: "Letztes Halbjahr"},
		{Value: document.SearchFavoriteDateThisYear, Label: "Dieses Jahr"},
		{Value: document.SearchFavoriteDateLastYear, Label: "Letztes Jahr"},
		{Value: document.SearchFavoriteDateLast7Days, Label: "Letzte 7 Tage"},
		{Value: document.SearchFavoriteDateLast30Days, Label: "Letzte 30 Tage"},
		{Value: document.SearchFavoriteDateLast90Days, Label: "Letzte 90 Tage"},
		{Value: document.SearchFavoriteDateLast365Days, Label: "Letzte 365 Tage"},
	}
}

func searchFavoriteSummary(favorite document.SearchFavorite, dateLabel string) string {
	return searchFavoriteSummaryForDisplay(favorite, dateLabel, tagDisplayModeLower, nil)
}

func searchFavoriteSummaryForDisplay(favorite document.SearchFavorite, dateLabel, tagDisplayMode string, fields []document.CustomField) string {
	parts := make([]string, 0, 4)
	if favorite.Query != "" {
		parts = append(parts, "Suche: "+favorite.Query)
	}
	if len(favorite.Tags) > 0 {
		parts = append(parts, "Tags: "+joinDisplayTags(tagDisplayMode, favorite.Tags))
	}
	if customFields := searchFavoriteCustomFieldSummary(favorite.CustomFields, fields); customFields != "" {
		parts = append(parts, customFields)
	}
	if dateLabel != "" {
		parts = append(parts, dateLabel)
	}
	if len(parts) == 0 {
		return "Alle Dokumente"
	}
	return strings.Join(parts, " · ")
}

func searchFavoriteCustomFieldSummary(filters []document.CustomFieldFilter, fields []document.CustomField) string {
	if len(filters) == 0 {
		return ""
	}
	labels := make(map[int64]string, len(fields))
	for _, field := range fields {
		labels[field.ID] = field.Label
	}
	parts := make([]string, 0, len(filters))
	for _, filter := range filters {
		value := document.CleanCustomFieldFilterValue(filter.Value)
		if filter.FieldID <= 0 || value == "" {
			continue
		}
		label := labels[filter.FieldID]
		if label == "" {
			label = "Feld " + strconv.FormatInt(filter.FieldID, 10)
		}
		parts = append(parts, label+": "+value)
	}
	if len(parts) == 0 {
		return ""
	}
	return "Felder: " + strings.Join(parts, ", ")
}

func searchFavoriteDynamicRange(mode string, now time.Time) (*time.Time, *time.Time) {
	if now.IsZero() {
		now = time.Now()
	}
	current := now.In(time.Local)
	switch mode {
	case document.SearchFavoriteDateThisMonth:
		return monthRange(current.Year(), current.Month())
	case document.SearchFavoriteDateLastMonth:
		year, month := previousMonth(current.Year(), current.Month())
		return monthRange(year, month)
	case document.SearchFavoriteDateThisYear:
		return yearRange(current.Year())
	case document.SearchFavoriteDateLastYear:
		return yearRange(current.Year() - 1)
	case document.SearchFavoriteDateThisQuarter:
		return quarterRange(current.Year(), quarterStartMonth(current.Month()))
	case document.SearchFavoriteDateLastQuarter:
		year := current.Year()
		month := quarterStartMonth(current.Month()) - 3
		if month < time.January {
			month += 12
			year--
		}
		return quarterRange(year, month)
	case document.SearchFavoriteDateThisHalf:
		return halfYearRange(current.Year(), halfYearStartMonth(current.Month()))
	case document.SearchFavoriteDateLastHalf:
		year := current.Year()
		month := halfYearStartMonth(current.Month()) - 6
		if month < time.January {
			month += 12
			year--
		}
		return halfYearRange(year, month)
	case document.SearchFavoriteDateLast7Days:
		return trailingDayRange(current, 7)
	case document.SearchFavoriteDateLast30Days:
		return trailingDayRange(current, 30)
	case document.SearchFavoriteDateLast90Days:
		return trailingDayRange(current, 90)
	case document.SearchFavoriteDateLast365Days:
		return trailingDayRange(current, 365)
	default:
		return nil, nil
	}
}

func monthRange(year int, month time.Month) (*time.Time, *time.Time) {
	from := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC)
	return &from, &to
}

func previousMonth(year int, month time.Month) (int, time.Month) {
	month--
	if month < time.January {
		return year - 1, time.December
	}
	return year, month
}

func yearRange(year int) (*time.Time, *time.Time) {
	from := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC)
	return &from, &to
}

func quarterStartMonth(month time.Month) time.Month {
	return time.Month(((int(month)-1)/3)*3 + 1)
}

func quarterRange(year int, startMonth time.Month) (*time.Time, *time.Time) {
	from := time.Date(year, startMonth, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(year, startMonth+3, 0, 0, 0, 0, 0, time.UTC)
	return &from, &to
}

func halfYearStartMonth(month time.Month) time.Month {
	if month <= time.June {
		return time.January
	}
	return time.July
}

func halfYearRange(year int, startMonth time.Month) (*time.Time, *time.Time) {
	from := time.Date(year, startMonth, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(year, startMonth+6, 0, 0, 0, 0, 0, time.UTC)
	return &from, &to
}

func trailingDayRange(now time.Time, days int) (*time.Time, *time.Time) {
	if days < 1 {
		return nil, nil
	}
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	from := to.AddDate(0, 0, -(days - 1))
	return &from, &to
}

func searchFavoriteAuditTarget(r *http.Request) string {
	return idAuditTarget("Suchfavorit", r.PathValue("id"))
}
