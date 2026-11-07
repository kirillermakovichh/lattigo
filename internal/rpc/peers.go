package rpc

import "net/http"

func (s *Server) peers(w http.ResponseWriter, r *http.Request) {
	if !s.authenticated(w, r) {
		return
	}
	if s.cfg.PeerDiagnostics == nil {
		writeJSON(w, 200, []any{})
		return
	}
	writeJSON(w, 200, s.cfg.PeerDiagnostics())
}
