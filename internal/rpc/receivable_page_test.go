package rpc

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"example.com/go-block-lattice/internal/ledger"
)

func TestReceivableCursorEndpointRejectsSnapshotChanges(t *testing.T) {
	s, a, appendRecord := pageFixture(t)
	first := httptest.NewRecorder()
	s.Handler().ServeHTTP(first, httptest.NewRequest("GET", "/v1/receivables/page?address="+string(a)+"&limit=1", nil))
	var page pageEnvelope[ledger.Receivable]
	if first.Code != 200 {
		t.Fatal(first.Code, first.Body.String())
	}
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || page.Next == "" {
		t.Fatal("receivable first page incomplete")
	}
	next := httptest.NewRecorder()
	s.Handler().ServeHTTP(next, httptest.NewRequest("GET", "/v1/receivables/page?address="+string(a)+"&cursor="+page.Next, nil))
	var continuation pageEnvelope[ledger.Receivable]
	if err := json.Unmarshal(next.Body.Bytes(), &continuation); err != nil || next.Code != 200 || continuation.Items[0].Source <= page.Items[0].Source {
		t.Fatal("receivable continuation repeated a source")
	}
	appendRecord()
	stale := httptest.NewRecorder()
	s.Handler().ServeHTTP(stale, httptest.NewRequest("GET", "/v1/receivables/page?address="+string(a)+"&cursor="+page.Next, nil))
	if stale.Code != 409 {
		t.Fatal("stale cursor returned shifted receivables")
	}
}
