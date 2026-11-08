package p2p

// PeerDiagnostic is a bounded snapshot of a configured peer. URLs are exposed
// only through authenticated administration, never as metric labels.
type PeerDiagnostic struct {
	LatencyNanos        int64  `json:"latency_nanos"`
	LastSuccessUnixNano int64  `json:"last_success_unix_nano"`
	URL                 string `json:"url"`
	Connected           bool   `json:"connected"`
	Height              uint64 `json:"height"`
	QueuedBytes         int64  `json:"queued_bytes"`
	QueuedMessages      int    `json:"queued_messages"`
	ConsecutiveFailures uint64 `json:"consecutive_failures"`
	RetryAtUnixNano     int64  `json:"retry_at_unix_nano"`
}

func (n *Network) Diagnostics() any {
	rows := make([]PeerDiagnostic, 0, len(n.peers))
	for _, p := range n.peers {
		rows = append(rows, PeerDiagnostic{LatencyNanos: p.latency.Load(), LastSuccessUnixNano: p.lastSuccess.Load(), URL: p.url, Connected: p.connected.Load(), Height: p.height.Load(), QueuedBytes: p.queued.Load() + p.priorityBytes.Load(), QueuedMessages: len(p.queue) + len(p.priority), ConsecutiveFailures: p.failures.Load(), RetryAtUnixNano: p.retryAt.Load()})
	}
	return rows
}
