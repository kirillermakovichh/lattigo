//go:build faultaudit

package rpc

import (
	"net/http/httptest"
	"testing"
)

func TestExtensionCursorAndStreamDependencyFailures(t *testing.T) {
	s, a, _ := pageFixture(t)
	for _, tc := range []struct {
		point, path string
		status      int
	}{
		{"cursor.go/encode/json.Marshal/1", "/v1/history/page?address=" + string(a) + "&limit=1", 503},
		{"cursor.go/encode/json.Marshal/1", "/v1/receivables/page?address=" + string(a) + "&limit=1", 503},
		{"events.go/events/json.Marshal/1", "/v1/events", 0},
	} {
		t.Run(tc.path, func(t *testing.T) {
			_auditArm(tc.point, 1)
			defer _auditArm("", 0)
			if tc.status == 0 {
				s.events(&eventWriter{}, httptest.NewRequest("GET", tc.path, nil))
			} else {
				w := httptest.NewRecorder()
				s.Handler().ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
				if w.Code != tc.status {
					t.Fatal(w.Code, w.Body.String())
				}
			}
			if !_auditReached() {
				t.Fatal("dependency fault not reached")
			}
		})
	}
	s.cfg.Store.Close()
	_auditArm("server.go/New/rand.Read/1", 1)
	defer _auditArm("", 0)
	if _, err := New(s.cfg); err == nil || !_auditReached() {
		t.Fatal("entropy failure accepted", err)
	}
}
