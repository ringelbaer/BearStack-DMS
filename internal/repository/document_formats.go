package repository

import (
	"strings"

	"bearstack/internal/documentformat"
)

// Construct once from the same rules used by the renderer. Keep filtering inside
// SQLite: keyset pagination must not return short/empty pages of unsupported files.
var thumbnailFormatPredicate = buildThumbnailFormatPredicate()

func buildThumbnailFormatPredicate() string {
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
	var mimeTypes, conditions []string
	for _, format := range documentformat.Formats() {
		for _, mimeType := range format.MIMETypes {
			mimeTypes = append(mimeTypes, quote(mimeType))
		}
		for _, ext := range format.Extensions {
			conditions = append(conditions, "lower(original_name) LIKE "+quote("%"+ext))
		}
	}
	// Strip parameters before trimming/case folding, just as NormalizeMIME does.
	mime := "lower(trim(substr(mime_type, 1, instr(mime_type || ';', ';') - 1), " + quote(documentformat.MIMEWhitespace) + "))"
	conditions = append([]string{mime + " IN (" + strings.Join(mimeTypes, ",") + ")"}, conditions...)
	return "(" + strings.Join(conditions, " OR ") + ")"
}
