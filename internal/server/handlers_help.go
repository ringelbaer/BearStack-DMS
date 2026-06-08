// Datei liefert Hilfeseiten und begleitende Informationen fuer die Bedienoberflaeche aus.
package server

import "net/http"

func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "help.html", PageData{
		Title:  "Hilfe",
		Active: "help",
	})
}
