// Datei baut Dokumentabfragen mit Filtern, Sortierung und Paginierung fuer Listenansichten.
package repository

import (
	"strings"

	"bearstack/internal/document"
	"bearstack/internal/searchtext"
	"bearstack/internal/sqlutil"
)

var documentSearchColumns = []string{"original_name", "title", "description", "tags", "search_text"}

func buildListQuery(filter document.ListFilter) (string, []any) {
	whereClause, args := buildListWhere(filter)

	query := summarySelect() + "\n" + whereClause + "\nORDER BY " + buildListOrderBy(filter)
	if filter.Limit > 0 {
		query += "\nLIMIT ?"
		args = append(args, filter.Limit)
		if filter.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, filter.Offset)
		}
	}
	return query, args
}

func buildListOrderBy(filter document.ListFilter) string {
	sort := document.NormalizeListSort(filter.Sort)
	direction := strings.ToUpper(document.NormalizeListDirection(filter.Direction, sort))

	switch sort {
	case document.ListSortName:
		return "lower(d.original_name) " + direction + ", d.id " + direction
	case document.ListSortTitle:
		return "lower(d.title) " + direction + ", d.id " + direction
	case document.ListSortDate:
		return "COALESCE(d.document_date, substr(d.uploaded_at, 1, 10)) " + direction + ", d.uploaded_at " + direction + ", d.id " + direction
	case document.ListSortSize:
		return "d.size_bytes " + direction + ", d.id " + direction
	case document.ListSortDeletedAt:
		return "d.deleted_at " + direction + ", d.id " + direction
	default:
		return "d.uploaded_at " + direction + ", d.id " + direction
	}
}

func buildListWhere(filter document.ListFilter) (string, []any) {
	where, args := buildDocumentFilter(filter)
	return "WHERE " + strings.Join(where, " AND "), args
}

func buildDocumentFilter(filter document.ListFilter) ([]string, []any) {
	var where []string
	var args []any

	if filter.Trash {
		where = append(where, "d.deleted_at IS NOT NULL")
	} else {
		where = append(where, "d.deleted_at IS NULL")
	}

	if filter.Query != "" {
		tokens := documentSearchTokens(filter.Query)
		ftsQuery := buildFTSQuery(filter.Query)
		likeQuery, likeArgs := documentSearchLikePredicate(tokens)
		switch {
		case ftsQuery != "":
			where = append(where, "d.id IN (SELECT rowid FROM document_search WHERE document_search MATCH ?)")
			args = append(args, ftsQuery)
		case likeQuery != "":
			where = append(where, "d.id IN (SELECT rowid FROM document_search WHERE "+likeQuery+")")
			args = append(args, likeArgs...)
		}
	}

	appendTagFilter(&where, &args, filter.Tags)
	appendCustomFieldFilter(&where, &args, filter.CustomFields)

	if filter.From != nil {
		where = append(where, "d.document_date >= ?")
		args = append(args, filter.From.Format("2006-01-02"))
	}
	if filter.To != nil {
		where = append(where, "d.document_date <= ?")
		args = append(args, filter.To.Format("2006-01-02"))
	}
	return where, args
}

func documentSearchLikePredicate(tokens []string) (string, []any) {
	tokenClauses := make([]string, 0, len(tokens))
	args := make([]any, 0, len(tokens)*len(documentSearchColumns))
	for _, token := range tokens {
		folded := searchtext.GermanFold(token)
		if folded == "" {
			continue
		}
		columnClauses := make([]string, 0, len(documentSearchColumns))
		pattern := searchtext.LikeContainsPattern(folded)
		for _, column := range documentSearchColumns {
			columnClauses = append(columnClauses, "bearstack_german_fold("+column+") LIKE ? ESCAPE '\\'")
			args = append(args, pattern)
		}
		tokenClauses = append(tokenClauses, "("+strings.Join(columnClauses, " OR ")+")")
	}
	return strings.Join(tokenClauses, " AND "), args
}

func appendTagFilter(where *[]string, args *[]any, tags []string) {
	tags = cleanTagNames(tags)
	if len(tags) == 0 {
		return
	}
	for _, tag := range tags {
		*args = append(*args, tag)
	}
	*args = append(*args, len(tags))
	placeholders := sqlutil.Placeholders(len(tags))
	*where = append(*where, `
		d.id IN (
			SELECT fdt.document_id
			FROM document_tags fdt
			JOIN tags ft ON ft.id = fdt.tag_id
			WHERE ft.name IN (`+placeholders+`)
			GROUP BY fdt.document_id
			HAVING COUNT(DISTINCT ft.name) = ?
		)`)
}

func appendCustomFieldFilter(where *[]string, args *[]any, filters []document.CustomFieldFilter) {
	for _, filter := range filters {
		value := document.CleanCustomFieldFilterValue(filter.Value)
		if filter.FieldID <= 0 || value == "" {
			continue
		}
		*where = append(*where, `
			EXISTS (
				SELECT 1
				FROM document_custom_values fcv
				WHERE fcv.document_id = d.id
				  AND fcv.field_id = ?
				  AND `+customFieldFilterValuePredicate(filter)+`
			)`)
		*args = append(*args, filter.FieldID, value)
	}
}

func customFieldFilterValuePredicate(filter document.CustomFieldFilter) string {
	if filter.Exact {
		return "fcv.value = ?"
	}
	return "lower(fcv.value) LIKE '%' || lower(?) || '%'"
}

func hasCustomFieldFilter(filters []document.CustomFieldFilter) bool {
	for _, filter := range filters {
		if filter.FieldID > 0 && document.CleanCustomFieldFilterValue(filter.Value) != "" {
			return true
		}
	}
	return false
}

func exactCustomFieldFilterIDs(filters []document.CustomFieldFilter) []int64 {
	seen := make(map[int64]struct{}, len(filters))
	ids := make([]int64, 0, len(filters))
	for _, filter := range filters {
		if !filter.Exact || filter.FieldID <= 0 || document.CleanCustomFieldFilterValue(filter.Value) == "" {
			continue
		}
		if _, ok := seen[filter.FieldID]; ok {
			continue
		}
		seen[filter.FieldID] = struct{}{}
		ids = append(ids, filter.FieldID)
	}
	return ids
}

func buildCountQuery(filter document.ListFilter) (string, []any) {
	where, args := buildDocumentFilter(filter)

	var query strings.Builder
	query.WriteString("SELECT COUNT(*) FROM documents d")
	query.WriteString("\nWHERE ")
	query.WriteString(strings.Join(where, " AND "))
	return query.String(), args
}
