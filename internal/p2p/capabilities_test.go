package p2p

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestCapabilitiesRetainNetworkAdmission(t *testing.T) {
	n := branchNetwork(t)
	r := httptest.NewRequest("GET", "/p2p/capabilities", nil)
	w := httptest.NewRecorder()
	n.Handler().ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatal("capability discovery bypasses network admission")
	}
	r.Header.Set("X-BL-Network", string(n.cfg.Store.Status().Network))
	r.Header.Set("X-BL-Version", "1")
	w = httptest.NewRecorder()
	n.Handler().ServeHTTP(w, r)
	var caps Capabilities
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &caps) != nil || caps.MaxMessageBytes != 20<<20 || caps.MaxRecords != 16 || caps.PriorityBurst != 4 {
		t.Fatal(w.Code, w.Body.String())
	}
}
