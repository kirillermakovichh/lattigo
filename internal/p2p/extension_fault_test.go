//go:build faultaudit

package p2p

import (
	"bytes"
	"net/http/httptest"
	"testing"
)

func TestExtensionInboundEncodingFailure(t *testing.T) {
	n := branchNetwork(t)
	for _, point := range _auditPoints {
		if point.Function != "message" {
			continue
		}
		_auditArm(point.ID, 1)
		w := httptest.NewRecorder()
		n.message(w, httptest.NewRequest("POST", "/p2p/message", bytes.NewBufferString(`{"type":"unknown"}`)))
		reached := _auditReached()
		_auditArm("", 0)
		if !reached || w.Code != 400 {
			t.Fatal(point.ID, w.Code)
		}
	}
}
