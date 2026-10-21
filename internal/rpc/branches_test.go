package rpc

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"example.com/go-block-lattice/internal/consensus"
	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

func branchServer(t *testing.T) (*Server, ed25519.PrivateKey) {
	t.Helper()
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	g := protocol.Genesis{Version: 1, Name: "rpc-branches", Supply: 100, Allocations: []protocol.Allocation{{Account: a, Balance: 100, Representative: a}}}
	s, err := ledger.Open(t.TempDir(), g)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	e, err := consensus.New(consensus.Config{Store: s, PrivateKey: key})
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(Config{Store: s, Engine: e, Token: "private-branch-token"})
	if err != nil {
		t.Fatal(err)
	}
	return server, key
}
func TestRPCStatusClassificationAndAdmissionBranches(t *testing.T) {
	s, _ := branchServer(t)
	if _, err := New(Config{}); err == nil {
		t.Fatal("missing dependencies accepted")
	}
	for _, item := range []struct {
		err    error
		status int
	}{{consensus.ErrBusy, 503}, {consensus.ErrStale, 409}, {ledger.ErrNotFound, 404}, {ledger.ErrConflict, 409}, {ledger.ErrDependency, 409}, {ledger.ErrInvalid, 400}, {ledger.ErrClosed, 503}, {ledger.ErrCapacity, 503}, {errors.New("sensitive local detail"), 503}} {
		out := httptest.NewRecorder()
		classify(out, item.err)
		if out.Code != item.status {
			t.Fatalf("classification%v:%d", item.err, out.Code)
		}
		if strings.Contains(out.Body.String(), "sensitive") {
			t.Fatal("internal error details exposed")
		}
	}
	h := s.Handler()
	call := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, strings.NewReader(body))
		if path == "/v1/blocks" {
			req.Method = "POST"
		}
		req.Header.Set("Authorization", "Bearer private-branch-token")
		out := httptest.NewRecorder()
		h.ServeHTTP(out, req)
		return out
	}
	for i := 0; i < cap(s.slots); i++ {
		s.slots <- struct{}{}
	}
	if call("/v1/status", "").Code != 503 {
		t.Fatal("concurrency capacity ignored")
	}
	for i := 0; i < cap(s.slots); i++ {
		<-s.slots
	}
	for _, path := range []string{"/v1/account?address=bad", "/v1/receivables?address=bad", "/v1/history?address=bad", "/v1/history?address=" + string(s.cfg.Store.Genesis().Allocations[0].Account) + "&limit=bad", "/v1/receivables?address=" + string(s.cfg.Store.Genesis().Allocations[0].Account) + "&limit=-1"} {
		if call(path, "").Code != 400 {
			t.Fatal("invalid query accepted")
		}
	}
	if call("/v1/blocks", strings.Repeat(" ", maxBody+1)).Code != 400 {
		t.Fatal("oversized body accepted")
	}
	s.cfg.Syncing = func() bool { return true }
	if call("/health/ready", "").Code != 503 {
		t.Fatal("syncing node reported ready")
	}
	s.cfg.Syncing = nil
	for _, stats := range []any{"unknown", map[string]any{"peers": "wrong type"}} {
		s.cfg.PeerStats = func() any { return stats }
		if call("/metrics", "").Code != 200 {
			t.Fatal("optional peer metrics failed")
		}
	}
	s.backupSlot <- struct{}{}
	if call("/v1/admin/backup", "").Code != 503 {
		t.Fatal("parallel backup admitted")
	}
	<-s.backupSlot
	missing := filepath.Join(t.TempDir(), "missing")
	t.Setenv("TMPDIR", missing)
	if call("/v1/admin/backup", "").Code != 503 {
		t.Fatal("backup staging failure ignored")
	}
}
func TestRPCStorageAndDuplicateFailureBranches(t *testing.T) {
	s, key := branchServer(t)
	g := s.cfg.Store.Genesis()
	a := g.Allocations[0].Account
	block := protocol.Block{Version: 1, Network: g.ID(), Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(a)}
	if err := block.Sign(key); err != nil {
		t.Fatal(err)
	}
	published := 0
	s.cfg.PublishBlock = func(protocol.Block) { published++ }
	request := func(block protocol.Block) *httptest.ResponseRecorder {
		data, err := json.Marshal(block)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("POST", "/v1/blocks", bytes.NewReader(data))
		r.Header.Set("Authorization", "Bearer private-branch-token")
		out := httptest.NewRecorder()
		s.Handler().ServeHTTP(out, r)
		return out
	}
	if request(block).Code != 202 || published != 1 {
		t.Fatal("admitted block callback lost")
	}
	invalid := block
	invalid.Signature = "bad"
	if request(invalid).Code != 400 {
		t.Fatal("invalid duplicate accepted")
	}
	state := s.cfg.Store.Status()
	next := block
	next.Previous = block.ID()
	next.Balance = 98
	if err := next.Sign(key); err != nil {
		t.Fatal(err)
	}
	record := ledger.Record{Height: 2, Previous: state.Tip, Block: next}
	vote := ledger.Vote{Network: g.ID(), Height: 2, Phase: "precommit", Parent: state.Tip, Block: next.ID(), Representative: a}
	if err := vote.Sign(key); err != nil {
		t.Fatal(err)
	}
	record.Votes = []ledger.Vote{vote}
	s.cfg.Store.Close()
	if s.cfg.Engine.Handle(consensus.Message{Type: "commit", Record: &record}) == nil {
		t.Fatal("closed storage committed")
	}
	ready := httptest.NewRecorder()
	s.Handler().ServeHTTP(ready, httptest.NewRequest("GET", "/health/ready", nil))
	if ready.Code != 503 {
		t.Fatal("failed engine reported ready")
	}
	backup := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/admin/backup", nil)
	r.Header.Set("Authorization", "Bearer private-branch-token")
	s.Handler().ServeHTTP(backup, r)
	if backup.Code != 503 {
		t.Fatal("closed storage backed up")
	}
}
