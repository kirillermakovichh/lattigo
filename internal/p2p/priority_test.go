package p2p

import (
	"bytes"
	"context"
	"example.com/go-block-lattice/internal/consensus"
	"testing"
)

func TestGlobalPriorityAdmissionAndFairness(t *testing.T) {
	n := branchNetwork(t)
	for i := 0; i < 5; i++ {
		n.Publish(consensus.Message{Type: "vote", Proposal: &consensus.Proposal{Round: uint32(i)}})
	}
	n.Publish(consensus.Message{Type: "block"})
	if len(n.priority) != 5 || len(n.out) != 1 {
		t.Fatal("priority admission")
	}
	for i := 0; i < 4; i++ {
		raw, ok := n.nextOutbound(context.Background())
		if !ok || !bytes.HasPrefix(raw, []byte(`{"type":"vote"`)) {
			t.Fatal(string(raw))
		}
	}
	raw, _ := n.nextOutbound(context.Background())
	if string(raw) != "{\"type\":\"block\"}" {
		t.Fatal("ordinary work starved", string(raw))
	}
	if n.queued.Load() != 0 {
		t.Fatal("ordinary bytes retained")
	}
}
