package event

import (
	"example.com/go-block-lattice/internal/protocol"
	"testing"
)

func TestCanonicalPosition(t *testing.T) {
	network := protocol.Digest([]byte("network"))
	for _, h := range []uint64{0, 1, ^uint64(0)} {
		got, err := ParseID(ID(network, h), network)
		if err != nil || got != h {
			t.Fatal(got, err)
		}
	}
	for _, raw := range []string{"", string(network) + ":-1", string(network) + ":01", string(network) + ":18446744073709551616", ID(protocol.Digest(nil), 1)} {
		if _, err := ParseID(raw, network); err == nil {
			t.Fatal(raw)
		}
	}
}
