package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPSRedirectURLUsesRequestHostAndTLSPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/path?q=1", nil)
	req.Host = "example.com:80"
	got := httpsRedirectURL(req, "127.0.0.1:8443")
	want := "https://example.com:8443/path?q=1"
	if got != want {
		t.Fatalf("url = %q want %q", got, want)
	}
}

func TestHTTPSRedirectURLOmitsDefaultTLSPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/path?q=1", nil)
	got := httpsRedirectURL(req, "127.0.0.1:443")
	want := "https://example.com/path?q=1"
	if got != want {
		t.Fatalf("url = %q want %q", got, want)
	}
}

func TestHTTPSRedirectURLFallsBackToTLSHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/path", nil)
	req.Host = ""
	got := httpsRedirectURL(req, "bearstack.local:8443")
	want := "https://bearstack.local:8443/path"
	if got != want {
		t.Fatalf("url = %q want %q", got, want)
	}
}

func TestHTTPToHTTPSRedirectHandler(t *testing.T) {
	handler := httpToHTTPSRedirectHandler("127.0.0.1:8443")
	req := httptest.NewRequest(http.MethodPost, "http://example.com/upload", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusPermanentRedirect {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "https://example.com:8443/upload" {
		t.Fatalf("location = %q", got)
	}
}
