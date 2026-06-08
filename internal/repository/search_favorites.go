// Datei speichert Suchfavoriten und stellt deren CRUD-Operationen bereit.
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"bearstack/internal/document"
)

var (
	ErrSearchFavoriteNameExists    = errors.New("Suchfavorit existiert bereits")
	ErrSearchFavoriteNameMissing   = errors.New("Name fehlt")
	ErrSearchFavoriteQueryTooShort = errors.New("Suchwort muss mindestens 3 Zeichen haben")
	ErrSearchFavoriteInvalidYear   = errors.New("ungültiges Jahr")
	ErrSearchFavoriteEmptyCriteria = errors.New("mindestens Suchwort, Tag oder Zeitraum auswählen")
)

func (r *Repository) ListSearchFavorites(ctx context.Context) ([]document.SearchFavorite, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, query, tags, custom_fields, date_mode, date_year
		FROM search_favorites
		ORDER BY lower(name) ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var favorites []document.SearchFavorite
	for rows.Next() {
		favorite, err := scanSearchFavorite(rows)
		if err != nil {
			return nil, err
		}
		favorites = append(favorites, favorite)
	}
	return favorites, rows.Err()
}

func (r *Repository) GetSearchFavorite(ctx context.Context, id int64) (document.SearchFavorite, error) {
	if id <= 0 {
		return document.SearchFavorite{}, sql.ErrNoRows
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, query, tags, custom_fields, date_mode, date_year
		FROM search_favorites
		WHERE id = ?`, id)
	return scanSearchFavorite(row)
}

func (r *Repository) CreateSearchFavorite(ctx context.Context, favorite document.SearchFavorite) (int64, error) {
	normalized, err := normalizeSearchFavorite(favorite)
	if err != nil {
		return 0, err
	}
	if err := r.ensureSearchFavoriteNameAvailable(ctx, 0, normalized.Name); err != nil {
		return 0, err
	}
	tagsJSON, err := searchFavoriteTagsJSON(normalized.Tags)
	if err != nil {
		return 0, err
	}
	customFieldsJSON, err := searchFavoriteCustomFieldsJSON(normalized.CustomFields)
	if err != nil {
		return 0, err
	}
	now := formatTime(time.Now().UTC())
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO search_favorites(name, query, tags, custom_fields, date_mode, date_year, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		normalized.Name,
		normalized.Query,
		tagsJSON,
		customFieldsJSON,
		normalized.DateMode,
		normalized.DateYear,
		now,
		now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *Repository) UpdateSearchFavorite(ctx context.Context, id int64, favorite document.SearchFavorite) error {
	if id <= 0 {
		return sql.ErrNoRows
	}
	normalized, err := normalizeSearchFavorite(favorite)
	if err != nil {
		return err
	}
	if err := r.ensureSearchFavoriteNameAvailable(ctx, id, normalized.Name); err != nil {
		return err
	}
	tagsJSON, err := searchFavoriteTagsJSON(normalized.Tags)
	if err != nil {
		return err
	}
	customFieldsJSON, err := searchFavoriteCustomFieldsJSON(normalized.CustomFields)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE search_favorites
		SET name = ?, query = ?, tags = ?, custom_fields = ?, date_mode = ?, date_year = ?, updated_at = ?
		WHERE id = ?`,
		normalized.Name,
		normalized.Query,
		tagsJSON,
		customFieldsJSON,
		normalized.DateMode,
		normalized.DateYear,
		formatTime(time.Now().UTC()),
		id,
	)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) DeleteSearchFavorite(ctx context.Context, id int64) (document.SearchFavorite, error) {
	favorite, err := r.GetSearchFavorite(ctx, id)
	if err != nil {
		return document.SearchFavorite{}, err
	}
	result, err := r.db.ExecContext(ctx, `DELETE FROM search_favorites WHERE id = ?`, id)
	if err != nil {
		return document.SearchFavorite{}, err
	}
	if err := requireAffected(result); err != nil {
		return document.SearchFavorite{}, err
	}
	return favorite, nil
}

func (r *Repository) ensureSearchFavoriteNameAvailable(ctx context.Context, id int64, name string) error {
	var existingID int64
	err := r.db.QueryRowContext(ctx, `
		SELECT id
		FROM search_favorites
		WHERE lower(name) = lower(?)
		  AND id != ?`, name, id).Scan(&existingID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return ErrSearchFavoriteNameExists
}

type searchFavoriteScanner interface {
	Scan(dest ...any) error
}

func scanSearchFavorite(scanner searchFavoriteScanner) (document.SearchFavorite, error) {
	var favorite document.SearchFavorite
	var tagsJSON string
	var customFieldsJSON string
	if err := scanner.Scan(&favorite.ID, &favorite.Name, &favorite.Query, &tagsJSON, &customFieldsJSON, &favorite.DateMode, &favorite.DateYear); err != nil {
		return document.SearchFavorite{}, err
	}
	favorite.Tags = searchFavoriteTagsFromJSON(tagsJSON)
	favorite.CustomFields = searchFavoriteCustomFieldsFromJSON(customFieldsJSON)
	return favorite, nil
}

func normalizeSearchFavorite(favorite document.SearchFavorite) (document.SearchFavorite, error) {
	favorite.Name = strings.Join(strings.Fields(strings.TrimSpace(favorite.Name)), " ")
	if favorite.Name == "" {
		return document.SearchFavorite{}, ErrSearchFavoriteNameMissing
	}
	favorite.Name = truncateString(favorite.Name, 80)
	favorite.Query = truncateString(strings.Join(strings.Fields(strings.TrimSpace(favorite.Query)), " "), 200)
	if favorite.Query != "" && len([]rune(favorite.Query)) < 3 {
		return document.SearchFavorite{}, ErrSearchFavoriteQueryTooShort
	}
	favorite.Tags = cleanTagNames(favorite.Tags)
	favorite.CustomFields = cleanSearchFavoriteCustomFieldFilters(favorite.CustomFields)
	favorite.DateMode = normalizeSearchFavoriteDateMode(favorite.DateMode)
	if favorite.DateMode != document.SearchFavoriteDateYear {
		favorite.DateYear = 0
	} else if favorite.DateYear < 1900 || favorite.DateYear > 2100 {
		return document.SearchFavorite{}, ErrSearchFavoriteInvalidYear
	}
	if favorite.Query == "" && len(favorite.Tags) == 0 && len(favorite.CustomFields) == 0 && favorite.DateMode == document.SearchFavoriteDateNone {
		return document.SearchFavorite{}, ErrSearchFavoriteEmptyCriteria
	}
	return favorite, nil
}

func normalizeSearchFavoriteDateMode(value string) string {
	switch strings.TrimSpace(value) {
	case document.SearchFavoriteDateYear:
		return document.SearchFavoriteDateYear
	case document.SearchFavoriteDateThisMonth:
		return document.SearchFavoriteDateThisMonth
	case document.SearchFavoriteDateLastMonth:
		return document.SearchFavoriteDateLastMonth
	case document.SearchFavoriteDateThisYear:
		return document.SearchFavoriteDateThisYear
	case document.SearchFavoriteDateLastYear:
		return document.SearchFavoriteDateLastYear
	case document.SearchFavoriteDateThisQuarter:
		return document.SearchFavoriteDateThisQuarter
	case document.SearchFavoriteDateLastQuarter:
		return document.SearchFavoriteDateLastQuarter
	case document.SearchFavoriteDateThisHalf:
		return document.SearchFavoriteDateThisHalf
	case document.SearchFavoriteDateLastHalf:
		return document.SearchFavoriteDateLastHalf
	case document.SearchFavoriteDateLast7Days:
		return document.SearchFavoriteDateLast7Days
	case document.SearchFavoriteDateLast30Days:
		return document.SearchFavoriteDateLast30Days
	case document.SearchFavoriteDateLast90Days:
		return document.SearchFavoriteDateLast90Days
	case document.SearchFavoriteDateLast365Days:
		return document.SearchFavoriteDateLast365Days
	default:
		return document.SearchFavoriteDateNone
	}
}

func searchFavoriteTagsJSON(tags []string) (string, error) {
	if tags == nil {
		tags = []string{}
	}
	data, err := json.Marshal(tags)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func searchFavoriteTagsFromJSON(value string) []string {
	var tags []string
	if err := json.Unmarshal([]byte(value), &tags); err != nil {
		return nil
	}
	return cleanTagNames(tags)
}

type searchFavoriteCustomFieldJSON struct {
	FieldID int64  `json:"field_id"`
	Value   string `json:"value"`
}

func searchFavoriteCustomFieldsJSON(filters []document.CustomFieldFilter) (string, error) {
	filters = cleanSearchFavoriteCustomFieldFilters(filters)
	values := make([]searchFavoriteCustomFieldJSON, 0, len(filters))
	for _, filter := range filters {
		values = append(values, searchFavoriteCustomFieldJSON{
			FieldID: filter.FieldID,
			Value:   filter.Value,
		})
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func searchFavoriteCustomFieldsFromJSON(value string) []document.CustomFieldFilter {
	var values []searchFavoriteCustomFieldJSON
	if err := json.Unmarshal([]byte(value), &values); err != nil {
		return nil
	}
	filters := make([]document.CustomFieldFilter, 0, len(values))
	for _, value := range values {
		filters = append(filters, document.CustomFieldFilter{
			FieldID: value.FieldID,
			Value:   value.Value,
		})
	}
	return cleanSearchFavoriteCustomFieldFilters(filters)
}

func cleanSearchFavoriteCustomFieldFilters(filters []document.CustomFieldFilter) []document.CustomFieldFilter {
	if len(filters) == 0 {
		return nil
	}
	byID := make(map[int64]string, len(filters))
	for _, filter := range filters {
		value := document.CleanCustomFieldFilterValue(filter.Value)
		if filter.FieldID <= 0 || value == "" {
			continue
		}
		byID[filter.FieldID] = truncateString(value, 2000)
	}
	if len(byID) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	cleaned := make([]document.CustomFieldFilter, 0, len(ids))
	for _, id := range ids {
		cleaned = append(cleaned, document.CustomFieldFilter{
			FieldID: id,
			Value:   byID[id],
		})
	}
	return cleaned
}
