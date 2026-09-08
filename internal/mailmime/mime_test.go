package mailmime

import (
	"io"
	"net/textproto"
	"strings"
	"testing"
)

func TestAttachmentNamesAndHeaders(t *testing.T) {
	for _, tc := range []struct{ contentType, disposition, want string }{
		{`application/pdf; name="fallback.pdf"`, `attachment; filename="chosen.pdf"`, "chosen.pdf"},
		{`application/pdf; name="fallback.pdf"`, `inline`, "fallback.pdf"},
		{`application/pdf`, `attachment; filename*=utf-8''Gr%C3%BC%C3%9Fe.pdf`, "Grüße.pdf"},
		{`application/pdf`, `attachment; filename="=?windows-1252?Q?Preis_=80.pdf?="`, "Preis €.pdf"},
	} {
		header := textproto.MIMEHeader{"Content-Type": {tc.contentType}, "Content-Disposition": {tc.disposition}}
		kind, params := MediaType(header)
		got, _ := AttachmentFilename(header, params)
		if kind != "application/pdf" || got != tc.want {
			t.Errorf("%s: %q %q", tc.disposition, kind, got)
		}
	}
	for _, tc := range []struct{ raw, want string }{
		{"  Plain subject  ", "Plain subject"},
		{"=?windows-1252?Q?Preis_=80?=", "Preis €"},
		{"=?utf-8?Q?Gr=C3=BC=C3=9Fe?=", "Grüße"},
		{"=?unknown?Q?unchanged?=", "=?unknown?Q?unchanged?="},
	} {
		if got := DecodeHeader(tc.raw); got != tc.want {
			t.Errorf("header: %q, want %q", got, tc.want)
		}
	}
}

func TestTransferDecodingAndFallbacks(t *testing.T) {
	for _, tc := range []struct {
		encoding, input, want string
		invalid               bool
	}{
		{" BASE64 ", "aGVsbG8=", "hello", false},
		{"quoted-printable", "a=3Db", "a=b", false},
		{"8bit", "hello", "hello", false},
		{"base64", "%%%", "", true},
	} {
		got, err := io.ReadAll(TransferReader(textproto.MIMEHeader{"Content-Transfer-Encoding": {tc.encoding}}, strings.NewReader(tc.input)))
		if (err != nil) != tc.invalid || string(got) != tc.want {
			t.Errorf("%s: %q %v", tc.encoding, got, err)
		}
	}
	for _, tc := range []struct{ header, want string }{{"", "text/plain"}, {" Broken Type ", "broken type"}} {
		got, _ := MediaType(textproto.MIMEHeader{"Content-Type": {tc.header}})
		if got != tc.want {
			t.Errorf("media type: %q", got)
		}
	}
	if got, err := DecodeBytes([]byte{0x80}, "windows-1252"); err != nil || got != "€" {
		t.Fatalf("charset: %q %v", got, err)
	}
	if _, err := DecodeBytes([]byte("text"), "unknown-charset"); err == nil {
		t.Fatal("accepted unknown charset")
	}
}
