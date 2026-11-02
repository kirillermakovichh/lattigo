package rpc

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

func pageFixture(t *testing.T) (*Server, protocol.Account, func()) {
	t.Helper()
	s, key := branchServer(t)
	g := s.cfg.Store.Genesis()
	a := g.Allocations[0].Account
	appendRecord := func() {
		t.Helper()
		state, err := s.cfg.Store.Account(a)
		if err != nil {
			t.Fatal(err)
		}
		b := protocol.Block{Version: 1, Network: g.ID(), Kind: "send", Account: a, Previous: state.Frontier, Representative: a, Balance: state.Balance - 1, Link: string(a)}
		if err := b.Sign(key); err != nil {
			t.Fatal(err)
		}
		if _, err := s.cfg.Engine.Submit(b); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		appendRecord()
	}
	return s, a, appendRecord
}
func TestHistoryCursorEndpointKeepsOriginalSnapshot(t *testing.T) {
	s, a, appendRecord := pageFixture(t)
	fetch := func(query string) pageEnvelope[ledger.Record] {
		t.Helper()
		out := httptest.NewRecorder()
		s.Handler().ServeHTTP(out, httptest.NewRequest("GET", "/v1/history/page?address="+string(a)+query, nil))
		if out.Code != 200 {
			t.Fatal(out.Code, out.Body.String())
		}
		var page pageEnvelope[ledger.Record]
		if err := json.Unmarshal(out.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		return page
	}
	first := fetch("&limit=1")
	if len(first.Items) != 1 || first.Items[0].Height != 3 || first.Next == "" {
		t.Fatal("first history page incomplete")
	}
	appendRecord()
	next := fetch("&cursor=" + first.Next)
	if len(next.Items) != 1 || next.Items[0].Height != 2 || next.Anchor != first.Anchor {
		t.Fatal("new confirmation changed cursor snapshot")
	}
}
