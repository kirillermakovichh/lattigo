package rpc

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPeerDiagnosticsRemainAuthenticated(t *testing.T) {
	s, _ := branchServer(t)
	calls := 0
	s.cfg.PeerDiagnostics = func() any { calls++; return []string{"http://configured"} }
	for _, authenticated := range []bool{false, true} {
		r := httptest.NewRequest("GET", "/v1/admin/peers", nil)
		if authenticated {
			r.Header.Set("Authorization", "Bearer private-branch-token")
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if !authenticated && (w.Code != 401 || calls != 0) {
			t.Fatal("diagnostic origin leaked")
		}
		if authenticated && (w.Code != 200 || calls != 1 || !strings.Contains(w.Body.String(), "configured")) {
			t.Fatal("diagnostics unavailable")
		}
	}
	s.cfg.PeerDiagnostics = nil
	r := httptest.NewRequest("GET", "/v1/admin/peers", nil)
	r.Header.Set("Authorization", "Bearer private-branch-token")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal("missing peer provider")
	}
}
