// Datei berechnet Repository-Statistiken fuer Dokumente, Speicher und Bearbeitungszustand.
package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/sqlutil"
)

func (r *Repository) Statistics(ctx context.Context) (document.Statistics, error) {
	var stats document.Statistics
	now := time.Now().UTC()
	since := now.AddDate(0, 0, -30)

	if err := r.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN deleted_at IS NULL THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN deleted_at IS NULL THEN size_bytes ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN deleted_at IS NOT NULL THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN deleted_at IS NOT NULL THEN size_bytes ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN deleted_at IS NULL AND uploaded_at >= ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN deleted_at IS NULL AND trim(content_text) != '' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN deleted_at IS NULL AND document_date IS NOT NULL THEN 1 ELSE 0 END), 0)
		FROM documents`, formatTime(since)).
		Scan(
			&stats.ActiveDocuments,
			&stats.TotalBytes,
			&stats.TrashDocuments,
			&stats.TrashBytes,
			&stats.UploadedLast30Days,
			&stats.DocumentsWithOCRText,
			&stats.DocumentsWithDocumentDate,
		); err != nil {
		return document.Statistics{}, err
	}

	stats.TotalDocuments = stats.ActiveDocuments + stats.TrashDocuments
	if stats.ActiveDocuments > 0 {
		stats.AverageBytes = stats.TotalBytes / int64(stats.ActiveDocuments)
	}
	stats.OCRCoveragePercent = percent(stats.DocumentsWithOCRText, stats.ActiveDocuments)
	stats.DocumentDateCoveragePercent = percent(stats.DocumentsWithDocumentDate, stats.ActiveDocuments)
	stats.TrashPercent = percent(stats.TrashDocuments, stats.TotalDocuments)

	if err := r.loadDuplicateStatistics(ctx, &stats); err != nil {
		return document.Statistics{}, err
	}
	if err := r.loadMonthlyUploads(ctx, &stats, now); err != nil {
		return document.Statistics{}, err
	}
	if err := r.loadDocumentDateStatistics(ctx, &stats); err != nil {
		return document.Statistics{}, err
	}
	if err := r.loadUploadWayStatistics(ctx, &stats); err != nil {
		return document.Statistics{}, err
	}
	if err := r.loadFileTypeStatistics(ctx, &stats); err != nil {
		return document.Statistics{}, err
	}
	if err := r.loadTopTagStatistics(ctx, &stats); err != nil {
		return document.Statistics{}, err
	}
	if err := r.loadTagUsageTimeline(ctx, &stats); err != nil {
		return document.Statistics{}, err
	}
	if err := r.loadOCRStatusStatistics(ctx, &stats); err != nil {
		return document.Statistics{}, err
	}
	if err := r.loadOCRAttentionJobs(ctx, &stats); err != nil {
		return document.Statistics{}, err
	}
	if err := r.loadContentTextSourceStatistics(ctx, &stats); err != nil {
		return document.Statistics{}, err
	}
	if err := r.loadTextIssueDocuments(ctx, &stats, 12); err != nil {
		return document.Statistics{}, err
	}
	if err := r.loadDatabaseStatus(ctx, &stats); err != nil {
		return document.Statistics{}, err
	}
	return stats, nil
}

func (r *Repository) loadDatabaseStatus(ctx context.Context, stats *document.Statistics) error {
	status := document.DatabaseStatus{TargetSearchVersion: document.CurrentSearchVersion}
	var minVersion, maxVersion sql.NullInt64
	if err := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN search_version >= ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN search_version < ? THEN 1 ELSE 0 END), 0),
			MIN(search_version),
			MAX(search_version)
		FROM documents`,
		document.CurrentSearchVersion,
		document.CurrentSearchVersion,
	).Scan(
		&status.TotalDocuments,
		&status.CurrentSearchVersionDocuments,
		&status.OutdatedSearchVersionDocuments,
		&minVersion,
		&maxVersion,
	); err != nil {
		return err
	}
	if minVersion.Valid {
		status.MinSearchVersion = int(minVersion.Int64)
	}
	if maxVersion.Valid {
		status.MaxSearchVersion = int(maxVersion.Int64)
	}

	searchIndexTrigram, err := r.documentSearchUsesTrigram(ctx)
	if err != nil {
		return err
	}
	status.SearchIndexTrigram = searchIndexTrigram

	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM document_search`).Scan(&status.SearchIndexDocuments); err != nil {
		return err
	}

	stats.Database = status
	return nil
}

func (r *Repository) loadDuplicateStatistics(ctx context.Context, stats *document.Statistics) error {
	var duplicateDocuments sql.NullInt64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(count), 0)
		FROM (
			SELECT COUNT(*) AS count
			FROM documents
			WHERE deleted_at IS NULL
			GROUP BY sha256
			HAVING COUNT(*) > 1
		)`).Scan(&stats.DuplicateGroups, &duplicateDocuments); err != nil {
		return err
	}
	stats.DuplicateDocuments = int(nullInt64Value(duplicateDocuments))
	return nil
}

func (r *Repository) loadMonthlyUploads(ctx context.Context, stats *document.Statistics, now time.Time) error {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -11, 0)
	rows, err := r.db.QueryContext(ctx, `
		SELECT substr(uploaded_at, 1, 7), COUNT(*), COALESCE(SUM(size_bytes), 0)
		FROM documents
		WHERE deleted_at IS NULL AND uploaded_at >= ?
		GROUP BY substr(uploaded_at, 1, 7)`, formatTime(start))
	if err != nil {
		return err
	}
	defer rows.Close()

	byMonth := map[string]document.StatisticBucket{}
	for rows.Next() {
		var bucket document.StatisticBucket
		if err := rows.Scan(&bucket.Key, &bucket.Count, &bucket.Bytes); err != nil {
			return err
		}
		byMonth[bucket.Key] = bucket
	}
	if err := rows.Err(); err != nil {
		return err
	}

	stats.MonthlyUploads = make([]document.StatisticBucket, 0, 12)
	for i := 0; i < 12; i++ {
		month := start.AddDate(0, i, 0)
		key := month.Format("2006-01")
		bucket := byMonth[key]
		bucket.Key = key
		bucket.Label = germanMonthShort(month)
		stats.MonthlyUploads = append(stats.MonthlyUploads, bucket)
	}
	stats.MonthlyUploadsMax = maxBucketCount(stats.MonthlyUploads)
	return nil
}

func (r *Repository) loadDocumentDateStatistics(ctx context.Context, stats *document.Statistics) error {
	buckets, err := r.statisticBuckets(ctx, `
		SELECT
			CASE
				WHEN document_date IS NULL OR trim(document_date) = '' THEN 'missing'
				ELSE substr(document_date, 1, 4)
			END AS date_year,
			COUNT(*),
			COALESCE(SUM(size_bytes), 0)
		FROM documents
		WHERE deleted_at IS NULL
		GROUP BY date_year
		ORDER BY CASE WHEN date_year = 'missing' THEN 1 ELSE 0 END ASC, date_year DESC`)
	if err != nil {
		return err
	}
	for i := range buckets {
		buckets[i].Label = documentDateStatLabel(buckets[i].Key)
	}
	stats.DocumentDateYears = buckets
	stats.DocumentDateYearMax = maxBucketCount(buckets)
	return nil
}

func (r *Repository) loadUploadWayStatistics(ctx context.Context, stats *document.Statistics) error {
	buckets, err := r.statisticBuckets(ctx, `
		SELECT upload_way, COUNT(*), COALESCE(SUM(size_bytes), 0)
		FROM documents
		WHERE deleted_at IS NULL
		GROUP BY upload_way
		ORDER BY COUNT(*) DESC, upload_way ASC`)
	if err != nil {
		return err
	}
	for i := range buckets {
		buckets[i].Label = uploadWayStatLabel(buckets[i].Key)
	}
	stats.UploadWays = buckets
	stats.UploadWayMax = maxBucketCount(buckets)
	return nil
}

func (r *Repository) loadFileTypeStatistics(ctx context.Context, stats *document.Statistics) error {
	buckets, err := r.statisticBuckets(ctx, `
		SELECT
			CASE
				WHEN mime_type = 'application/pdf' THEN 'pdf'
				WHEN mime_type LIKE 'image/%' THEN 'image'
				ELSE 'other'
			END AS type,
			COUNT(*),
			COALESCE(SUM(size_bytes), 0)
		FROM documents
		WHERE deleted_at IS NULL
		GROUP BY type
		ORDER BY COUNT(*) DESC, type ASC`)
	if err != nil {
		return err
	}
	for i := range buckets {
		buckets[i].Label = fileTypeStatLabel(buckets[i].Key)
	}
	stats.FileTypes = buckets
	stats.FileTypeMax = maxBucketCount(buckets)
	return nil
}

func (r *Repository) loadTopTagStatistics(ctx context.Context, stats *document.Statistics) error {
	buckets, err := r.statisticBuckets(ctx, `
		SELECT t.name, COUNT(d.id), COALESCE(SUM(d.size_bytes), 0)
		FROM tags t
		JOIN document_tags dt ON dt.tag_id = t.id
		JOIN documents d ON d.id = dt.document_id AND d.deleted_at IS NULL
		GROUP BY t.id, t.name
		ORDER BY COUNT(d.id) DESC, t.name ASC
		LIMIT 8`)
	if err != nil {
		return err
	}
	for i := range buckets {
		buckets[i].Label = buckets[i].Key
	}
	stats.TopTags = buckets
	stats.TopTagMax = maxBucketCount(buckets)
	return nil
}

func (r *Repository) loadTagUsageTimeline(ctx context.Context, stats *document.Statistics) error {
	topTags, err := r.statisticBuckets(ctx, `
		SELECT t.name, COUNT(d.id), 0
		FROM tags t
		JOIN document_tags dt ON dt.tag_id = t.id
		JOIN documents d ON d.id = dt.document_id AND d.deleted_at IS NULL
		WHERE d.document_date IS NOT NULL AND trim(d.document_date) != ''
		GROUP BY t.id, t.name
		ORDER BY COUNT(d.id) DESC, t.name ASC
		LIMIT 8`)
	if err != nil {
		return err
	}
	if len(topTags) == 0 {
		return nil
	}

	args := make([]any, len(topTags))
	tagOrder := make(map[string]int, len(topTags))
	for i, bucket := range topTags {
		stats.TagUsageTags = append(stats.TagUsageTags, bucket.Key)
		args[i] = bucket.Key
		tagOrder[bucket.Key] = i
	}
	placeholders := sqlutil.Placeholders(len(topTags))

	rows, err := r.db.QueryContext(ctx, `
		SELECT substr(d.document_date, 1, 4) AS date_year, t.name, COUNT(d.id)
		FROM tags t
		JOIN document_tags dt ON dt.tag_id = t.id
		JOIN documents d ON d.id = dt.document_id AND d.deleted_at IS NULL
		WHERE d.document_date IS NOT NULL
		  AND trim(d.document_date) != ''
		  AND t.name IN (`+placeholders+`)
		GROUP BY date_year, t.id, t.name
		ORDER BY date_year ASC, t.name ASC`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	type tagYearCount struct {
		tag   string
		count int
	}
	years := make([]string, 0)
	byYear := make(map[string][]tagYearCount)
	for rows.Next() {
		var year, tag string
		var count int
		if err := rows.Scan(&year, &tag, &count); err != nil {
			return err
		}
		if _, ok := byYear[year]; !ok {
			years = append(years, year)
		}
		byYear[year] = append(byYear[year], tagYearCount{tag: tag, count: count})
	}
	if err := rows.Err(); err != nil {
		return err
	}

	stats.TagUsageYears = make([]document.TagUsageYear, 0, len(years))
	for _, year := range years {
		counts := make([]document.TagUsageSegment, len(topTags))
		for _, item := range byYear[year] {
			index, ok := tagOrder[item.tag]
			if !ok {
				continue
			}
			counts[index] = document.TagUsageSegment{Tag: item.tag, Count: item.count}
		}

		statYear := document.TagUsageYear{Year: year}
		for _, segment := range counts {
			if segment.Count <= 0 {
				continue
			}
			statYear.Total += segment.Count
			statYear.Segments = append(statYear.Segments, segment)
		}
		if statYear.Total > stats.TagUsageYearMax {
			stats.TagUsageYearMax = statYear.Total
		}
		stats.TagUsageYears = append(stats.TagUsageYears, statYear)
	}
	return nil
}

func (r *Repository) loadOCRStatusStatistics(ctx context.Context, stats *document.Statistics) error {
	buckets, err := r.statisticBuckets(ctx, `
		SELECT o.status, COUNT(*), 0
		FROM ocr_jobs o
		JOIN documents d ON d.id = o.document_id AND d.deleted_at IS NULL
		WHERE o.id IN (
			SELECT MAX(id)
			FROM ocr_jobs
			GROUP BY document_id
		)
		GROUP BY o.status
		ORDER BY COUNT(*) DESC, o.status ASC`)
	if err != nil {
		return err
	}
	for i := range buckets {
		buckets[i].Label = ocrStatusStatLabel(buckets[i].Key)
	}
	stats.OCRStatuses = buckets
	stats.OCRStatusMax = maxBucketCount(buckets)
	return nil
}

func (r *Repository) loadOCRAttentionJobs(ctx context.Context, stats *document.Statistics) error {
	rows, err := r.db.QueryContext(ctx, `
		SELECT o.id, o.document_id, o.language, o.language_label, o.status, o.current_page, o.total_pages,
			o.text_length, o.message, o.error, o.created_at, o.started_at, o.finished_at, o.updated_at,
			d.original_name, d.title
		FROM ocr_jobs o
		JOIN documents d ON d.id = o.document_id AND d.deleted_at IS NULL
		WHERE o.id IN (
			SELECT MAX(id)
			FROM ocr_jobs
			GROUP BY document_id
		)
		  AND o.status IN (?, ?, ?)
		ORDER BY CASE o.status
			WHEN ? THEN 0
			WHEN ? THEN 1
			WHEN ? THEN 2
			ELSE 3
		END, o.updated_at DESC, o.id DESC`,
		document.OCRJobStatusRunning,
		document.OCRJobStatusQueued,
		document.OCRJobStatusFailed,
		document.OCRJobStatusRunning,
		document.OCRJobStatusQueued,
		document.OCRJobStatusFailed,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var job document.OCRJobStatistic
		var createdAt string
		var startedAt sql.NullString
		var finishedAt sql.NullString
		var updatedAt string
		if err := rows.Scan(
			&job.ID,
			&job.DocumentID,
			&job.Language,
			&job.LanguageLabel,
			&job.Status,
			&job.CurrentPage,
			&job.TotalPages,
			&job.TextLength,
			&job.Message,
			&job.Error,
			&createdAt,
			&startedAt,
			&finishedAt,
			&updatedAt,
			&job.DocumentOriginalName,
			&job.DocumentTitle,
		); err != nil {
			return err
		}
		parsedCreatedAt, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return err
		}
		parsedUpdatedAt, err := time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return err
		}
		job.CreatedAt = parsedCreatedAt
		job.UpdatedAt = parsedUpdatedAt
		if startedAt.Valid {
			parsed, err := time.Parse(time.RFC3339, startedAt.String)
			if err != nil {
				return err
			}
			job.StartedAt = &parsed
		}
		if finishedAt.Valid {
			parsed, err := time.Parse(time.RFC3339, finishedAt.String)
			if err != nil {
				return err
			}
			job.FinishedAt = &parsed
		}
		stats.OCRAttentionJobs = append(stats.OCRAttentionJobs, job)
	}
	return rows.Err()
}

func (r *Repository) loadContentTextSourceStatistics(ctx context.Context, stats *document.Statistics) error {
	buckets, err := r.statisticBuckets(ctx, `
		SELECT
			CASE
				WHEN trim(content_text) = '' THEN ?
				WHEN content_text_source IN (?, ?, ?, ?, ?) THEN content_text_source
				ELSE ?
			END AS source,
			COUNT(*),
			0
		FROM documents
		WHERE deleted_at IS NULL
		GROUP BY source
		ORDER BY COUNT(*) DESC, source ASC`,
		document.ContentTextSourceNone,
		document.ContentTextSourcePDF,
		document.ContentTextSourceFile,
		document.ContentTextSourceRaw,
		document.ContentTextSourceOCR,
		document.ContentTextSourceUnknown,
		document.ContentTextSourceUnknown,
	)
	if err != nil {
		return err
	}
	for i := range buckets {
		buckets[i].Label = document.ContentTextSourceLabel(buckets[i].Key)
	}
	stats.ContentTextSources = buckets
	stats.ContentTextSourceMax = maxBucketCount(buckets)
	return nil
}

func (r *Repository) loadTextIssueDocuments(ctx context.Context, stats *document.Statistics, limit int) error {
	if limit <= 0 {
		limit = 12
	}
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM documents d
		WHERE `+textIssueWhereClause(),
		document.ContentTextSourceRaw,
		document.ContentTextSourceNone,
	).Scan(&stats.TextIssueDocumentCount); err != nil {
		return err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT d.id, d.original_name, d.title, d.mime_type, d.content_text_source, d.updated_at
		FROM documents d
		WHERE `+textIssueWhereClause()+`
		ORDER BY CASE d.content_text_source
			WHEN ? THEN 0
			WHEN ? THEN 1
			ELSE 2
		END, d.updated_at DESC, d.id DESC
		LIMIT ?`,
		document.ContentTextSourceRaw,
		document.ContentTextSourceNone,
		document.ContentTextSourceRaw,
		document.ContentTextSourceNone,
		limit,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var item document.TextIssueDocument
		var updatedAt string
		if err := rows.Scan(&item.ID, &item.OriginalName, &item.Title, &item.MIMEType, &item.ContentTextSource, &updatedAt); err != nil {
			return err
		}
		parsedUpdatedAt, err := time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return err
		}
		switch item.ContentTextSource {
		case document.ContentTextSourceRaw:
		case document.ContentTextSourceNone:
		default:
			item.ContentTextSource = document.ContentTextSourceUnknown
		}
		item.UpdatedAt = parsedUpdatedAt
		stats.TextIssueDocuments = append(stats.TextIssueDocuments, item)
	}
	return rows.Err()
}

func (r *Repository) ProblemContentOCRCandidates(ctx context.Context) ([]document.Document, error) {
	rows, err := r.db.QueryContext(ctx, summarySelect()+`
		WHERE `+textIssueWhereClause()+`
		ORDER BY CASE d.content_text_source
			WHEN ? THEN 0
			WHEN ? THEN 1
			ELSE 2
		END, d.updated_at DESC, d.id DESC`,
		document.ContentTextSourceRaw,
		document.ContentTextSourceNone,
		document.ContentTextSourceRaw,
		document.ContentTextSourceNone,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSummaryDocuments(rows)
}

func textIssueWhereClause() string {
	return `d.deleted_at IS NULL
		  AND d.content_text_source IN (?, ?)
		  AND (d.mime_type = 'application/pdf' OR d.mime_type LIKE 'image/%')`
}

func (r *Repository) statisticBuckets(ctx context.Context, query string, args ...any) ([]document.StatisticBucket, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var buckets []document.StatisticBucket
	for rows.Next() {
		var bucket document.StatisticBucket
		if err := rows.Scan(&bucket.Key, &bucket.Count, &bucket.Bytes); err != nil {
			return nil, err
		}
		buckets = append(buckets, bucket)
	}
	return buckets, rows.Err()
}

func percent(part, total int) int {
	if total <= 0 || part <= 0 {
		return 0
	}
	return (part*100 + total/2) / total
}

func maxBucketCount(buckets []document.StatisticBucket) int {
	maximum := 1
	for _, bucket := range buckets {
		if bucket.Count > maximum {
			maximum = bucket.Count
		}
	}
	return maximum
}

func nullInt64Value(value sql.NullInt64) int64 {
	if value.Valid {
		return value.Int64
	}
	return 0
}

func germanMonthShort(t time.Time) string {
	names := [...]string{"Jan", "Feb", "Mrz", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"}
	return names[int(t.Month())-1] + " " + t.Format("06")
}

func uploadWayStatLabel(value string) string {
	switch document.NormalizeUploadWay(value) {
	case document.UploadWayAPI:
		return "API"
	case document.UploadWayMail:
		return "E-Mail"
	case document.UploadWayWebDAV:
		return "WebDAV"
	default:
		return "Web"
	}
}

func fileTypeStatLabel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pdf":
		return "PDF"
	case "image":
		return "Bilder"
	default:
		return "Andere"
	}
}

func documentDateStatLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "missing" {
		return "Ohne Dateidatum"
	}
	return value
}

func ocrStatusStatLabel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case document.OCRJobStatusQueued:
		return "Wartend"
	case document.OCRJobStatusRunning:
		return "Laufend"
	case document.OCRJobStatusCompleted:
		return "Abgeschlossen"
	case document.OCRJobStatusFailed:
		return "Fehler"
	case document.OCRJobStatusInterrupted:
		return "Unterbrochen"
	default:
		return "Unbekannt"
	}
}
