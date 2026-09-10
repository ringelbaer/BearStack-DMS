// Package documentformat owns the document formats supported by preview generation.
package documentformat

import (
	"path/filepath"
	"slices"
	"strings"
)

type Kind uint8

const (
	Unknown Kind = iota
	PDF
	Image
	PlainText
	Office
)

// MIMEWhitespace is the ASCII whitespace accepted around a MIME type token.
const MIMEWhitespace = " \t\r\n\v\f"

type Format struct {
	Kind       Kind
	MIMETypes  []string
	Extensions []string
}

// MIME takes precedence for native PDF/image rendering. Text and Office formats
// also accept filenames, including documents received as application/octet-stream.
var formats = []Format{
	{Kind: PDF, MIMETypes: []string{"application/pdf"}},
	{Kind: Image, MIMETypes: []string{"image/jpeg", "image/png", "image/gif"}},
	{Kind: PlainText, MIMETypes: []string{"text/plain", "text/markdown"}, Extensions: []string{".txt", ".md"}},
	{Kind: Office, MIMETypes: []string{"text/rtf", "application/rtf", "application/msword", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "application/vnd.apple.pages"}, Extensions: []string{".rtf", ".doc", ".docx", ".pages"}},
}

// Formats returns an independent snapshot for adapters such as SQL query builders.
func Formats() []Format {
	result := slices.Clone(formats)
	for i := range result {
		result[i].MIMETypes = slices.Clone(result[i].MIMETypes)
		result[i].Extensions = slices.Clone(result[i].Extensions)
	}
	return result
}

func NormalizeMIME(value string) string {
	token, _, _ := strings.Cut(value, ";")
	return strings.ToLower(strings.Trim(token, MIMEWhitespace))
}

func Classify(name, mimeType string) Kind {
	mimeType = NormalizeMIME(mimeType)
	ext := strings.ToLower(filepath.Ext(name))
	for _, format := range formats {
		if slices.Contains(format.MIMETypes, mimeType) || slices.Contains(format.Extensions, ext) {
			return format.Kind
		}
	}
	return Unknown
}

func IsLibreOfficeDocument(name, mimeType string) bool { return Classify(name, mimeType) == Office }
func IsPlainTextDocument(name, mimeType string) bool   { return Classify(name, mimeType) == PlainText }
func IsPreviewDocument(name, mimeType string) bool {
	kind := Classify(name, mimeType)
	return kind == PlainText || kind == Office
}

// IsInlinePreview includes image types the browser can display even when the
// server has no thumbnail decoder for them.
func IsInlinePreview(mimeType string) bool {
	mimeType = NormalizeMIME(mimeType)
	return mimeType == "application/pdf" || strings.HasPrefix(mimeType, "image/")
}
