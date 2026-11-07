package rpc

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/go-block-lattice/internal/client"
	"example.com/go-block-lattice/internal/consensus"
	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

func TestEventPositionsSurviveLedgerAndServerRestart(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	g := protocol.Genesis{Version: 1, Name: "event-restart", Supply: 100, Allocations: []protocol.Allocation{{Account: a, Balance: 100, Representative: a}}}
	directory := t.TempDir()
	open := func() (*Server, *httptest.Server) {
		t.Helper()
		store, err := ledger.Open(directory, g)
		if err != nil {
			t.Fatal(err)
		}
		engine, err := consensus.New(consensus.Config{Store: store, PrivateKey: key})
		if err != nil {
			t.Fatal(err)
		}
		api, err := New(Config{Store: store, Engine: engine})
		if err != nil {
			t.Fatal(err)
		}
		return api, httptest.NewServer(api.Handler())
	}
	api, server := open()
	appendRecord := func() {
		t.Helper()
		state, err := api.cfg.Store.Account(a)
		if err != nil {
			t.Fatal(err)
		}
		b := protocol.Block{Version: 1, Network: g.ID(), Kind: "send", Account: a, Previous: state.Frontier, Representative: a, Balance: state.Balance - 1, Link: string(a)}
		if err := b.Sign(key); err != nil {
			t.Fatal(err)
		}
		if _, err := api.cfg.Engine.Submit(b); err != nil {
			t.Fatal(err)
		}
	}
	appendRecord()
	appendRecord()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, _ := client.New(server.URL, "")
	stop := errors.New("stop")
	last, err := c.Events(ctx, client.EventOptions{Network: g.ID()}, func(e client.Event) error {
		if e.Height == 2 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || last == "" {
		t.Fatal(last, err)
	}
	api.Stop()
	server.Close()
	if err := api.cfg.Store.Close(); err != nil {
		t.Fatal(err)
	}
	api, server = open()
	defer server.Close()
	defer api.cfg.Store.Close()
	defer api.Stop()
	appendRecord()
	c, _ = client.New(server.URL, "")
	var heights []uint64
	_, err = c.Events(ctx, client.EventOptions{Network: g.ID(), After: last}, func(e client.Event) error {
		heights = append(heights, e.Height)
		if e.Height == 3 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || len(heights) != 2 || heights[0] != 2 || heights[1] != 3 {
		t.Fatal(heights, err)
	}
}
