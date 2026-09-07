package server

import (
	"io"
	"net/http"
	"net/http/httptest"
)

func newLoopbackRequest(method, target string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, target, body)
	r.Host = "localhost"
	r.RemoteAddr = "127.0.0.1:12345"
	return r
}

func (c *documentCountCache) countSize() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.counts)
}

func (s *Server) authenticateBasic(user, password string) (authPrincipal, bool) {
	principal, ok, _ := s.authenticateBasicCheck(user, password)
	return principal, ok
}
