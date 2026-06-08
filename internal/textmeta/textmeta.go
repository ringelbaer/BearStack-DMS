// Datei extrahiert einfache Text-Metadaten aus Inhalten fuer Suche und Anzeige.
package textmeta

import (
	"bytes"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/ledongthuc/pdf"
)

var datePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(20\d{2}|19\d{2})[-_. ]?([01]\d)[-_. ]?([0-3]\d)`),
	regexp.MustCompile(`([0-3]\d)[-_. ]([01]\d)[-_. ]((?:20|19)\d{2})`),
}

func FromFilename(filename string) (string, *time.Time) {
	base := filename
	if dot := strings.LastIndex(base, "."); dot > 0 {
		base = base[:dot]
	}

	documentDate := parseDate(base)
	title := cleanTitle(removeDate(base))
	if title == "" {
		title = cleanTitle(base)
	}
	if title == "" {
		title = "Dokument"
	}

	return title, documentDate
}

func ExtractPlainText(r io.Reader, limit int64) string {
	if limit <= 0 {
		limit = 2 << 20
	}
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, io.LimitReader(r, limit))
	raw := buf.Bytes()

	var out strings.Builder
	out.Grow(len(raw) / 2)
	for _, r := range string(raw) {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			out.WriteByte(' ')
		case unicode.IsPrint(r) && !unicode.IsControl(r):
			out.WriteRune(r)
		default:
			out.WriteByte(' ')
		}
	}

	text := strings.Join(strings.Fields(out.String()), " ")
	if len(text) > 200000 {
		text = text[:200000]
	}
	return text
}

func ExtractPDFText(path string, limit int64) (string, error) {
	file, reader, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	plain, err := reader.GetPlainText()
	if err != nil {
		return "", err
	}
	return ExtractPlainText(plain, limit), nil
}

func parseDate(input string) *time.Time {
	for _, re := range datePatterns {
		matches := re.FindStringSubmatch(input)
		if matches == nil {
			continue
		}

		var layout, value string
		if len(matches[1]) == 4 {
			layout = "2006-01-02"
			value = matches[1] + "-" + matches[2] + "-" + matches[3]
		} else {
			layout = "2006-01-02"
			value = matches[3] + "-" + matches[2] + "-" + matches[1]
		}
		if parsed, err := time.Parse(layout, value); err == nil {
			return &parsed
		}
	}
	return nil
}

func removeDate(input string) string {
	result := input
	for _, re := range datePatterns {
		result = re.ReplaceAllString(result, " ")
	}
	return result
}

func cleanTitle(input string) string {
	replacer := strings.NewReplacer("_", " ", "-", " ", ".", " ")
	input = replacer.Replace(input)
	input = strings.Join(strings.Fields(input), " ")
	return strings.TrimSpace(input)
}
