package rpc

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

type confirmationResult struct {
	Hash      protocol.Hash   `json:"hash"`
	Confirmed bool            `json:"confirmed"`
	Block     *protocol.Block `json:"block,omitempty"`
}

func (s *Server) writeConfirmed(w http.ResponseWriter, id protocol.Hash) bool {
	b, err := s.cfg.Store.Block(id)
	if err == nil {
		writeJSON(w, 200, confirmationResult{Hash: id, Confirmed: true, Block: &b})
		return true
	}
	if !errors.Is(err, ledger.ErrNotFound) {
		classify(w, err)
		return true
	}
	return false
}
func (s *Server) confirmation(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.RawQuery) > 512 {
		writeError(w, 400, "invalid_confirmation", "confirmation query exceeds limit")
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, 400, "invalid_confirmation", "invalid query")
		return
	}
	for key, values := range query {
		if (key != "id" && key != "wait_ms") || len(values) != 1 {
			writeError(w, 400, "invalid_confirmation", "unknown or repeated confirmation field")
			return
		}
	}
	id := protocol.Hash(query.Get("id"))
	if _, err := id.Bytes(); err != nil {
		writeError(w, 400, "invalid_hash", "id must be a canonical hash")
		return
	}
	wait := uint64(0)
	if value := query.Get("wait_ms"); value != "" {
		wait, err = strconv.ParseUint(value, 10, 16)
		if err != nil || wait > 15000 {
			writeError(w, 400, "invalid_wait", "wait_ms must be 0..15000")
			return
		}
	}
	if s.writeConfirmed(w, id) {
		return
	}
	if wait == 0 {
		writeJSON(w, 200, confirmationResult{Hash: id})
		return
	}
	select {
	case s.waitSlots <- struct{}{}:
		defer func() { <-s.waitSlots }()
	default:
		writeError(w, 503, "overloaded", "confirmation wait capacity reached")
		return
	}
	deadline := time.NewTimer(time.Duration(wait) * time.Millisecond)
	defer deadline.Stop()
	for {
		b, changed, err := s.cfg.Store.ObserveConfirmation(id)
		if err == nil {
			writeJSON(w, 200, confirmationResult{Hash: id, Confirmed: true, Block: &b})
			return
		}
		if !errors.Is(err, ledger.ErrNotFound) {
			classify(w, err)
			return
		}
		select {
		case <-s.stopping:
			if !s.writeConfirmed(w, id) {
				writeJSON(w, 200, confirmationResult{Hash: id})
			}
			return
		case <-r.Context().Done():
			return
		case <-deadline.C:
			if !s.writeConfirmed(w, id) {
				writeJSON(w, 200, confirmationResult{Hash: id})
			}
			return
		case <-changed:
		}
	}
}
