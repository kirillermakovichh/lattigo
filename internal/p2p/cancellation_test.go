package p2p

import (
	"context"
	"testing"
)

func TestCanceledDispatcherLeavesReadyQueues(t *testing.T) {
	n := branchNetwork(t)
	n.priority <- []byte("priority")
	n.out <- []byte("ordinary")
	n.priorityBytes.Store(8)
	n.queued.Store(8)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 10; i++ {
		if _, ok := n.nextOutbound(ctx); ok {
			t.Fatal("canceled dispatcher consumed ready work")
		}
	}
	if len(n.priority) != 1 || len(n.out) != 1 || n.priorityBytes.Load() != 8 || n.queued.Load() != 8 {
		t.Fatal("cancellation changed queue ownership")
	}
}
