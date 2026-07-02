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

const mailBodyTextLimit = 2 << 20

type Options struct {
	MaxBytes  int64
	MergePDFs func(context.Context, string, []string) error
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
	BodySource       string
	PDFs             []pdfAttachment
	OtherAttachments []AttachmentInfo
}

func Build(ctx context.Context, _ string, r io.Reader, opts Options) (Result, error) {
	raw, err := readLimited(r, uploadlimit.EnvelopeLimit(opts.MaxBytes))
	if err != nil {
		return Result{}, err
	}

	tempDir, err := os.MkdirTemp("", "bearstack-mailarchive-*")
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

	mailPDF := filepath.Join(tempDir, "message.pdf")
	pdf, err := documentconvert.PlainTextPDFSections([]string{coverText(msg), bodyText(msg)})
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(mailPDF, pdf, 0o600); err != nil {
		return Result{}, err
	}

	outputPath := mailPDF
	if len(msg.PDFs) > 0 {
		merge := opts.MergePDFs
		if merge == nil {
			merge = mergePDFsWithPDFUnite
		}
		outputPath = filepath.Join(tempDir, "archive.pdf")
		inputs := make([]string, 0, len(msg.PDFs)+1)
		inputs = append(inputs, mailPDF)
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
			data.BodySource = "text/plain"
		}
	case "text/html":
		text, err := readTextBody(body, params)
		if err != nil {
			return err
		}
		if data.BodySource != "text/plain" && strings.TrimSpace(text) != "" {
			data.BodyText = htmlToText(text)
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
	scriptStyleRE = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</\s*(script|style)\s*>`)
	blockTagRE    = regexp.MustCompile(`(?i)</?(br|p|div|section|article|header|footer|table|tr|li|ul|ol|h[1-6])[^>]*>`)
	tagRE         = regexp.MustCompile(`(?s)<[^>]+>`)
	spaceLineRE   = regexp.MustCompile(`[ \t]+`)
	blankLineRE   = regexp.MustCompile(`\n{3,}`)
)

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
