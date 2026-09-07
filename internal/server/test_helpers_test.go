package server

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
