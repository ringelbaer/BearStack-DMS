// Datei verwaltet Dokument-Tags, Tag-Farben und deren Zuordnung zu Dokumenten.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/sqlutil"
	"bearstack/internal/tagutil"
)

type autoTagRule struct {
	TagName         string
	Scope           string
	MatchMode       string
	Keywords        []string
	ExcludeKeywords []string
}

const (
	defaultTagCloudItemLimit    = 200
	defaultTagCloudRelatedLimit = 18
)

var (
	ErrTagNameExists  = errors.New("Tag existiert bereits")
	ErrTagNameMissing = errors.New("Tagname fehlt")
)

func (r *Repository) ListTags(ctx context.Context) ([]document.Tag, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.id, t.name, t.description, t.color, t.primary_tag, t.group_mode, t.list_hidden, t.delete_protected, COUNT(d.id)
		FROM tags t
		LEFT JOIN document_tags dt ON dt.tag_id = t.id
		LEFT JOIN documents d ON d.id = dt.document_id AND d.deleted_at IS NULL
		GROUP BY t.id, t.name, t.description, t.color, t.primary_tag, t.group_mode, t.list_hidden, t.delete_protected
		ORDER BY t.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []document.Tag
	for rows.Next() {
		var tag document.Tag
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.Description, &tag.Color, &tag.PrimaryTag, &tag.GroupMode, &tag.ListHidden, &tag.DeleteProtected, &tag.Count); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (r *Repository) TagCloud(ctx context.Context, itemLimit, relatedLimit int) (document.TagCloud, error) {
	if itemLimit < 1 {
		itemLimit = defaultTagCloudItemLimit
	}
	if relatedLimit < 1 {
		relatedLimit = defaultTagCloudRelatedLimit
	}

	primaryTags, err := r.primaryTagCloudItems(ctx)
	if err != nil {
		return document.TagCloud{}, err
	}
	if len(primaryTags) == 0 {
		items, err := r.centralTagCloudItems(ctx, itemLimit)
		if err != nil {
			return document.TagCloud{}, err
		}
		return document.TagCloud{
			Items:    items,
			MaxCount: maxTagCloudItemCount(items),
		}, nil
	}

	clusters := make([]document.TagCloudCluster, len(primaryTags))
	primaryIndex := make(map[int64]int, len(primaryTags))
	for i, tag := range primaryTags {
		primaryIndex[tag.ID] = i
		clusters[i].Primary = document.TagCloudItem{Tag: tag.Name, Count: tag.Count, Primary: true}
	}

	related, err := r.primaryRelatedTagCloudItems(ctx, relatedLimit)
	if err != nil {
		return document.TagCloud{}, err
	}
	for _, item := range related {
		index, ok := primaryIndex[item.PrimaryID]
		if !ok {
			continue
		}
		clusters[index].Items = append(clusters[index].Items, document.TagCloudItem{Tag: item.Tag, Count: item.Count})
		if item.Count > clusters[index].MaxCount {
			clusters[index].MaxCount = item.Count
		}
	}

	return document.TagCloud{
		HasPrimaryTags: true,
		Clusters:       clusters,
	}, nil
}

func (r *Repository) ListFolderTags(ctx context.Context, filter document.ListFilter) ([]document.Tag, error) {
	selectedTags := cleanTagNames(filter.Tags)
	filter.Tags = selectedTags
	if len(selectedTags) == 0 && !hasFolderDocumentFilter(filter) {
		return r.ListTags(ctx)
	}

	where, args := buildListWhere(filter)
	excludeSelected := ""
	if len(selectedTags) > 0 {
		for _, tag := range selectedTags {
			args = append(args, tag)
		}
		excludeSelected = " AND t.name NOT IN (" + sqlutil.Placeholders(len(selectedTags)) + ")"
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT t.id, t.name, t.description, t.color, t.primary_tag, t.group_mode, t.list_hidden, t.delete_protected, COUNT(DISTINCT d.id)
		FROM tags t
		JOIN document_tags dt ON dt.tag_id = t.id
		JOIN documents d ON d.id = dt.document_id
		`+where+excludeSelected+`
		GROUP BY t.id, t.name, t.description, t.color, t.primary_tag, t.group_mode, t.list_hidden, t.delete_protected
		HAVING COUNT(DISTINCT d.id) > 0
		ORDER BY t.name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []document.Tag
	for rows.Next() {
		var tag document.Tag
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.Description, &tag.Color, &tag.PrimaryTag, &tag.GroupMode, &tag.ListHidden, &tag.DeleteProtected, &tag.Count); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (r *Repository) primaryTagCloudItems(ctx context.Context) ([]document.Tag, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.id, t.name, COUNT(DISTINCT d.id)
		FROM tags t
		LEFT JOIN document_tags dt ON dt.tag_id = t.id
		LEFT JOIN documents d ON d.id = dt.document_id AND d.deleted_at IS NULL
		WHERE t.primary_tag = 1
		GROUP BY t.id, t.name
		ORDER BY t.name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []document.Tag
	for rows.Next() {
		var tag document.Tag
		tag.PrimaryTag = true
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.Count); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (r *Repository) centralTagCloudItems(ctx context.Context, limit int) ([]document.TagCloudItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.name, COUNT(DISTINCT d.id) AS document_count
		FROM tags t
		JOIN document_tags dt ON dt.tag_id = t.id
		JOIN documents d ON d.id = dt.document_id AND d.deleted_at IS NULL
		GROUP BY t.id, t.name
		HAVING document_count > 0
		ORDER BY document_count DESC, t.name ASC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []document.TagCloudItem
	for rows.Next() {
		var item document.TagCloudItem
		if err := rows.Scan(&item.Tag, &item.Count); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type primaryRelatedTagCloudItem struct {
	PrimaryID int64
	Tag       string
	Count     int
}

func (r *Repository) primaryRelatedTagCloudItems(ctx context.Context, limit int) ([]primaryRelatedTagCloudItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		WITH related_counts AS (
			SELECT pt.id AS primary_id, rt.name AS tag_name, COUNT(DISTINCT d.id) AS document_count
			FROM tags pt
			JOIN document_tags pdt ON pdt.tag_id = pt.id
			JOIN documents d ON d.id = pdt.document_id AND d.deleted_at IS NULL
			JOIN document_tags rdt ON rdt.document_id = d.id
			JOIN tags rt ON rt.id = rdt.tag_id
			WHERE pt.primary_tag = 1
			  AND rt.primary_tag = 0
			  AND rt.id != pt.id
			GROUP BY pt.id, rt.id, rt.name
		),
		ranked AS (
			SELECT primary_id, tag_name, document_count,
			       ROW_NUMBER() OVER (
				       PARTITION BY primary_id
				       ORDER BY document_count DESC, tag_name ASC
			       ) AS rank
			FROM related_counts
		)
		SELECT primary_id, tag_name, document_count
		FROM ranked
		WHERE rank <= ?
		ORDER BY primary_id ASC, rank ASC`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []primaryRelatedTagCloudItem
	for rows.Next() {
		var item primaryRelatedTagCloudItem
		if err := rows.Scan(&item.PrimaryID, &item.Tag, &item.Count); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func maxTagCloudItemCount(items []document.TagCloudItem) int {
	maximum := 0
	for _, item := range items {
		if item.Count > maximum {
			maximum = item.Count
		}
	}
	return maximum
}

func hasFolderDocumentFilter(filter document.ListFilter) bool {
	return filter.Query != "" || hasCustomFieldFilter(filter.CustomFields) || filter.From != nil || filter.To != nil || filter.Trash
}

func (r *Repository) GetTag(ctx context.Context, id int64) (document.Tag, error) {
	var tag document.Tag
	err := r.db.QueryRowContext(ctx, `
		SELECT t.id, t.name, t.description, t.color, t.primary_tag, t.group_mode, t.list_hidden, t.delete_protected, COUNT(d.id)
		FROM tags t
		LEFT JOIN document_tags dt ON dt.tag_id = t.id
		LEFT JOIN documents d ON d.id = dt.document_id AND d.deleted_at IS NULL
		WHERE t.id = ?
		GROUP BY t.id, t.name, t.description, t.color, t.primary_tag, t.group_mode, t.list_hidden, t.delete_protected`, id).
		Scan(&tag.ID, &tag.Name, &tag.Description, &tag.Color, &tag.PrimaryTag, &tag.GroupMode, &tag.ListHidden, &tag.DeleteProtected, &tag.Count)
	return tag, err
}

func (r *Repository) GetTagByName(ctx context.Context, name string) (document.Tag, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return document.Tag{}, sql.ErrNoRows
	}
	var tag document.Tag
	err := r.db.QueryRowContext(ctx, `
		SELECT t.id, t.name, t.description, t.color, t.primary_tag, t.group_mode, t.list_hidden, t.delete_protected, COUNT(d.id)
		FROM tags t
		LEFT JOIN document_tags dt ON dt.tag_id = t.id
		LEFT JOIN documents d ON d.id = dt.document_id AND d.deleted_at IS NULL
		WHERE t.name = ?
		GROUP BY t.id, t.name, t.description, t.color, t.primary_tag, t.group_mode, t.list_hidden, t.delete_protected`, name).
		Scan(&tag.ID, &tag.Name, &tag.Description, &tag.Color, &tag.PrimaryTag, &tag.GroupMode, &tag.ListHidden, &tag.DeleteProtected, &tag.Count)
	return tag, err
}

func (r *Repository) SaveTag(ctx context.Context, name, description, color string, groupMode, listHidden bool, tagFlags ...bool) (int64, error) {
	name = cleanSingleTagName(name)
	if name == "" {
		return 0, ErrTagNameMissing
	}
	color = tagutil.NormalizeColor(color)
	deleteProtected := false
	if len(tagFlags) > 0 {
		deleteProtected = tagFlags[0]
	}
	primaryTag := false
	if len(tagFlags) > 1 {
		primaryTag = tagFlags[1]
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tags(name, description, color, primary_tag, group_mode, list_hidden, delete_protected)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			description = excluded.description,
			color = excluded.color,
			primary_tag = excluded.primary_tag,
			group_mode = excluded.group_mode,
			list_hidden = excluded.list_hidden,
			delete_protected = excluded.delete_protected`,
		name,
		strings.TrimSpace(description),
		color,
		primaryTag,
		groupMode,
		listHidden,
		deleteProtected,
	); err != nil {
		return 0, err
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM tags WHERE name = ?`, name).Scan(&id); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func (r *Repository) RenameTag(ctx context.Context, id int64, name, description, color string, groupMode, listHidden bool, tagFlags ...bool) (document.Tag, error) {
	if id <= 0 {
		return document.Tag{}, sql.ErrNoRows
	}
	name = cleanSingleTagName(name)
	if name == "" {
		return document.Tag{}, ErrTagNameMissing
	}
	color = tagutil.NormalizeColor(color)
	deleteProtected := false
	if len(tagFlags) > 0 {
		deleteProtected = tagFlags[0]
	}
	primaryTag := false
	if len(tagFlags) > 1 {
		primaryTag = tagFlags[1]
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return document.Tag{}, err
	}
	defer tx.Rollback()

	current, err := tagByIDTx(ctx, tx, id)
	if err != nil {
		return document.Tag{}, err
	}
	nameChanged := current.Name != name
	if nameChanged {
		if err := ensureTagNameAvailableTx(ctx, tx, id, name); err != nil {
			return document.Tag{}, err
		}
	}

	var affectedDocIDs []int64
	if nameChanged {
		affectedDocIDs, err = documentIDsForTagTx(ctx, tx, id)
		if err != nil {
			return document.Tag{}, err
		}
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE tags
		SET name = ?, description = ?, color = ?, primary_tag = ?, group_mode = ?, list_hidden = ?, delete_protected = ?
		WHERE id = ?`,
		name,
		strings.TrimSpace(description),
		color,
		primaryTag,
		groupMode,
		listHidden,
		deleteProtected,
		id,
	)
	if err != nil {
		return document.Tag{}, err
	}
	if err := requireAffected(result); err != nil {
		return document.Tag{}, err
	}

	if nameChanged {
		now := formatTime(time.Now().UTC())
		if err := renameSearchFavoriteTagTx(ctx, tx, current.Name, name, now); err != nil {
			return document.Tag{}, err
		}
		if err := reindexDocumentsByIDTx(ctx, tx, affectedDocIDs, now); err != nil {
			return document.Tag{}, err
		}
	}

	updated, err := tagByIDTx(ctx, tx, id)
	if err != nil {
		return document.Tag{}, err
	}
	if err := tx.Commit(); err != nil {
		return document.Tag{}, err
	}
	return updated, nil
}

func (r *Repository) DeleteTag(ctx context.Context, id int64) (document.Tag, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return document.Tag{}, err
	}
	defer tx.Rollback()

	tag, err := tagByIDTx(ctx, tx, id)
	if err != nil {
		return document.Tag{}, err
	}

	affectedDocIDs, err := documentIDsForTagTx(ctx, tx, id)
	if err != nil {
		return document.Tag{}, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM tag_rules WHERE tag_id = ?`, id); err != nil {
		return document.Tag{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM document_tags WHERE tag_id = ?`, id); err != nil {
		return document.Tag{}, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, id)
	if err != nil {
		return document.Tag{}, err
	}
	if err := requireAffected(result); err != nil {
		return document.Tag{}, err
	}

	now := formatTime(time.Now().UTC())
	if err := reindexDocumentsByIDTx(ctx, tx, affectedDocIDs, now); err != nil {
		return document.Tag{}, err
	}

	if err := tx.Commit(); err != nil {
		return document.Tag{}, err
	}
	return tag, nil
}

func (r *Repository) ListTagRules(ctx context.Context, tagID int64) ([]document.TagRule, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tag_id, label, scope, match_mode, keywords, exclude_keywords, position
		FROM tag_rules
		WHERE tag_id = ?
		ORDER BY position ASC, id ASC`, tagID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []document.TagRule
	for rows.Next() {
		var rule document.TagRule
		var keywords, excludes string
		if err := rows.Scan(&rule.ID, &rule.TagID, &rule.Label, &rule.Scope, &rule.MatchMode, &keywords, &excludes, &rule.Position); err != nil {
			return nil, err
		}
		rule.Keywords = splitRuleKeywords(keywords)
		rule.Excludes = splitRuleKeywords(excludes)
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (r *Repository) SaveTagRules(ctx context.Context, tagID int64, rules []document.TagRule, deleteIDs []int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM tags WHERE id = ?`, tagID).Scan(&exists); err != nil {
		return err
	}

	for _, id := range deleteIDs {
		if id <= 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM tag_rules WHERE id = ? AND tag_id = ?`, id, tagID); err != nil {
			return err
		}
	}

	position := 0
	deleted := idSet(deleteIDs)
	for _, rule := range rules {
		if rule.ID > 0 {
			if _, ok := deleted[rule.ID]; ok {
				continue
			}
		}
		rule.TagID = tagID
		rule.Position = position
		normalized, ok := normalizeTagRule(rule)
		if !ok {
			if rule.ID > 0 {
				if _, err := tx.ExecContext(ctx, `DELETE FROM tag_rules WHERE id = ? AND tag_id = ?`, rule.ID, tagID); err != nil {
					return err
				}
			}
			continue
		}
		keywords := strings.Join(normalized.Keywords, "\n")
		excludes := strings.Join(normalized.Excludes, "\n")
		if normalized.ID > 0 {
			result, err := tx.ExecContext(ctx, `
				UPDATE tag_rules
				SET label = ?, scope = ?, match_mode = ?, keywords = ?, exclude_keywords = ?, position = ?
				WHERE id = ? AND tag_id = ?`,
				normalized.Label,
				normalized.Scope,
				normalized.MatchMode,
				keywords,
				excludes,
				position,
				normalized.ID,
				tagID,
			)
			if err != nil {
				return err
			}
			if err := requireAffected(result); err != nil {
				return err
			}
		} else {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO tag_rules(tag_id, label, scope, match_mode, keywords, exclude_keywords, position)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				tagID,
				normalized.Label,
				normalized.Scope,
				normalized.MatchMode,
				keywords,
				excludes,
				position,
			); err != nil {
				return err
			}
		}
		position++
	}

	return tx.Commit()
}

func tagsForDocumentTx(ctx context.Context, tx *sql.Tx, id int64) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT t.name
		FROM document_tags dt
		JOIN tags t ON t.id = dt.tag_id
		WHERE dt.document_id = ?
		ORDER BY t.name`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func tagIDsByNameTx(ctx context.Context, tx *sql.Tx, tags []string) ([]int64, error) {
	tags = cleanTagNames(tags)
	if len(tags) == 0 {
		return nil, nil
	}
	args := make([]any, len(tags))
	for i, tag := range tags {
		args[i] = tag
	}
	placeholders := sqlutil.Placeholders(len(tags))
	rows, err := tx.QueryContext(ctx, `
		SELECT id
		FROM tags
		WHERE name IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func tagByIDTx(ctx context.Context, tx *sql.Tx, id int64) (document.Tag, error) {
	if id <= 0 {
		return document.Tag{}, sql.ErrNoRows
	}
	var tag document.Tag
	err := tx.QueryRowContext(ctx, `
		SELECT t.id, t.name, t.description, t.color, t.primary_tag, t.group_mode, t.list_hidden, t.delete_protected, COUNT(d.id)
		FROM tags t
		LEFT JOIN document_tags dt ON dt.tag_id = t.id
		LEFT JOIN documents d ON d.id = dt.document_id AND d.deleted_at IS NULL
		WHERE t.id = ?
		GROUP BY t.id, t.name, t.description, t.color, t.primary_tag, t.group_mode, t.list_hidden, t.delete_protected`, id).
		Scan(&tag.ID, &tag.Name, &tag.Description, &tag.Color, &tag.PrimaryTag, &tag.GroupMode, &tag.ListHidden, &tag.DeleteProtected, &tag.Count)
	return tag, err
}

func ensureTagNameAvailableTx(ctx context.Context, tx *sql.Tx, id int64, name string) error {
	var existingID int64
	err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM tags
		WHERE name = ?
		  AND id != ?`, name, id).Scan(&existingID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return ErrTagNameExists
}

func documentIDsForTagTx(ctx context.Context, tx *sql.Tx, tagID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT document_id
		FROM document_tags
		WHERE tag_id = ?`, tagID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func renameSearchFavoriteTagTx(ctx context.Context, tx *sql.Tx, oldName, newName, updatedAt string) error {
	if oldName == newName {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, tags
		FROM search_favorites`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type favoriteTagUpdate struct {
		id   int64
		tags string
	}
	var updates []favoriteTagUpdate
	for rows.Next() {
		var id int64
		var tagsJSON string
		if err := rows.Scan(&id, &tagsJSON); err != nil {
			return err
		}
		tags := searchFavoriteTagsFromJSON(tagsJSON)
		changed := false
		for i, tag := range tags {
			if tag != oldName {
				continue
			}
			tags[i] = newName
			changed = true
		}
		if !changed {
			continue
		}
		nextTagsJSON, err := searchFavoriteTagsJSON(cleanTagNames(tags))
		if err != nil {
			return err
		}
		updates = append(updates, favoriteTagUpdate{id: id, tags: nextTagsJSON})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, update := range updates {
		if _, err := tx.ExecContext(ctx, `
			UPDATE search_favorites
			SET tags = ?, updated_at = ?
			WHERE id = ?`, update.tags, updatedAt, update.id); err != nil {
			return err
		}
	}
	return nil
}

func appendMissingTags(existing, additional []string) []string {
	merged := append([]string(nil), existing...)
	seen := make(map[string]struct{}, len(existing)+len(additional))
	for _, tag := range existing {
		seen[tag] = struct{}{}
	}
	for _, tag := range cleanTagNames(additional) {
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		merged = append(merged, tag)
	}
	return merged
}

func replaceTags(ctx context.Context, tx *sql.Tx, docID int64, tags []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM document_tags WHERE document_id = ?`, docID); err != nil {
		return err
	}
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags(name) VALUES (?)`, tag); err != nil {
			return err
		}
		var tagID int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM tags WHERE name = ?`, tag).Scan(&tagID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO document_tags(document_id, tag_id)
			VALUES (?, ?)`,
			docID,
			tagID,
		); err != nil {
			return err
		}
	}
	return nil
}

func autoTagValuesForNewDocumentTx(ctx context.Context, tx *sql.Tx, doc document.Document) ([]string, error) {
	rules, err := autoTagRulesTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	tags := cleanTagNames(doc.Tags)
	seen := make(map[string]struct{}, len(tags)+len(rules))
	for _, tag := range tags {
		seen[tag] = struct{}{}
	}
	for _, tag := range matchingAutoTags(doc, rules) {
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	return tags, nil
}

func autoTagRulesTx(ctx context.Context, tx *sql.Tx) ([]autoTagRule, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT t.name, tr.scope, tr.match_mode, tr.keywords, tr.exclude_keywords
		FROM tag_rules tr
		JOIN tags t ON t.id = tr.tag_id
		ORDER BY tr.tag_id, tr.position, tr.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []autoTagRule
	for rows.Next() {
		var rule autoTagRule
		var keywords, excludes string
		if err := rows.Scan(&rule.TagName, &rule.Scope, &rule.MatchMode, &keywords, &excludes); err != nil {
			return nil, err
		}
		rule.Keywords = splitRuleKeywords(keywords)
		rule.ExcludeKeywords = splitRuleKeywords(excludes)
		if len(rule.Keywords) == 0 {
			continue
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func matchingAutoTags(doc document.Document, rules []autoTagRule) []string {
	var matches []string
	if len(rules) == 0 {
		return matches
	}
	seen := make(map[string]struct{}, len(rules))
	filename := normalizedRuleText(doc.OriginalName)
	text := normalizedRuleText(doc.ContentText)
	for _, rule := range rules {
		if ruleMatches(rule, filename, text) {
			if _, ok := seen[rule.TagName]; ok {
				continue
			}
			seen[rule.TagName] = struct{}{}
			matches = append(matches, rule.TagName)
		}
	}
	return matches
}

func ruleMatches(rule autoTagRule, filename, text string) bool {
	haystacks := ruleHaystacks(rule.Scope, filename, text)
	if len(haystacks) == 0 || len(rule.Keywords) == 0 {
		return false
	}
	for _, keyword := range rule.ExcludeKeywords {
		if containsAny(haystacks, keyword) {
			return false
		}
	}

	if rule.MatchMode == document.TagRuleMatchAll {
		for _, keyword := range rule.Keywords {
			if !containsAny(haystacks, keyword) {
				return false
			}
		}
		return true
	}

	for _, keyword := range rule.Keywords {
		if containsAny(haystacks, keyword) {
			return true
		}
	}
	return false
}

func ruleHaystacks(scope, filename, text string) []string {
	switch scope {
	case document.TagRuleScopeFilename:
		if filename == "" {
			return nil
		}
		return []string{filename}
	case document.TagRuleScopeText:
		if text == "" {
			return nil
		}
		return []string{text}
	default:
		var haystacks []string
		if filename != "" {
			haystacks = append(haystacks, filename)
		}
		if text != "" {
			haystacks = append(haystacks, text)
		}
		return haystacks
	}
}

func containsAny(haystacks []string, keyword string) bool {
	for _, haystack := range haystacks {
		if strings.Contains(haystack, keyword) {
			return true
		}
	}
	return false
}

func normalizeTagRule(rule document.TagRule) (document.TagRule, bool) {
	rule.Label = strings.Join(strings.Fields(strings.TrimSpace(rule.Label)), " ")
	rule.Label = truncateString(rule.Label, 100)
	if rule.Scope != document.TagRuleScopeFilename && rule.Scope != document.TagRuleScopeText {
		rule.Scope = document.TagRuleScopeBoth
	}
	if rule.MatchMode != document.TagRuleMatchAll {
		rule.MatchMode = document.TagRuleMatchAny
	}
	rule.Keywords = normalizeRuleKeywords(rule.Keywords)
	rule.Excludes = normalizeRuleKeywords(rule.Excludes)
	return rule, len(rule.Keywords) > 0
}

func splitRuleKeywords(input string) []string {
	parts := strings.FieldsFunc(input, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	})
	return normalizeRuleKeywords(parts)
}

func normalizeRuleKeywords(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	keywords := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizedRuleText(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		keywords = append(keywords, truncateString(value, 120))
		if len(keywords) == 50 {
			break
		}
	}
	return keywords
}

func normalizedRuleText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func idSet(values []int64) map[int64]struct{} {
	set := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value > 0 {
			set[value] = struct{}{}
		}
	}
	return set
}

func cleanTagNames(values []string) []string {
	return tagutil.Normalize(values)
}

func cleanSingleTagName(value string) string {
	tags := cleanTagNames([]string{value})
	if len(tags) == 0 {
		return ""
	}
	return tags[0]
}
