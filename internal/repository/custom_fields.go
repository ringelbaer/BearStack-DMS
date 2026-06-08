// Datei verwaltet benutzerdefinierte Felder samt Definitionen, Werten und Sortierung.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/sqlutil"
)

var ErrCustomFieldLabelExists = errors.New("Feldname existiert bereits")

func (r *Repository) ListCustomFields(ctx context.Context) ([]document.CustomField, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, label, position, autocomplete_enabled, value_folder_min_documents
		FROM custom_fields
		ORDER BY position ASC, lower(label) ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fields []document.CustomField
	for rows.Next() {
		var field document.CustomField
		if err := rows.Scan(&field.ID, &field.Label, &field.Position, &field.AutocompleteEnabled, &field.ValueFolderMinDocuments); err != nil {
			return nil, err
		}
		field.ValueFolderMinDocuments = document.NormalizeCustomFieldValueFolderMinDocuments(field.ValueFolderMinDocuments)
		fields = append(fields, field)
	}
	return fields, rows.Err()
}

func (r *Repository) GetCustomField(ctx context.Context, id int64) (document.CustomField, error) {
	var field document.CustomField
	err := r.db.QueryRowContext(ctx, `
		SELECT id, label, position, autocomplete_enabled, value_folder_min_documents
		FROM custom_fields
		WHERE id = ?`, id).Scan(&field.ID, &field.Label, &field.Position, &field.AutocompleteEnabled, &field.ValueFolderMinDocuments)
	field.ValueFolderMinDocuments = document.NormalizeCustomFieldValueFolderMinDocuments(field.ValueFolderMinDocuments)
	return field, err
}

func (r *Repository) SaveCustomField(ctx context.Context, label string, autocompleteEnabled bool, valueFolderMinDocuments ...int) error {
	label, err := normalizeCustomFieldLabel(label)
	if err != nil {
		return err
	}
	minDocuments := document.CustomFieldValueFolderNever
	if len(valueFolderMinDocuments) > 0 {
		minDocuments = document.NormalizeCustomFieldValueFolderMinDocuments(valueFolderMinDocuments[0])
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO custom_fields(label, position, autocomplete_enabled, value_folder_min_documents)
		VALUES (?, COALESCE((SELECT MAX(position) + 1 FROM custom_fields), 0), ?, ?)
		ON CONFLICT(label) DO UPDATE SET
			label = excluded.label,
			autocomplete_enabled = excluded.autocomplete_enabled,
			value_folder_min_documents = excluded.value_folder_min_documents`,
		label,
		autocompleteEnabled,
		minDocuments,
	)
	return err
}

func (r *Repository) UpdateCustomField(ctx context.Context, id int64, label string, autocompleteEnabled bool, valueFolderMinDocuments ...int) error {
	label, err := normalizeCustomFieldLabel(label)
	if err != nil {
		return err
	}
	minDocuments := document.CustomFieldValueFolderNever
	if len(valueFolderMinDocuments) > 0 {
		minDocuments = document.NormalizeCustomFieldValueFolderMinDocuments(valueFolderMinDocuments[0])
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var conflictID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id
		FROM custom_fields
		WHERE label = ?
		  AND id != ?`, label, id).Scan(&conflictID)
	if err == nil {
		return ErrCustomFieldLabelExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE custom_fields
		SET label = ?, autocomplete_enabled = ?, value_folder_min_documents = ?
		WHERE id = ?`, label, autocompleteEnabled, minDocuments, id)
	if err != nil {
		return err
	}
	if err := requireAffected(result); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) UpdateCustomFieldAutocomplete(ctx context.Context, id int64, enabled bool) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE custom_fields
		SET autocomplete_enabled = ?
		WHERE id = ?`, enabled, id)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) CustomFieldValueSuggestions(ctx context.Context, fieldID int64, query string, limit int) ([]string, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var autocompleteEnabled bool
	if err := r.db.QueryRowContext(ctx, `
		SELECT autocomplete_enabled
		FROM custom_fields
		WHERE id = ?`, fieldID).Scan(&autocompleteEnabled); err != nil {
		return nil, err
	}
	if !autocompleteEnabled {
		return []string{}, nil
	}

	query = strings.Join(strings.Fields(strings.TrimSpace(query)), " ")
	args := []any{fieldID}
	filter := ""
	if query != "" {
		filter = "AND lower(v.value) LIKE '%' || lower(?) || '%'"
		args = append(args, query)
	}
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, `
		SELECT v.value
		FROM document_custom_values v
		JOIN documents d ON d.id = v.document_id AND d.deleted_at IS NULL
		WHERE v.field_id = ?
		  AND trim(v.value) != ''
		  `+filter+`
		GROUP BY v.value
		ORDER BY COUNT(*) DESC, lower(v.value) ASC
		LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r *Repository) CustomFieldValues(ctx context.Context, fieldID int64) ([]document.CustomFieldValue, error) {
	if _, err := r.GetCustomField(ctx, fieldID); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT v.value, COUNT(*) AS document_count
		FROM document_custom_values v
		JOIN documents d ON d.id = v.document_id AND d.deleted_at IS NULL
		WHERE v.field_id = ?
		  AND trim(v.value) != ''
		GROUP BY v.value
		ORDER BY lower(v.value) ASC`, fieldID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []document.CustomFieldValue
	for rows.Next() {
		var value document.CustomFieldValue
		if err := rows.Scan(&value.Value, &value.Count); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r *Repository) ListFolderCustomFieldValues(ctx context.Context, filter document.ListFilter) ([]document.CustomFieldValueFolder, error) {
	filter.Tags = cleanTagNames(filter.Tags)
	where, args := buildListWhere(filter)

	excludeSelectedFields := ""
	selectedFieldIDs := exactCustomFieldFilterIDs(filter.CustomFields)
	if len(selectedFieldIDs) > 0 {
		for _, fieldID := range selectedFieldIDs {
			args = append(args, fieldID)
		}
		excludeSelectedFields = " AND cf.id NOT IN (" + sqlutil.Placeholders(len(selectedFieldIDs)) + ")"
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT cf.id, cf.label, v.value, COUNT(DISTINCT d.id) AS document_count
		FROM custom_fields cf
		JOIN document_custom_values v ON v.field_id = cf.id AND trim(v.value) != ''
		JOIN documents d ON d.id = v.document_id
		`+where+`
		  AND cf.value_folder_min_documents IN (1, 5, 10, 20, 50)
		  `+excludeSelectedFields+`
		GROUP BY cf.id, cf.label, v.value, cf.value_folder_min_documents
		HAVING COUNT(DISTINCT d.id) >= cf.value_folder_min_documents
		ORDER BY lower(v.value) ASC, lower(cf.label) ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []document.CustomFieldValueFolder
	for rows.Next() {
		var folder document.CustomFieldValueFolder
		if err := rows.Scan(&folder.FieldID, &folder.FieldLabel, &folder.Value, &folder.Count); err != nil {
			return nil, err
		}
		folders = append(folders, folder)
	}
	return folders, rows.Err()
}

func (r *Repository) RenameCustomFieldValue(ctx context.Context, fieldID int64, oldValue, newValue string) (int, error) {
	oldValue = strings.Join(strings.Fields(strings.TrimSpace(oldValue)), " ")
	cleaned := cleanCustomValues(map[int64]string{fieldID: newValue})
	newValue, ok := cleaned[fieldID]
	if oldValue == "" || !ok || oldValue == newValue {
		return 0, nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var existingFieldID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM custom_fields WHERE id = ?`, fieldID).Scan(&existingFieldID); err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT d.id
		FROM documents d
		JOIN document_custom_values v ON v.document_id = d.id
		WHERE v.field_id = ?
		  AND v.value = ?
		  AND d.deleted_at IS NULL
		ORDER BY d.id ASC`, fieldID, oldValue)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, tx.Commit()
	}

	docs, err := documentsByIDTx(ctx, tx, ids)
	if err != nil {
		return 0, err
	}
	now := formatTime(time.Now().UTC())
	updated := 0
	for _, id := range ids {
		current, ok := docs[id]
		if !ok || current.DeletedAt != nil {
			continue
		}
		current.CustomValues[fieldID] = newValue
		current.SearchVersion = document.CurrentSearchVersion
		result, err := tx.ExecContext(ctx, `
			UPDATE documents
			SET search_version = ?, updated_at = ?
			WHERE id = ? AND deleted_at IS NULL`, current.SearchVersion, now, id)
		if err != nil {
			return 0, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		if affected == 0 {
			continue
		}
		if err := setCustomValuesTx(ctx, tx, id, []int64{fieldID}, current.CustomValues); err != nil {
			return 0, err
		}
		if err := replaceSearchIndex(ctx, tx, id, current); err != nil {
			return 0, err
		}
		updated++
	}

	return updated, tx.Commit()
}

func (r *Repository) DeleteCustomField(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM custom_fields WHERE id = ?`, id).Scan(&exists); err != nil {
		return err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT document_id
		FROM document_custom_values
		WHERE field_id = ?`, id)
	if err != nil {
		return err
	}
	var affectedDocIDs []int64
	for rows.Next() {
		var docID int64
		if err := rows.Scan(&docID); err != nil {
			_ = rows.Close()
			return err
		}
		affectedDocIDs = append(affectedDocIDs, docID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM custom_fields WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if err := requireAffected(result); err != nil {
		return err
	}

	now := formatTime(time.Now().UTC())
	if err := reindexDocumentsByIDTx(ctx, tx, affectedDocIDs, now); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *Repository) SetCustomValuesForDocuments(ctx context.Context, ids []int64, values map[int64]string) (int, error) {
	values = cleanCustomValues(values)
	if len(ids) == 0 || len(values) == 0 {
		return 0, nil
	}
	ids = uniqueInt64(ids)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	docs, err := documentsByIDTx(ctx, tx, ids)
	if err != nil {
		return 0, err
	}
	fieldIDs := make([]int64, 0, len(values))
	for fieldID := range values {
		fieldIDs = append(fieldIDs, fieldID)
	}

	now := formatTime(time.Now().UTC())
	updated := 0
	for _, id := range ids {
		current, ok := docs[id]
		if !ok {
			continue
		}
		if current.DeletedAt != nil {
			continue
		}
		for fieldID, value := range values {
			current.CustomValues[fieldID] = value
		}
		current.SearchVersion = document.CurrentSearchVersion

		result, err := tx.ExecContext(ctx, `
			UPDATE documents
			SET search_version = ?, updated_at = ?
			WHERE id = ? AND deleted_at IS NULL`, current.SearchVersion, now, id)
		if err != nil {
			return 0, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		if affected == 0 {
			continue
		}
		if err := setCustomValuesTx(ctx, tx, id, fieldIDs, values); err != nil {
			return 0, err
		}
		if err := replaceSearchIndex(ctx, tx, id, current); err != nil {
			return 0, err
		}
		updated++
	}

	return updated, tx.Commit()
}

func normalizeCustomFieldLabel(label string) (string, error) {
	label = strings.Join(strings.Fields(strings.TrimSpace(label)), " ")
	if label == "" {
		return "", errors.New("Feldname fehlt")
	}
	return truncateString(label, 80), nil
}

func (r *Repository) attachCustomValues(ctx context.Context, docs []document.Document) error {
	if len(docs) == 0 {
		return nil
	}
	index := make(map[int64]int, len(docs))
	for i := range docs {
		index[docs[i].ID] = i
		if docs[i].CustomValues == nil {
			docs[i].CustomValues = map[int64]string{}
		}
	}

	return forDocumentBatches(docs, func(placeholders string, args []any) error {
		rows, err := r.db.QueryContext(ctx, `
			SELECT document_id, field_id, value
			FROM document_custom_values
			WHERE document_id IN (`+placeholders+`)`, args...)
		if err != nil {
			return err
		}

		for rows.Next() {
			var docID, fieldID int64
			var value string
			if err := rows.Scan(&docID, &fieldID, &value); err != nil {
				_ = rows.Close()
				return err
			}
			docs[index[docID]].CustomValues[fieldID] = value
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		return nil
	})
}

func customValuesForDocumentTx(ctx context.Context, tx *sql.Tx, id int64) (map[int64]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT field_id, value
		FROM document_custom_values
		WHERE document_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := map[int64]string{}
	for rows.Next() {
		var fieldID int64
		var value string
		if err := rows.Scan(&fieldID, &value); err != nil {
			return nil, err
		}
		values[fieldID] = value
	}
	return values, rows.Err()
}

func attachCustomValuesTx(ctx context.Context, tx *sql.Tx, docs []document.Document) error {
	if len(docs) == 0 {
		return nil
	}
	index := make(map[int64]int, len(docs))
	for i := range docs {
		index[docs[i].ID] = i
		if docs[i].CustomValues == nil {
			docs[i].CustomValues = map[int64]string{}
		}
	}
	placeholders, args := documentIDPlaceholders(docs)
	rows, err := tx.QueryContext(ctx, `
		SELECT document_id, field_id, value
		FROM document_custom_values
		WHERE document_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var docID, fieldID int64
		var value string
		if err := rows.Scan(&docID, &fieldID, &value); err != nil {
			return err
		}
		docs[index[docID]].CustomValues[fieldID] = value
	}
	return rows.Err()
}

func setCustomValuesTx(ctx context.Context, tx *sql.Tx, docID int64, fieldIDs []int64, values map[int64]string) error {
	if len(fieldIDs) == 0 {
		return nil
	}
	args := make([]any, len(fieldIDs)+1)
	args[0] = docID
	for i, fieldID := range fieldIDs {
		args[i+1] = fieldID
	}
	placeholders := sqlutil.Placeholders(len(fieldIDs))
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM document_custom_values
		WHERE document_id = ? AND field_id IN (`+placeholders+`)`, args...); err != nil {
		return err
	}
	for _, fieldID := range fieldIDs {
		value, ok := values[fieldID]
		if !ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO document_custom_values(document_id, field_id, value)
			SELECT ?, id, ?
			FROM custom_fields
			WHERE id = ?`, docID, value, fieldID); err != nil {
			return err
		}
	}
	return nil
}

func replaceCustomValues(ctx context.Context, tx *sql.Tx, docID int64, values map[int64]string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM document_custom_values WHERE document_id = ?`, docID); err != nil {
		return err
	}
	for fieldID, value := range cleanCustomValues(values) {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO document_custom_values(document_id, field_id, value)
			SELECT ?, id, ?
			FROM custom_fields
			WHERE id = ?`, docID, value, fieldID); err != nil {
			return err
		}
	}
	return nil
}

func cleanCustomValues(values map[int64]string) map[int64]string {
	cleaned := make(map[int64]string, len(values))
	for fieldID, value := range values {
		if fieldID <= 0 {
			continue
		}
		value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
		if value == "" {
			continue
		}
		value = truncateString(value, 2000)
		cleaned[fieldID] = value
	}
	return cleaned
}
