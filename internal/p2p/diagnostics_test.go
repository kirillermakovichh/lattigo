package p2p

import "testing"

func TestDiagnosticsSnapshotsConfiguredPeers(t *testing.T) {
	n := branchNetwork(t)
	n.peers = []*peer{{url: "http://configured", queue: make(chan []byte, 1)}}
	p := n.peers[0]
	p.connected.Store(true)
	p.height.Store(7)
	p.queued.Store(12)
	rows := n.Diagnostics().([]PeerDiagnostic)
	if len(rows) != 1 || rows[0].Height != 7 || rows[0].QueuedBytes != 12 || !rows[0].Connected {
		t.Fatal(rows)
	}
	rows[0].URL = "changed"
	if n.Diagnostics().([]PeerDiagnostic)[0].URL != "http://configured" {
		t.Fatal("diagnostics alias state")
	}
}

func TestAggregateMetricsIncludeBothQueueClasses(t *testing.T) {
	n := branchNetwork(t)
	n.queued.Store(3)
	n.priorityBytes.Store(5)
	p := &peer{queue: make(chan []byte, 1), priority: make(chan []byte, 1)}
	p.queued.Store(7)
	p.priorityBytes.Store(11)
	n.peers = []*peer{p}
	metrics := n.Stats().(map[string]any)
	if metrics["queued_bytes"] != 26 || metrics["queue_item_budget"] != 368 || metrics["queue_byte_budget"] != int(112<<20) {
		t.Fatal(metrics)
	}
	for key := range metrics {
		if key == p.url {
			t.Fatal("identity metric label")
		}
	}
}
