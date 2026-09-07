package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"bearstack/internal/config"
)

func TestUnauthenticatedAccessRequiresLoopback(t *testing.T) {
	s := &Server{}
	for _, tc := range []struct {
		name, host, peer string
		allowed          bool
	}{
		{"IPv4", "127.0.0.1:8080", "127.0.0.1:4567", true},
		{"IPv6", "[::1]:8080", "[::1]:4567", true},
		{"localhost", "localhost:8080", "127.0.0.1:4567", true},
		{"absolute localhost", "LOCALHOST.:8080", "127.0.0.1:4567", true},
		{"rebound hostname", "attacker.example:8080", "127.0.0.1:4567", false},
		{"localhost suffix", "localhost.attacker.example:8080", "127.0.0.1:4567", false},
		{"remote peer", "localhost:8080", "192.0.2.20:4567", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodPost, "PROPFIND"} {
				called := false
				handler := s.basicAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					called = true
					w.WriteHeader(http.StatusNoContent)
				}))
				r := httptest.NewRequest(method, "http://"+tc.host+"/api/documents", nil)
				r.RemoteAddr = tc.peer
				r.Header.Set("Origin", "http://"+tc.host)
				r.Header.Set("X-Forwarded-Host", "localhost:8080")
				r.Header.Set("X-Forwarded-For", "127.0.0.1")
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				want := http.StatusForbidden
				if tc.allowed {
					want = http.StatusNoContent
				}
				if w.Code != want || called != tc.allowed {
					t.Fatalf("%s: status=%d called=%v; want status=%d called=%v", method, w.Code, called, want, tc.allowed)
				}
			}
		})
	}
}

func TestLoopbackHandlerRejectsRebindingHost(t *testing.T) {
	repo := openAuthSecurityRepository(t)
	s := newAuthSecurityServer(t, config.Config{Addr: "127.0.0.1:8080"}, repo)
	handler := s.Handler()
	for _, host := range []string{"127.0.0.1:8080", "attacker.example:8080"} {
		r := httptest.NewRequest(http.MethodGet, "http://"+host+"/api/documents", nil)
		r.RemoteAddr = "127.0.0.1:4567"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		want := http.StatusOK
		if host != "127.0.0.1:8080" {
			want = http.StatusForbidden
		}
		if w.Code != want {
			t.Fatalf("host %s: status=%d, want %d", host, w.Code, want)
		}
	}
}
