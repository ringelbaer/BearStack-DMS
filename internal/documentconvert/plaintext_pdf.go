package documentconvert

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"unicode"
)

const (
	plainPDFPageWidth  = 595.0
	plainPDFPageHeight = 842.0
	plainPDFMargin     = 54.0
	plainPDFFontSize   = 10.0
	plainPDFLeading    = 14.0
	plainPDFMaxBytes   = 2 << 20
)

func ConvertPlainTextToPDF(source, target string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, plainPDFMaxBytes))
	if err != nil {
		return err
	}
	pdf, err := PlainTextPDF(string(raw))
	if err != nil {
		return err
	}
	return os.WriteFile(target, pdf, 0o600)
}

func PlainTextPDF(text string) ([]byte, error) {
	return PlainTextPDFSections([]string{text})
}

func PlainTextPDFSections(sections []string) ([]byte, error) {
	if len(sections) == 0 {
		sections = []string{""}
	}
	linesPerPage := int(math.Floor((plainPDFPageHeight - 2*plainPDFMargin) / plainPDFLeading))
	if linesPerPage < 1 {
		return nil, fmt.Errorf("invalid PDF text layout")
	}

	var pages [][]string
	for _, section := range sections {
		lines := wrapPlainTextLines(section, plainPDFMaxCharsPerLine())
		if len(lines) == 0 {
			lines = []string{""}
		}
		for len(lines) > 0 {
			n := linesPerPage
			if len(lines) < n {
				n = len(lines)
			}
			pages = append(pages, lines[:n])
			lines = lines[n:]
		}
	}

	objectCount := 3 + len(pages)*2
	objects := make([]string, objectCount+1)
	objects[1] = "<< /Type /Catalog /Pages 2 0 R >>"
	pageRefs := make([]string, len(pages))
	for i := range pages {
		pageObj := 4 + i*2
		pageRefs[i] = fmt.Sprintf("%d 0 R", pageObj)
	}
	objects[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(pageRefs, " "), len(pages))
	objects[3] = "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>"
	for i, pageLines := range pages {
		contentObj := 5 + i*2
		pageObj := 4 + i*2
		stream := plainPDFContentStream(pageLines)
		objects[pageObj] = fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", plainPDFPageWidth, plainPDFPageHeight, contentObj)
		objects[contentObj] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream)
	}

	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, objectCount+1)
	for i := 1; i <= objectCount; i++ {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i, objects[i])
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", objectCount+1)
	for i := 1; i <= objectCount; i++ {
		fmt.Fprintf(&out, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", objectCount+1, xref)
	return out.Bytes(), nil
}

func plainPDFMaxCharsPerLine() int {
	return int(math.Floor((plainPDFPageWidth - 2*plainPDFMargin) / (plainPDFFontSize * 0.6)))
}

func plainPDFContentStream(lines []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "BT\n/F1 %.0f Tf\n%.0f %.0f Td\n%.0f TL\n", plainPDFFontSize, plainPDFMargin, plainPDFPageHeight-plainPDFMargin, plainPDFLeading)
	for i, line := range lines {
		if i > 0 {
			b.WriteString("T*\n")
		}
		fmt.Fprintf(&b, "(%s) Tj\n", escapePDFText(line))
	}
	b.WriteString("ET")
	return b.String()
}

func wrapPlainTextLines(text string, maxChars int) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	sourceLines := strings.Split(text, "\n")
	out := make([]string, 0, len(sourceLines))
	for _, line := range sourceLines {
		line = normalizePDFPlainText(line)
		if line == "" {
			out = append(out, "")
			continue
		}
		for len([]rune(line)) > maxChars {
			part, rest := splitPlainTextLine(line, maxChars)
			out = append(out, part)
			line = rest
		}
		out = append(out, line)
	}
	return out
}

func splitPlainTextLine(line string, maxChars int) (string, string) {
	runes := []rune(line)
	if len(runes) <= maxChars {
		return line, ""
	}
	split := maxChars
	for i := maxChars; i > maxChars/2; i-- {
		if unicode.IsSpace(runes[i-1]) {
			split = i
			break
		}
	}
	left := strings.TrimRightFunc(string(runes[:split]), unicode.IsSpace)
	right := strings.TrimLeftFunc(string(runes[split:]), unicode.IsSpace)
	return left, right
}

func normalizePDFPlainText(text string) string {
	var b strings.Builder
	for _, r := range text {
		switch {
		case r == '\t':
			b.WriteString("    ")
		case r >= 32 && r <= 126:
			b.WriteRune(r)
		case r == 'ä':
			b.WriteString("ae")
		case r == 'Ä':
			b.WriteString("Ae")
		case r == 'ö':
			b.WriteString("oe")
		case r == 'Ö':
			b.WriteString("Oe")
		case r == 'ü':
			b.WriteString("ue")
		case r == 'Ü':
			b.WriteString("Ue")
		case r == 'ß':
			b.WriteString("ss")
		case unicode.IsPrint(r) && !unicode.IsControl(r):
			b.WriteRune('?')
		default:
			b.WriteByte(' ')
		}
	}
	return b.String()
}

func escapePDFText(text string) string {
	return strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(text)
}
