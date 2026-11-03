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
