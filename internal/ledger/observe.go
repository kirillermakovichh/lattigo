package ledger

import "example.com/go-block-lattice/internal/protocol"

// ObserveConfirmation atomically reads confirmed state and captures its next
// change notification. Callers must re-read after waking; channels carry no data.
func (s *Store) ObserveConfirmation(id protocol.Hash) (protocol.Block, <-chan struct{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return protocol.Block{}, nil, ErrClosed
	}
	if b, ok := s.state.blocks[id]; ok {
		return b, s.changed, nil
	}
	if s.failed != nil {
		return protocol.Block{}, nil, s.failed
	}
	return protocol.Block{}, s.changed, ErrNotFound
}

// ObserveRecord returns the first record after a durable height. It retains no
// subscriber queues and copies certificates so readers cannot mutate the ledger.
func (s *Store) ObserveRecord(after uint64) (Record, <-chan struct{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return Record{}, nil, ErrClosed
	}
	height := uint64(len(s.state.records))
	if after > height {
		return Record{}, nil, ErrStaleView
	}
	if after < height {
		return copyRecord(s.state.records[after]), s.changed, nil
	}
	if s.failed != nil {
		return Record{}, nil, s.failed
	}
	return Record{}, s.changed, ErrNotFound
}
