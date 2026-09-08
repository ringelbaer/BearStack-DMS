// Package mailmime shares MIME decoding between mailbox import and EML archives.
// Attachment selection, size limits and recursion remain with the caller.
package mailmime

import (
	"encoding/base64"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/textproto"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"
)

func MediaType(header textproto.MIMEHeader) (string, map[string]string) {
	contentType := strings.TrimSpace(header.Get("Content-Type"))
	if contentType == "" {
		return "text/plain", nil
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.ToLower(contentType), nil
	}
	return strings.ToLower(mediaType), params
}

// AttachmentFilename decodes RFC 2047/2231 names but does not turn them into
// filesystem paths. Storage callers still apply their own safe-name policy.
func AttachmentFilename(header textproto.MIMEHeader, params map[string]string) (filename, disposition string) {
	disposition, dispositionParams, _ := mime.ParseMediaType(header.Get("Content-Disposition"))
	filename = dispositionParams["filename"]
	if filename == "" {
		filename = params["name"]
	}
	return strings.TrimSpace(DecodeHeader(filename)), disposition
}

func TransferReader(header textproto.MIMEHeader, body io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(header.Get("Content-Transfer-Encoding"))) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, body)
	case "quoted-printable":
		return quotedprintable.NewReader(body)
	default:
		return body
	}
}

func DecodeHeader(value string) string {
	value = strings.TrimSpace(value)
	decoded, err := (&mime.WordDecoder{CharsetReader: CharsetReader}).DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}

func CharsetReader(charset string, input io.Reader) (io.Reader, error) {
	enc, err := charsetEncoding(charset)
	if err != nil {
		return nil, err
	}
	if enc == nil {
		return input, nil
	}
	return transform.NewReader(input, enc.NewDecoder()), nil
}

func charsetEncoding(charset string) (encoding.Encoding, error) {
	charset = strings.TrimSpace(charset)
	if charset == "" || strings.EqualFold(charset, "utf-8") || strings.EqualFold(charset, "us-ascii") {
		return nil, nil
	}
	return htmlindex.Get(charset)
}

func DecodeBytes(raw []byte, charset string) (string, error) {
	enc, err := charsetEncoding(charset)
	if err != nil {
		return "", err
	}
	if enc == nil {
		return string(raw), nil
	}
	decoded, _, err := transform.Bytes(enc.NewDecoder(), raw)
	return string(decoded), err
}
