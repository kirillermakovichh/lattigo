package client

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"example.com/go-block-lattice/internal/event"
	"example.com/go-block-lattice/internal/protocol"
)

func eventFixture(t *testing.T, height uint64) (EventOptions, string) {
	t.Helper()
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	network := protocol.Digest([]byte("events"))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	b := protocol.Block{Version: 1, Network: network, Kind: "send", Account: a, Representative: a, Balance: 1, Link: string(a)}
	if err := b.Sign(key); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(struct {
		Height uint64         `json:"height"`
		Block  protocol.Block `json:"block"`
	}{height, b})
	return EventOptions{Network: network}, fmt.Sprintf("id: %s\nevent: confirmed\ndata: %s\n\n", event.ID(network, height), data)
}
func TestEventParserAcknowledgesOnlySuccessfulFrames(t *testing.T) {
	options, frame := eventFixture(t, 1)
	calls := 0
	last, err := parseEvents(context.Background(), strings.NewReader(": heartbeat\n\n"+frame), options, func(e Event) error {
		calls++
		if e.Height != 1 {
			t.Fatal(e)
		}
		return nil
	})
	if !errors.Is(err, io.EOF) || calls != 1 || last != event.ID(options.Network, 1) {
		t.Fatal(last, calls, err)
	}
	denied := errors.New("consumer")
	last, err = parseEvents(context.Background(), strings.NewReader(frame), options, func(Event) error { return denied })
	if last != "" || !errors.Is(err, denied) {
		t.Fatal(last, err)
	}
	last, err = parseEvents(context.Background(), strings.NewReader(strings.TrimSuffix(frame, "\n")), options, func(Event) error { t.Fatal("incomplete frame delivered"); return nil })
	if last != "" || !errors.Is(err, io.EOF) {
		t.Fatal(last, err)
	}
}
func TestEventParserBoundsAndRejectsMalformedFrames(t *testing.T) {
	options, frame := eventFixture(t, 1)
	for _, input := range []string{strings.Repeat("x", MaxEventBytes+1), strings.Replace(frame, "event: confirmed", "event: unknown", 1), frame + frame, strings.Replace(frame, "id: ", "id: wrong\nid: ", 1), "unknown: field\n\n"} {
		_, err := parseEvents(context.Background(), strings.NewReader(input), options, func(Event) error { return nil })
		if !errors.Is(err, ErrEventProtocol) {
			t.Fatal(err)
		}
	}
}

func TestFilteredCheckpointsResumeWithoutFinancialCallback(t *testing.T) {
	options, frame := eventFixture(t, 2)
	var b struct {
		Block protocol.Block `json:"block"`
	}
	start := strings.Index(frame, "data: ") + 6
	if err := json.Unmarshal([]byte(strings.TrimSpace(frame[start:])), &b); err != nil {
		t.Fatal(err)
	}
	options.Address = b.Block.Account
	checkpoint := fmt.Sprintf("id: %s\nevent: checkpoint\ndata: {\"height\":1}\n\n", event.ID(options.Network, 1))
	count := 0
	last, err := parseEvents(context.Background(), strings.NewReader(checkpoint+frame), options, func(Event) error { count++; return nil })
	if !errors.Is(err, io.EOF) || count != 1 || last != event.ID(options.Network, 2) {
		t.Fatal(last, count, err)
	}
	options.Address = ""
	if _, err := parseEvents(context.Background(), strings.NewReader(checkpoint), options, func(Event) error { return nil }); !errors.Is(err, ErrEventProtocol) {
		t.Fatal(err)
	}
}
