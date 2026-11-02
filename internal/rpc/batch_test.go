package rpc

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func TestBatchReadsReturnIndependentOrderedOutcomes(t *testing.T) {
	s, _ := branchServer(t)
	a := s.cfg.Store.Genesis().Allocations[0].Account
	queries := []readQuery{{ID: "state", Operation: "status"}, {ID: "account", Operation: "account", Address: a}, {ID: "absent", Operation: "block", Hash: protocol.Digest([]byte("missing"))}, {ID: "unsupported", Operation: "submit"}}
	raw, err := json.Marshal(map[string]any{"queries": queries})
	if err != nil {
		t.Fatal(err)
	}
	out := httptest.NewRecorder()
	s.Handler().ServeHTTP(out, httptest.NewRequest("POST", "/v1/query", bytes.NewReader(raw)))
	if out.Code != 200 {
		t.Fatal(out.Code, out.Body.String())
	}
	var rows []struct {
		ID    string
		Data  json.RawMessage
		Error *readError
	}
	if err := json.Unmarshal(out.Body.Bytes(), &rows); err != nil || len(rows) != 4 {
		t.Fatal("invalid batch response")
	}
	for i, row := range rows {
		if row.ID != queries[i].ID {
			t.Fatal("batch lost correlation order")
		}
	}
	if rows[0].Error != nil || rows[1].Error != nil || rows[2].Error.Code != "not_found" || rows[3].Error.Code != "invalid_operation" {
		t.Fatal("batch confused success and failure")
	}
	if s.cfg.Store.Status().Height != 0 {
		t.Fatal("read batch changed confirmed state")
	}
}
func TestBatchReadsRejectDuplicateIDsAndOversizedArrays(t *testing.T) {
	s, _ := branchServer(t)
	for _, body := range []string{`{}`, `{"queries":null}`, `{"queries":[]}`, `{"queries":[{"id":"x","operation":"status"},{"id":"x","operation":"status"}]}`, `{"queries":[{"id":"","operation":"status"}]}`, `{"queries":[{"id":"x","operation":"status","unknown":true}]}`} {
		out := httptest.NewRecorder()
		s.Handler().ServeHTTP(out, httptest.NewRequest("POST", "/v1/query", bytes.NewBufferString(body)))
		if out.Code != 400 {
			t.Fatal("invalid batch admitted")
		}
	}
	queries := make([]readQuery, 33)
	raw, err := json.Marshal(map[string]any{"queries": queries})
	if err != nil {
		t.Fatal(err)
	}
	out := httptest.NewRecorder()
	s.Handler().ServeHTTP(out, httptest.NewRequest("POST", "/v1/query", bytes.NewReader(raw)))
	if out.Code != 400 {
		t.Fatal("query count budget ignored")
	}
}
