package blogcontent

import (
	"strings"
	"testing"
)

func TestRenderKeepsMarkupAndUntrustedTextSeparate(t *testing.T) {
	raw := []byte("# <script>alert(1)</script>\n- <img src=x onerror=alert(1)>\n\n## A & B\n<!-- @pg-date 2024-06-01 -->\n")
	text, html := Render("post.md", raw)
	if strings.Contains(string(html), "<script>") || strings.Contains(string(html), "<img ") || strings.Contains(string(html), "@pg-date") {
		t.Fatalf("untrusted markup or annotation exposed: %s", html)
	}
	want := "<h2>&lt;script&gt;alert(1)&lt;/script&gt;</h2><ul><li>&lt;img src=x onerror=alert(1)&gt;</li></ul><h3>A &amp; B</h3>"
	if string(html) != want || !strings.Contains(text, "<script>alert(1)</script>") {
		t.Fatalf("text %q, HTML %s", text, html)
	}
	if date := Date(raw); date == nil || date.Format("2006-01-02") != "2024-06-01" {
		t.Fatalf("date: %v", date)
	}
}

func TestPlainTextKeepsWhitespaceAndEscapesHTML(t *testing.T) {
	raw := []byte("  <a href='javascript:alert(1)'>text</a>\n# heading\n")
	text, html := Render("POST.TXT", raw)
	if text != string(raw) || string(html) != "<pre>  &lt;a href=&#39;javascript:alert(1)&#39;&gt;text&lt;/a&gt;\n# heading\n</pre>" {
		t.Fatalf("text %q, HTML %s", text, html)
	}
	if Date([]byte("<!-- @pg-date 2024-02-30 -->")) != nil {
		t.Fatal("invalid date accepted")
	}
}
