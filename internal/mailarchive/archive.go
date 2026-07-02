// Package mailarchive erzeugt revisionsfreundliche PDF-Archive aus angehaengten EML-Dateien.
package mailarchive

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"bearstack/internal/documentconvert"
	"bearstack/internal/storage"
	"bearstack/internal/uploadlimit"

	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"
)

var ErrMessageTooLarge = errors.New("EML-Anhang überschreitet das konfigurierte Größenlimit")
var ErrPDFUniteUnavailable = errors.New("pdfunite ist lokal nicht installiert oder nicht im PATH")
var ErrHTMLRendererUnavailable = errors.New("chromium ist lokal nicht installiert oder nicht im PATH")

const mailBodyTextLimit = 2 << 20

type Options struct {
	MaxBytes   int64
	TempDir    string
	MergePDFs  func(context.Context, string, []string) error
	RenderHTML func(context.Context, string, string, string) error
}

type Result struct {
	Path             string
	Filename         string
	Title            string
	Description      string
	DocumentDate     *time.Time
	PDFs             int
	OtherAttachments []AttachmentInfo
	BodySource       string
	tempDir          string
}

func (r Result) Cleanup() {
	if r.tempDir != "" {
		_ = os.RemoveAll(r.tempDir)
	}
}

type AttachmentInfo struct {
	Filename  string
	MIMEType  string
	SizeBytes int64
}

type pdfAttachment struct {
	AttachmentInfo
	Path string
}

type messageData struct {
	Subject          string
	From             string
	To               string
	Cc               string
	Date             string
	MessageID        string
	DocumentDate     *time.Time
	BodyText         string
	BodyHTML         string
	BodySource       string
	PDFs             []pdfAttachment
	OtherAttachments []AttachmentInfo
}

func Build(ctx context.Context, _ string, r io.Reader, opts Options) (Result, error) {
	raw, err := readLimited(r, uploadlimit.EnvelopeLimit(opts.MaxBytes))
	if err != nil {
		return Result{}, err
	}

	tempDir, err := os.MkdirTemp(opts.TempDir, "bearstack-mailarchive-*")
	if err != nil {
		return Result{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tempDir)
		}
	}()

	msg, err := parseMessage(bytes.NewReader(raw), tempDir, opts.MaxBytes)
	if err != nil {
		return Result{}, err
	}

	messagePDF := filepath.Join(tempDir, "message.pdf")
	if msg.BodyHTML != "" {
		render := opts.RenderHTML
		if render == nil {
			render = renderHTMLWithChromium
		}
		if err := render(ctx, archiveHTML(msg), messagePDF, tempDir); err != nil {
			return Result{}, err
		}
		if err := ensureFileHasContent(messagePDF); err != nil {
			return Result{}, err
		}
	} else {
		pdf, err := documentconvert.PlainTextPDFSections([]string{coverText(msg), bodyText(msg)})
		if err != nil {
			return Result{}, err
		}
		if err := os.WriteFile(messagePDF, pdf, 0o600); err != nil {
			return Result{}, err
		}
	}

	outputPath := messagePDF
	if len(msg.PDFs) > 0 {
		merge := opts.MergePDFs
		if merge == nil {
			merge = mergePDFsWithPDFUnite
		}
		outputPath = filepath.Join(tempDir, "archive.pdf")
		inputs := make([]string, 0, len(msg.PDFs)+1)
		inputs = append(inputs, messagePDF)
		for _, pdf := range msg.PDFs {
			inputs = append(inputs, pdf.Path)
		}
		if err := merge(ctx, outputPath, inputs); err != nil {
			return Result{}, err
		}
		if err := ensureFileHasContent(outputPath); err != nil {
			return Result{}, err
		}
	}

	result := Result{
		Path:             outputPath,
		Filename:         archiveFilename(msg),
		Title:            archiveTitle(msg),
		Description:      archiveDescription(msg),
		DocumentDate:     msg.DocumentDate,
		PDFs:             len(msg.PDFs),
		OtherAttachments: append([]AttachmentInfo(nil), msg.OtherAttachments...),
		BodySource:       msg.BodySource,
		tempDir:          tempDir,
	}
	cleanup = false
	return result, nil
}

func parseMessage(r io.Reader, tempDir string, maxBytes int64) (messageData, error) {
	msg, err := mail.ReadMessage(r)
	if err != nil {
		return messageData{}, err
	}
	data := messageData{
		Subject:   decodeHeader(msg.Header.Get("Subject")),
		From:      decodeAddressHeader(msg.Header.Get("From")),
		To:        decodeAddressHeader(msg.Header.Get("To")),
		Cc:        decodeAddressHeader(msg.Header.Get("Cc")),
		MessageID: strings.TrimSpace(msg.Header.Get("Message-ID")),
	}
	if sentAt, err := msg.Header.Date(); err == nil {
		data.Date = sentAt.Format(time.RFC1123Z)
		documentDate := time.Date(sentAt.Year(), sentAt.Month(), sentAt.Day(), 0, 0, 0, 0, time.UTC)
		data.DocumentDate = &documentDate
	} else {
		data.Date = strings.TrimSpace(msg.Header.Get("Date"))
	}

	if err := walkPart(textproto.MIMEHeader(msg.Header), msg.Body, &data, tempDir, maxBytes); err != nil {
		return messageData{}, err
	}
	if strings.TrimSpace(data.BodyText) == "" {
		data.BodyText = "Kein lesbarer Nachrichtentext."
		data.BodySource = "none"
	}
	if data.BodyHTML == "" && looksLikeHTML(data.BodyText) {
		data.BodyHTML = data.BodyText
		data.BodySource = "text/plain-html"
	}
	return data, nil
}

func walkPart(header textproto.MIMEHeader, body io.Reader, data *messageData, tempDir string, maxBytes int64) error {
	mediaType, params := parseMediaType(header)
	body = transferReader(header, body)
	filename := attachmentFilename(header, params)
	disposition, _, _ := mime.ParseMediaType(header.Get("Content-Disposition"))
	isAttachment := filename != "" || strings.EqualFold(disposition, "attachment")

	if strings.EqualFold(mediaType, "message/rfc822") {
		if filename == "" {
			filename = "message.eml"
		}
		return collectOtherAttachment(data, filename, mediaType, body, maxBytes)
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return errors.New("Multipart-EML ohne Boundary")
		}
		reader := multipart.NewReader(body, boundary)
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			if err := walkPart(part.Header, part, data, tempDir, maxBytes); err != nil {
				_ = part.Close()
				return err
			}
			if err := part.Close(); err != nil {
				return err
			}
		}
	}

	if isPDFAttachment(mediaType, filename) {
		if filename == "" {
			filename = "attachment.pdf"
		} else if filepath.Ext(filename) == "" {
			filename += ".pdf"
		}
		return collectPDFAttachment(data, filename, mediaType, body, tempDir, maxBytes)
	}

	if isAttachment {
		if filename == "" {
			filename = "attachment"
		}
		return collectOtherAttachment(data, filename, mediaType, body, maxBytes)
	}

	switch mediaType {
	case "text/plain":
		text, err := readTextBody(body, params)
		if err != nil {
			return err
		}
		if strings.TrimSpace(text) != "" {
			data.BodyText = text
			if data.BodyHTML == "" {
				data.BodySource = "text/plain"
			}
		}
	case "text/html":
		text, err := readTextBody(body, params)
		if err != nil {
			return err
		}
		if strings.TrimSpace(text) != "" {
			data.BodyHTML = text
			if strings.TrimSpace(data.BodyText) == "" {
				data.BodyText = htmlToText(text)
			}
			data.BodySource = "text/html"
		}
	default:
		_, err := io.Copy(io.Discard, body)
		return err
	}
	return nil
}

func collectPDFAttachment(data *messageData, filename, mediaType string, r io.Reader, tempDir string, maxBytes int64) error {
	file, err := os.CreateTemp(tempDir, "attachment-*.pdf")
	if err != nil {
		return err
	}
	path := file.Name()
	size, copyErr := copyLimited(file, r, maxBytes)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(path)
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}
	data.PDFs = append(data.PDFs, pdfAttachment{
		AttachmentInfo: AttachmentInfo{
			Filename:  safeDisplayFilename(filename),
			MIMEType:  mediaType,
			SizeBytes: size,
		},
		Path: path,
	})
	return nil
}

func collectOtherAttachment(data *messageData, filename, mediaType string, r io.Reader, maxBytes int64) error {
	size, err := drainLimited(r, maxBytes)
	if err != nil {
		return err
	}
	data.OtherAttachments = append(data.OtherAttachments, AttachmentInfo{
		Filename:  safeDisplayFilename(filename),
		MIMEType:  mediaType,
		SizeBytes: size,
	})
	return nil
}

func coverText(msg messageData) string {
	var b strings.Builder
	b.WriteString("E-Mail-Archiv\n\n")
	writeField(&b, "Betreff", emptyDash(msg.Subject))
	writeField(&b, "Von", emptyDash(msg.From))
	writeField(&b, "An", emptyDash(msg.To))
	writeField(&b, "Cc", emptyDash(msg.Cc))
	writeField(&b, "Datum", emptyDash(msg.Date))
	writeField(&b, "Message-ID", emptyDash(msg.MessageID))
	writeField(&b, "Textquelle", emptyDash(msg.BodySource))
	fmt.Fprintf(&b, "PDF-Anhaenge: %d\n", len(msg.PDFs))
	fmt.Fprintf(&b, "Weitere Anhaenge: %d\n", len(msg.OtherAttachments))

	if len(msg.PDFs) > 0 {
		b.WriteString("\nPDF-Anhaenge im Archiv:\n")
		for _, att := range msg.PDFs {
			fmt.Fprintf(&b, "- %s (%s, %d Byte)\n", att.Filename, emptyDash(att.MIMEType), att.SizeBytes)
		}
	}
	if len(msg.OtherAttachments) > 0 {
		b.WriteString("\nNicht eingebettete Anhaenge:\n")
		for _, att := range msg.OtherAttachments {
			fmt.Fprintf(&b, "- %s (%s, %d Byte)\n", att.Filename, emptyDash(att.MIMEType), att.SizeBytes)
		}
	}
	return b.String()
}

func bodyText(msg messageData) string {
	var b strings.Builder
	b.WriteString("E-Mail-Abbildung\n\n")
	writeField(&b, "Betreff", emptyDash(msg.Subject))
	writeField(&b, "Von", emptyDash(msg.From))
	writeField(&b, "An", emptyDash(msg.To))
	writeField(&b, "Cc", emptyDash(msg.Cc))
	writeField(&b, "Datum", emptyDash(msg.Date))
	b.WriteString("\nNachrichtentext:\n\n")
	b.WriteString(strings.TrimSpace(msg.BodyText))
	b.WriteByte('\n')
	return b.String()
}

func archiveHTML(msg messageData) string {
	styles, body := sanitizeHTMLForArchive(msg.BodyHTML)
	var b strings.Builder
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\">")
	b.WriteString(`<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; media-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'">`)
	b.WriteString("<title>")
	b.WriteString(html.EscapeString(archiveTitle(msg)))
	b.WriteString("</title><style>")
	b.WriteString(archiveHTMLCSS())
	b.WriteString("</style>")
	if styles != "" {
		b.WriteString(styles)
	}
	b.WriteString("</head><body>")
	b.WriteString("<section class=\"cover\"><h1>E-Mail-Archiv</h1><dl>")
	writeHTMLField(&b, "Betreff", msg.Subject)
	writeHTMLField(&b, "Von", msg.From)
	writeHTMLField(&b, "An", msg.To)
	writeHTMLField(&b, "Cc", msg.Cc)
	writeHTMLField(&b, "Datum", msg.Date)
	writeHTMLField(&b, "Message-ID", msg.MessageID)
	writeHTMLField(&b, "Textquelle", msg.BodySource)
	writeHTMLField(&b, "PDF-Anhaenge", fmt.Sprintf("%d", len(msg.PDFs)))
	writeHTMLField(&b, "Weitere Anhaenge", fmt.Sprintf("%d", len(msg.OtherAttachments)))
	b.WriteString("</dl>")
	if len(msg.PDFs) > 0 {
		b.WriteString("<h2>PDF-Anhaenge im Archiv</h2><ul>")
		for _, att := range msg.PDFs {
			writeHTMLAttachment(&b, att.AttachmentInfo)
		}
		b.WriteString("</ul>")
	}
	if len(msg.OtherAttachments) > 0 {
		b.WriteString("<h2>Nicht eingebettete Anhaenge</h2><ul>")
		for _, att := range msg.OtherAttachments {
			writeHTMLAttachment(&b, att)
		}
		b.WriteString("</ul>")
	}
	b.WriteString("</section><section class=\"mail\"><header class=\"mail-head\"><h1>E-Mail-Abbildung</h1><dl>")
	writeHTMLField(&b, "Betreff", msg.Subject)
	writeHTMLField(&b, "Von", msg.From)
	writeHTMLField(&b, "An", msg.To)
	writeHTMLField(&b, "Cc", msg.Cc)
	writeHTMLField(&b, "Datum", msg.Date)
	b.WriteString("</dl></header><main class=\"mail-body\">")
	b.WriteString(body)
	b.WriteString("</main></section></body></html>")
	return b.String()
}

func archiveHTMLCSS() string {
	return `@page{size:A4;margin:18mm}*{box-sizing:border-box}body{margin:0;color:#171717;background:#fff;font:14px/1.45 Arial,Helvetica,sans-serif}.cover{break-after:page}.cover h1,.mail-head h1{font-size:24px;margin:0 0 18px}.cover h2{font-size:16px;margin:24px 0 8px}dl{display:grid;grid-template-columns:34mm 1fr;gap:7px 14px;margin:0}dt{font-weight:700;color:#555}dd{margin:0;overflow-wrap:anywhere}ul{margin:0;padding-left:20px}.mail-head{border-bottom:1px solid #ddd;margin-bottom:18px;padding-bottom:12px}.mail-body{overflow-wrap:anywhere}.mail-body img{max-width:100%;height:auto}.mail-body table{max-width:100%;border-collapse:collapse}.mail-body pre{white-space:pre-wrap}`
}

func writeHTMLField(b *strings.Builder, label, value string) {
	b.WriteString("<dt>")
	b.WriteString(html.EscapeString(label))
	b.WriteString("</dt><dd>")
	b.WriteString(html.EscapeString(emptyDash(value)))
	b.WriteString("</dd>")
}

func writeHTMLAttachment(b *strings.Builder, att AttachmentInfo) {
	b.WriteString("<li>")
	b.WriteString(html.EscapeString(att.Filename))
	b.WriteString(" (")
	b.WriteString(html.EscapeString(emptyDash(att.MIMEType)))
	fmt.Fprintf(b, ", %d Byte)", att.SizeBytes)
	b.WriteString("</li>")
}

func writeField(b *strings.Builder, label, value string) {
	fmt.Fprintf(b, "%s: %s\n", label, value)
}

func archiveTitle(msg messageData) string {
	if strings.TrimSpace(msg.Subject) == "" {
		return "E-Mail-Archiv"
	}
	return strings.TrimSpace(msg.Subject)
}

func archiveDescription(msg messageData) string {
	parts := []string{"E-Mail-Archiv"}
	if msg.From != "" {
		parts = append(parts, "Von "+msg.From)
	}
	if msg.Date != "" {
		parts = append(parts, msg.Date)
	}
	return strings.Join(parts, " · ")
}

func archiveFilename(msg messageData) string {
	base := "E-Mail"
	if strings.TrimSpace(msg.Subject) != "" {
		base += " - " + strings.TrimSpace(msg.Subject)
	}
	if msg.DocumentDate != nil {
		base = msg.DocumentDate.Format("2006-01-02") + "_" + base
	}
	filename := storage.SafeFilename(base + ".pdf")
	if filename == "" {
		return "email-archive.pdf"
	}
	return filename
}

func parseMediaType(header textproto.MIMEHeader) (string, map[string]string) {
	contentType := strings.TrimSpace(header.Get("Content-Type"))
	if contentType == "" {
		return "text/plain", map[string]string{}
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.ToLower(contentType), map[string]string{}
	}
	return strings.ToLower(mediaType), params
}

func attachmentFilename(header textproto.MIMEHeader, params map[string]string) string {
	_, dispositionParams, _ := mime.ParseMediaType(header.Get("Content-Disposition"))
	filename := dispositionParams["filename"]
	if filename == "" && params != nil {
		filename = params["name"]
	}
	return strings.TrimSpace(decodeHeader(filename))
}

func isPDFAttachment(mediaType, filename string) bool {
	return strings.EqualFold(mediaType, "application/pdf") || strings.EqualFold(filepath.Ext(filename), ".pdf")
}

func transferReader(header textproto.MIMEHeader, r io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(header.Get("Content-Transfer-Encoding"))) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, r)
	case "quoted-printable":
		return quotedprintable.NewReader(r)
	default:
		return r
	}
}

func readTextBody(r io.Reader, params map[string]string) (string, error) {
	raw, truncated, err := readTextLimited(r, mailBodyTextLimit)
	if err != nil {
		return "", err
	}
	text, err := decodeBytes(raw, params["charset"])
	if err != nil {
		text = string(raw)
	}
	if truncated {
		text += "\n\n[Nachrichtentext gekuerzt.]"
	}
	return text, nil
}

func readTextLimited(r io.Reader, limit int64) ([]byte, bool, error) {
	var buf bytes.Buffer
	limited := io.LimitReader(r, limit+1)
	n, err := buf.ReadFrom(limited)
	if err != nil {
		return nil, false, err
	}
	raw := buf.Bytes()
	if n > limit {
		return raw[:int(limit)], true, nil
	}
	return raw, false, nil
}

func decodeBytes(raw []byte, charset string) (string, error) {
	charset = strings.TrimSpace(charset)
	if charset == "" || strings.EqualFold(charset, "utf-8") || strings.EqualFold(charset, "us-ascii") {
		return string(raw), nil
	}
	enc, err := htmlindex.Get(charset)
	if err != nil {
		return "", err
	}
	decoded, _, err := transform.Bytes(enc.NewDecoder(), raw)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

var (
	scriptStyleRE     = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</\s*(script|style)\s*>`)
	blockTagRE        = regexp.MustCompile(`(?i)</?(br|p|div|section|article|header|footer|table|tr|li|ul|ol|h[1-6])[^>]*>`)
	tagRE             = regexp.MustCompile(`(?s)<[^>]+>`)
	spaceLineRE       = regexp.MustCompile(`[ \t]+`)
	blankLineRE       = regexp.MustCompile(`\n{3,}`)
	htmlSignalRE      = regexp.MustCompile(`(?is)<\s*(html|body|table|tr|td|div|span|p|br|strong|b|style|img|a)\b`)
	archiveUnsafeRE   = regexp.MustCompile(`(?is)<(script|iframe|frame|object|embed|applet|form|input|button|textarea|select|video|audio|source|canvas|svg|math)[^>]*>.*?</\s*(script|iframe|frame|object|embed|applet|form|input|button|textarea|select|video|audio|source|canvas|svg|math)\s*>`)
	archiveSingleRE   = regexp.MustCompile(`(?is)<(meta|link|base)[^>]*>`)
	archiveStyleRE    = regexp.MustCompile(`(?is)<style[^>]*>.*?</\s*style\s*>`)
	archiveHeadRE     = regexp.MustCompile(`(?is)<head[^>]*>.*?</\s*head\s*>`)
	archiveBodyRE     = regexp.MustCompile(`(?is)<body[^>]*>(.*)</\s*body\s*>`)
	archiveDocTypeRE  = regexp.MustCompile(`(?is)<!doctype[^>]*>`)
	archiveShellTagRE = regexp.MustCompile(`(?is)</?(html|head|body|title)[^>]*>`)
	archiveEventRE    = regexp.MustCompile(`(?is)\s+on[a-z0-9_-]+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
)

func looksLikeHTML(value string) bool {
	return htmlSignalRE.MatchString(value)
}

func sanitizeHTMLForArchive(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	value = archiveUnsafeRE.ReplaceAllString(value, " ")
	value = archiveSingleRE.ReplaceAllString(value, " ")
	value = archiveEventRE.ReplaceAllString(value, "")

	styles := strings.Join(archiveStyleRE.FindAllString(value, -1), "\n")
	value = archiveStyleRE.ReplaceAllString(value, " ")
	if matches := archiveBodyRE.FindStringSubmatch(value); len(matches) > 1 {
		value = matches[1]
	} else {
		value = archiveHeadRE.ReplaceAllString(value, " ")
	}
	value = archiveDocTypeRE.ReplaceAllString(value, " ")
	value = archiveShellTagRE.ReplaceAllString(value, " ")
	return styles, strings.TrimSpace(value)
}

func htmlToText(value string) string {
	value = scriptStyleRE.ReplaceAllString(value, " ")
	value = blockTagRE.ReplaceAllString(value, "\n")
	value = tagRE.ReplaceAllString(value, " ")
	value = html.UnescapeString(value)
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(spaceLineRE.ReplaceAllString(line, " "))
	}
	value = strings.Join(lines, "\n")
	value = blankLineRE.ReplaceAllString(value, "\n\n")
	return strings.TrimSpace(value)
}

func decodeHeader(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	decoded, err := (&mime.WordDecoder{CharsetReader: charsetReader}).DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}

func decodeAddressHeader(value string) string {
	value = decodeHeader(value)
	if value == "" {
		return ""
	}
	addresses, err := mail.ParseAddressList(value)
	if err != nil || len(addresses) == 0 {
		return value
	}
	out := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if strings.TrimSpace(address.Name) == "" {
			out = append(out, address.Address)
			continue
		}
		out = append(out, strings.TrimSpace(address.Name)+" <"+address.Address+">")
	}
	return strings.Join(out, ", ")
}

func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	charset = strings.TrimSpace(charset)
	if charset == "" || strings.EqualFold(charset, "utf-8") || strings.EqualFold(charset, "us-ascii") {
		return input, nil
	}
	enc, err := htmlindex.Get(charset)
	if err != nil {
		return nil, err
	}
	return transform.NewReader(input, enc.NewDecoder()), nil
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	var buf bytes.Buffer
	n, err := buf.ReadFrom(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if n > limit {
		return nil, ErrMessageTooLarge
	}
	return buf.Bytes(), nil
}

func copyLimited(w io.Writer, r io.Reader, maxBytes int64) (int64, error) {
	limit := maxBytes
	if limit < 1 {
		limit = uploadlimit.DefaultMaxBytes
	}
	limited := io.LimitReader(r, limit+1)
	n, err := io.Copy(w, limited)
	if err != nil {
		return n, err
	}
	if n > limit {
		return n, ErrMessageTooLarge
	}
	return n, nil
}

func drainLimited(r io.Reader, maxBytes int64) (int64, error) {
	return copyLimited(io.Discard, r, maxBytes)
}

func renderHTMLWithChromium(ctx context.Context, htmlContent, output, tempDir string) error {
	command, err := chromiumCommand()
	if err != nil {
		return err
	}
	htmlPath := filepath.Join(tempDir, "message.html")
	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0o600); err != nil {
		return err
	}
	profileDir, err := os.MkdirTemp(tempDir, "chromium-profile-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(profileDir)

	htmlURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(htmlPath)}).String()
	args := []string{
		"--headless",
		"--disable-gpu",
		"--no-sandbox",
		"--disable-dev-shm-usage",
		"--disable-background-networking",
		"--disable-default-apps",
		"--disable-extensions",
		"--disable-sync",
		"--disable-javascript",
		"--no-pdf-header-footer",
		"--user-data-dir=" + profileDir,
		"--print-to-pdf=" + output,
		htmlURL,
	}
	cmd := exec.CommandContext(ctx, command, args...)
	combined, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("chromium: %w", ctx.Err())
		}
		return fmt.Errorf("chromium: %w: %s", err, strings.TrimSpace(string(combined)))
	}
	return normalizeBrowserPDF(output)
}

func chromiumCommand() (string, error) {
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	if _, err := os.Stat("/snap/bin/chromium"); err == nil {
		return "/snap/bin/chromium", nil
	}
	return "", ErrHTMLRendererUnavailable
}

var browserPDFDateRE = regexp.MustCompile(`D:\d{14}`)

func normalizeBrowserPDF(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	normalized := browserPDFDateRE.ReplaceAll(content, []byte("D:20000101000000"))
	if bytes.Equal(content, normalized) {
		return nil
	}
	return os.WriteFile(path, normalized, 0o600)
}

func mergePDFsWithPDFUnite(ctx context.Context, output string, inputs []string) error {
	command, err := exec.LookPath("pdfunite")
	if err != nil {
		return ErrPDFUniteUnavailable
	}
	args := append(append([]string{}, inputs...), output)
	cmd := exec.CommandContext(ctx, command, args...)
	combined, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("pdfunite: %w", ctx.Err())
		}
		return fmt.Errorf("pdfunite: %w: %s", err, strings.TrimSpace(string(combined)))
	}
	return nil
}

func ensureFileHasContent(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return errors.New("Archiv-PDF ist leer")
	}
	return nil
}

func safeDisplayFilename(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	if value == "" || value == "." || value == ".." {
		return "attachment"
	}
	return value
}

func emptyDash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}
