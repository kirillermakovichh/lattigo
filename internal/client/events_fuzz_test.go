package client

import (
	"context"
	"errors"
	"strings"
	"testing"

	"example.com/go-block-lattice/internal/event"
)

func FuzzEventFrames(f *testing.F) {
	options, valid := eventFixture(f, 1)
	for _, seed := range []string{"", valid, valid + valid, strings.TrimSuffix(valid, "\n"), ": heartbeat\n\n", "id: bad\nevent: confirmed\ndata: {}\n\n", "data: " + strings.Repeat("x", 8192)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		// The fuzz engine owns its corpus bytes; parser allocation stays bounded by
		// its independent frame budget even when a caller supplies a larger stream.
		if len(input) > MaxEventBytes+4096 {
			t.Skip()
		}
		previous := uint64(0)
		last, err := parseEvents(context.Background(), strings.NewReader(input), options, func(e Event) error {
			height, err := event.ParseID(e.ID, options.Network)
			if err != nil || height != e.Height || height <= previous || e.Block.Verify() != nil || e.Block.Network != options.Network {
				t.Fatal("invalid event delivered")
			}
			previous = height
			return nil
		})
		if last != "" {
			height, parseErr := event.ParseID(last, options.Network)
			if parseErr != nil || height != previous {
				t.Fatal("acknowledgement differs from delivered state")
			}
		}
		if err == nil {
			t.Fatal("finite stream ended without EOF")
		}
		if errors.Is(err, context.Canceled) {
			t.Fatal("parser invented cancellation")
		}
	})
}
