// Datei vereinheitlicht die Drosselung sicherheitsrelevanter Passwortbestätigungen.
package server

import (
	"errors"
	"net/http"
	"strconv"
)

func (s *Server) passwordConfirmationFailure(w http.ResponseWriter, r *http.Request, password string) (int, error) {
	ok, retryAfter := s.authPasswordCheck(r, password)
	if ok {
		return 0, nil
	}
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.FormatInt(retryAfterSeconds(retryAfter), 10))
		return http.StatusTooManyRequests, errors.New("zu viele fehlgeschlagene Passwortbestätigungen; bitte versuchen Sie es später erneut")
	}
	return http.StatusForbidden, errors.New("Passwortbestätigung fehlt oder ist ungültig")
}
