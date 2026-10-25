//go:build faultaudit

package p2p

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"example.com/go-block-lattice/internal/consensus"
	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"example.com/go-block-lattice/internal/faultprobe"
)

func TestByteReservationRetriesAfterConcurrentCounterChange(t *testing.T) {
	var counter atomic.Int64
	faultprobe.Before("network.go/reserve/counter.CompareAndSwap/1", 1, func() { counter.Add(1) })
	defer faultprobe.Arm("", 0)
	if !reserve(&counter, 4, 10) || counter.Load() != 5 || !faultprobe.Reached() {
		t.Fatal("contended reservation lost capacity or concurrent increment")
	}
}

func TestPeerEncodingFailuresAreBoundedAndExplicit(t *testing.T) {
	for _, scenario := range []string{"publish", "pending", "records"} {
		t.Run(scenario, func(t *testing.T) {
			n := branchNetwork(t)
			key := ed25519.NewKeyFromSeed(make([]byte, 32))
			a := protocol.Address(key.Public().(ed25519.PublicKey))
			b := protocol.Block{Version: 1, Network: n.cfg.Store.Genesis().ID(), Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(a)}
			if err := b.Sign(key); err != nil {
				t.Fatal(err)
			}
			if scenario == "records" {
				tip := n.cfg.Store.Status().Tip
				v := ledger.Vote{Network: b.Network, Height: 1, Parent: tip, Block: b.ID(), Phase: "precommit", Representative: a}
				if err := v.Sign(key); err != nil {
					t.Fatal(err)
				}
				if err := n.cfg.Store.Commit(ledger.Record{Height: 1, Previous: tip, Block: b, Votes: []ledger.Vote{v}}); err != nil {
					t.Fatal(err)
				}
			} else if scenario == "pending" {
				if _, err := n.cfg.Engine.Submit(b); err != nil {
					t.Fatal(err)
				}
			}
			fn := "Publish"
			if scenario == "pending" {
				fn = "syncPeer"
			}
			if scenario == "records" {
				fn = "records"
			}
			_auditArm("network.go/"+fn+"/json.Marshal/1", 1)
			defer _auditArm("", 0)
			switch scenario {
			case "publish":
				n.Publish(consensus.Message{Type: "block", Block: &b})
				if n.dropped.Load() != 1 || len(n.out) != 0 {
					t.Fatal("failed encoding retained publication")
				}
			case "pending":
				posts := 0
				n.client.Transport = peerTransport(func(r *http.Request) (*http.Response, error) {
					if r.Method == "POST" {
						posts++
					}
					h := make(http.Header)
					h.Set("X-BL-Network", string(b.Network))
					h.Set("X-BL-Version", "1")
					raw, err := json.Marshal(peerStatus{Version: 1, Network: b.Network})
					if err != nil {
						t.Fatal(err)
					}
					return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
				})
				n.syncPeer(context.Background(), &peer{url: "http://local.invalid"})
				if posts != 0 {
					t.Fatal("unencoded pending message sent")
				}
			case "records":
				out := httptest.NewRecorder()
				n.records(out, httptest.NewRequest("GET", "/p2p/records?after=0", nil))
				if out.Code != 500 {
					t.Fatal("record encoding failure acknowledged")
				}
			}
			if !_auditReached() {
				t.Fatal("encoding failure not reached")
			}
		})
	}
}
