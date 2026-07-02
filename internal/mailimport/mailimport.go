// Datei verarbeitet Mail-Importe und bereitet Nachrichten sowie Anhaenge fuer die Dokumentablage auf.
package mailimport

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/textproto"
	"path/filepath"
	"strings"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/uploadlimit"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

var ErrMessageTooLarge = errors.New("E-Mail-Nachricht überschreitet das konfigurierte Größenlimit")

type Client = client.Client

type Message struct {
	Subject  string
	From     string
	PDFs     int
	EMLs     int
	Rejected bool
}

type Attachment struct {
	Filename string
	Reader   io.Reader
}

func OpenMailbox(settings document.MailImportSettings, readOnly bool) (*Client, error) {
	c, err := Dial(settings)
	if err != nil {
		return nil, err
	}
	if err := c.Login(settings.Username, settings.Password); err != nil {
		_ = c.Terminate()
		return nil, err
	}
	if _, err := c.Select(settings.Mailbox, readOnly); err != nil {
		_ = c.Terminate()
		return nil, err
	}
	return c, nil
}

func Dial(settings document.MailImportSettings) (*Client, error) {
	addr := net.JoinHostPort(settings.Host, fmt.Sprintf("%d", settings.Port))
	dialer := &net.Dialer{Timeout: 20 * time.Second}
	tlsConfig := &tls.Config{ServerName: settings.Host, MinVersion: tls.VersionTLS12}

	var c *client.Client
	var err error
	switch settings.Security {
	case document.MailImportSecurityTLS:
		c, err = client.DialWithDialerTLS(dialer, addr, tlsConfig)
	case document.MailImportSecuritySTARTTLS:
		c, err = client.DialWithDialer(dialer, addr)
		if err == nil {
			err = c.StartTLS(tlsConfig)
		}
	case document.MailImportSecurityNone:
		c, err = client.DialWithDialer(dialer, addr)
	default:
		err = errors.New("IMAP-Verschlüsselung ist ungültig")
	}
	if err != nil {
		if c != nil {
			_ = c.Terminate()
		}
		return nil, err
	}
	c.Timeout = 2 * time.Minute
	c.ErrorLog = log.New(io.Discard, "", 0)
	return c, nil
}

func UndeletedUIDs(c *Client) ([]uint32, error) {
	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.DeletedFlag}
	return c.UidSearch(criteria)
}

func FetchMessage(c *Client, uid uint32) (io.Reader, error) {
	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)
	section := &imap.BodySectionName{Peek: true}
	items := []imap.FetchItem{imap.FetchUid, section.FetchItem()}
	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() {
		done <- c.UidFetch(seqset, items, messages)
	}()

	var msg *imap.Message
	for candidate := range messages {
		msg = candidate
	}
	if err := <-done; err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, errors.New("IMAP-Nachricht nicht gefunden")
	}
	body := msg.GetBody(section)
	if body == nil {
		return nil, errors.New("IMAP-Nachricht hat keinen lesbaren Inhalt")
	}
	return body, nil
}

func DeleteMessage(c *Client, uid uint32) error {
	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.DeletedFlag}
	if err := c.UidStore(seqset, item, flags, nil); err != nil {
		return err
	}
	return c.Expunge(nil)
}

func ImportPDFsFromMessage(r io.Reader, allowedSenders string, maxUploadBytes int64, handle func(Attachment) error) (Message, error) {
	return ImportAttachmentsFromMessage(r, allowedSenders, maxUploadBytes, handle, nil)
}

func ImportAttachmentsFromMessage(r io.Reader, allowedSenders string, maxUploadBytes int64, handlePDF func(Attachment) error, handleEML func(Attachment) error) (Message, error) {
	r = &limitReader{
		r:         r,
		remaining: uploadlimit.EnvelopeLimit(maxUploadBytes),
	}
	msg, err := mail.ReadMessage(r)
	if err != nil {
		return Message{}, err
	}

	result := Message{
		Subject: decodeHeader(msg.Header.Get("Subject")),
		From:    SenderAddress(msg.Header.Get("From")),
	}
	if !SenderAllowed(result.From, allowedSenders) {
		result.Rejected = true
		return result, nil
	}

	header := textproto.MIMEHeader(msg.Header)
	err = WalkAttachments(header, msg.Body, func(att Attachment) error {
		result.PDFs++
		if handlePDF == nil {
			return nil
		}
		return handlePDF(att)
	}, func(att Attachment) error {
		result.EMLs++
		if handleEML == nil {
			return nil
		}
		return handleEML(att)
	})
	return result, err
}

type limitReader struct {
	r         io.Reader
	remaining int64
}

func (r *limitReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		var probe [1]byte
		n, err := r.r.Read(probe[:])
		if n > 0 {
			return 0, ErrMessageTooLarge
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.r.Read(p)
	r.remaining -= int64(n)
	return n, err
}

func WalkPDFs(header textproto.MIMEHeader, body io.Reader, handle func(Attachment) error) error {
	return WalkAttachments(header, body, handle, nil)
}

func WalkAttachments(header textproto.MIMEHeader, body io.Reader, handlePDF func(Attachment) error, handleEML func(Attachment) error) error {
	mediaType, params := mediaType(header)
	body = transferReader(header, body)

	if filename, ok := emlAttachmentFilename(header, mediaType, params); ok {
		if handleEML == nil {
			return nil
		}
		return handleEML(Attachment{Filename: filename, Reader: body})
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return errors.New("Multipart-Mail ohne Boundary")
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
			if err := WalkAttachments(part.Header, part, handlePDF, handleEML); err != nil {
				_ = part.Close()
				return err
			}
			if err := part.Close(); err != nil {
				return err
			}
		}
	}

	filename, ok := pdfAttachmentFilename(header, mediaType, params)
	if !ok {
		return nil
	}
	if handlePDF == nil {
		return nil
	}
	return handlePDF(Attachment{Filename: filename, Reader: body})
}

func mediaType(header textproto.MIMEHeader) (string, map[string]string) {
	contentType := header.Get("Content-Type")
	if strings.TrimSpace(contentType) == "" {
		return "text/plain", nil
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(contentType)), nil
	}
	return strings.ToLower(mediaType), params
}

func pdfAttachmentFilename(header textproto.MIMEHeader, mediaType string, params map[string]string) (string, bool) {
	disposition, dispositionParams, _ := mime.ParseMediaType(header.Get("Content-Disposition"))
	filename := dispositionParams["filename"]
	if filename == "" && params != nil {
		filename = params["name"]
	}
	filename = strings.TrimSpace(decodeHeader(filename))
	isPDF := strings.EqualFold(mediaType, "application/pdf") || strings.EqualFold(filepath.Ext(filename), ".pdf")
	if !isPDF {
		return "", false
	}
	if filename == "" {
		filename = "attachment.pdf"
	} else if filepath.Ext(filename) == "" && strings.EqualFold(mediaType, "application/pdf") {
		filename += ".pdf"
	}
	if disposition != "" && !strings.EqualFold(disposition, "attachment") && !strings.EqualFold(disposition, "inline") {
		return "", false
	}
	return filename, true
}

func emlAttachmentFilename(header textproto.MIMEHeader, mediaType string, params map[string]string) (string, bool) {
	disposition, dispositionParams, _ := mime.ParseMediaType(header.Get("Content-Disposition"))
	filename := dispositionParams["filename"]
	if filename == "" && params != nil {
		filename = params["name"]
	}
	filename = strings.TrimSpace(decodeHeader(filename))
	isEML := strings.EqualFold(mediaType, "message/rfc822") || strings.EqualFold(filepath.Ext(filename), ".eml")
	if !isEML {
		return "", false
	}
	if filename == "" {
		filename = "message.eml"
	} else if filepath.Ext(filename) == "" && strings.EqualFold(mediaType, "message/rfc822") {
		filename += ".eml"
	}
	if disposition != "" && !strings.EqualFold(disposition, "attachment") && !strings.EqualFold(disposition, "inline") {
		return "", false
	}
	return filename, true
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

func decodeHeader(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	decoded, err := new(mime.WordDecoder).DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}

func SenderAddress(value string) string {
	value = strings.TrimSpace(decodeHeader(value))
	if value == "" {
		return ""
	}
	addresses, err := mail.ParseAddressList(value)
	if err != nil || len(addresses) == 0 {
		return strings.ToLower(value)
	}
	return strings.ToLower(strings.TrimSpace(addresses[0].Address))
}

func SenderAllowed(sender, allowedSenders string) bool {
	rules := allowedSenderRules(allowedSenders)
	if len(rules) == 0 {
		return true
	}
	sender = strings.ToLower(strings.TrimSpace(sender))
	if sender == "" {
		return false
	}
	_, domain, ok := strings.Cut(sender, "@")
	if !ok || domain == "" {
		return false
	}
	for _, rule := range rules {
		if strings.Contains(rule, "@") && !strings.HasPrefix(rule, "@") {
			if sender == rule {
				return true
			}
			continue
		}
		rule = strings.TrimPrefix(rule, "@")
		rule = strings.TrimPrefix(rule, "*.")
		if domain == rule || strings.HasSuffix(domain, "."+rule) {
			return true
		}
	}
	return false
}

func allowedSenderRules(value string) []string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	rules := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "" {
			continue
		}
		rules = append(rules, line)
	}
	return rules
}
