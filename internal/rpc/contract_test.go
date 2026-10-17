package rpc

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/go-block-lattice/internal/consensus"
	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/p2p"
	"example.com/go-block-lattice/internal/protocol"
)

func TestPaymentHistoryHealthAndBackupContract(t *testing.T) {
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, 32))
	owner := protocol.Address(key.Public().(ed25519.PublicKey))
	receiverKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{2}, 32))
	receiver := protocol.Address(receiverKey.Public().(ed25519.PublicKey))
	genesis := protocol.Genesis{Version: 1, Name: "http-contract", Supply: 100, Allocations: []protocol.Allocation{{Account: owner, Balance: 100, Representative: owner}}}
	store, err := ledger.Open(t.TempDir(), genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	engine, err := consensus.New(consensus.Config{Store: store, PrivateKey: key})
	if err != nil {
		t.Fatal(err)
	}
	peers, err := p2p.New(p2p.Config{Store: store, Engine: engine})
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(Config{Store: store, Engine: engine, Token: "private-contract-token", PeerStats: peers.Stats, Syncing: peers.Syncing})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	request := func(method, path string, body any, want int) []byte {
		t.Helper()
		var data []byte
		if body != nil {
			var err error
			data, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer private-contract-token")
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, req)
		if result.Code != want {
			t.Fatalf("%s%s: got%d, want%d: %s", method, path, result.Code, want, result.Body.String())
		}
		return result.Body.Bytes()
	}
	request("GET", "/health/live", nil, 200)
	request("GET", "/health/ready", nil, 200)
	request("GET", "/v1/status", nil, 200)
	request("GET", "/v1/genesis", nil, 200)
	send := protocol.Block{Version: 1, Network: genesis.ID(), Kind: "send", Account: owner, Representative: owner, Balance: 90, Link: string(receiver)}
	if err = send.Sign(key); err != nil {
		t.Fatal(err)
	}
	request("POST", "/v1/blocks", send, 202)
	var history []ledger.Record
	raw := request("GET", "/v1/history?address="+string(owner)+"&limit=1", nil, 200)
	if err = json.Unmarshal(raw, &history); err != nil || len(history) != 1 || history[0].Block.ID() != send.ID() {
		t.Fatal("history does not contain confirmed send")
	}
	request("GET", "/v1/history?address="+string(owner)+"&offset=1", nil, 200)
	request("GET", "/v1/receivables?address="+string(receiver), nil, 200)
	request("GET", "/v1/account?address="+string(receiver), nil, 404)
	request("GET", "/v1/block?id="+string(send.ID()), nil, 200)
	request("POST", "/v1/blocks", send, 200)
	for _, path := range []string{"/v1/history?address=bad", "/v1/history?address=" + string(owner) + "&limit=1001", "/v1/receivables?address=" + string(owner) + "&offset=-1", "/v1/block?id=bad"} {
		request("GET", path, nil, 400)
	}
	stale := send
	stale.Balance = 80
	if err = stale.Sign(key); err != nil {
		t.Fatal(err)
	}
	request("POST", "/v1/blocks", stale, 409)
	invalid := send
	invalid.Balance = 1
	request("POST", "/v1/blocks", invalid, 400)
	metrics := string(request("GET", "/metrics", nil, 200))
	for _, line := range []string{"bl_ledger_height 1", "bl_p2p_peers 0", "bl_consensus_pending 0"} {
		if !strings.Contains(metrics, line) {
			t.Fatalf("metric%s missing", line)
		}
	}
	archive := request("GET", "/v1/admin/backup", nil, 200)
	restored := t.TempDir() + "/restored"
	if err = ledger.Restore(restored, bytes.NewReader(archive), genesis); err != nil {
		t.Fatal(err)
	}
	recovered, err := ledger.OpenExisting(restored, genesis, ledger.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if recovered.Status().Tip != store.Status().Tip {
		t.Fatal("HTTP backup changed confirmed state")
	}
}
func TestReadinessReflectsPendingQuorumAndAdmissionCapacity(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	owner := protocol.Address(key.Public().(ed25519.PublicKey))
	g := protocol.Genesis{Version: 1, Name: "quorum-readiness", Supply: 100, Allocations: []protocol.Allocation{{Account: owner, Balance: 100, Representative: owner}}}
	store, err := ledger.Open(t.TempDir(), g)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	engine, err := consensus.New(consensus.Config{Store: store, MaxPending: 1})
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(Config{Store: store, Engine: engine, Token: "readiness-contract-token"})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	for index, balance := range []protocol.Amount{90, 80} {
		block := protocol.Block{Version: 1, Network: g.ID(), Kind: "send", Account: owner, Representative: owner, Balance: balance, Link: string(owner)}
		if err = block.Sign(key); err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(block)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", "/v1/blocks", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer readiness-contract-token")
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		expected := 202
		if index == 1 {
			expected = 503
		}
		if out.Code != expected {
			t.Fatal(fmt.Sprintf("admission=%d want%d", out.Code, expected))
		}
	}
	ready := httptest.NewRecorder()
	handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if ready.Code != 503 {
		t.Fatal("pending work without voting quorum reported ready")
	}
}
