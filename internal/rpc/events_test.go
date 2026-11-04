package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/go-block-lattice/internal/ledger"
)

func TestEventsReplayAndShutdown(t *testing.T) {
	api, _, appendRecord := pageFixture(t)
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/v1/events", nil)
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal(response.Header)
	}
	reader := bufio.NewReader(response.Body)
	readRecord := func(height uint64) {
		t.Helper()
		line, err := reader.ReadString('\n')
		if err != nil || !strings.HasPrefix(line, "id: ") {
			t.Fatal(line, err)
		}
		line, err = reader.ReadString('\n')
		if err != nil || line != "event: confirmed\n" {
			t.Fatalf("event: %q %v", line, err)
		}
		line, err = reader.ReadString('\n')
		var record ledger.Record
		if err != nil || !strings.HasPrefix(line, "data: ") || json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &record) != nil || record.Height != height {
			t.Fatalf("data: %q %v", line, err)
		}
		if line, err := reader.ReadString('\n'); err != nil || line != "\n" {
			t.Fatal(line, err)
		}
	}
	for h := uint64(1); h <= 3; h++ {
		readRecord(h)
	}
	appendRecord()
	readRecord(4)
	api.Stop()
	if _, err := reader.ReadByte(); err == nil {
		t.Fatal("stream retained after shutdown")
	}
}

func TestEventResumeScope(t *testing.T) {
	api, _, _ := pageFixture(t)
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	defer api.Stop()
	network := api.cfg.Store.Status().Network
	for _, tc := range []struct {
		id     string
		status int
	}{
		{string(network) + ":01", 400}, {string(network) + ":4", 409}, {strings.Repeat("a", 64) + ":1", 400},
	} {
		req, _ := http.NewRequest("GET", server.URL+"/v1/events", nil)
		req.Header.Set("Last-Event-ID", tc.id)
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Fatal(tc.id, resp.StatusCode)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/v1/events", nil)
	req.Header.Set("Last-Event-ID", string(network)+":2")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil || line != "id: "+string(network)+":3\n" {
		t.Fatal(line, err)
	}
}
