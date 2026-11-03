package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"example.com/go-block-lattice/internal/ledger"
)

const eventWriteTimeout = 5 * time.Second
const eventHeartbeat = 15 * time.Second

// events tails durable records directly, without retaining subscriber queues.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		writeError(w, 400, "invalid_events", "event query fields are unsupported")
		return
	}
	select {
	case s.eventSlots <- struct{}{}:
		defer func() { <-s.eventSlots }()
	default:
		writeError(w, 503, "overloaded", "event subscriber capacity reached")
		return
	}
	controller := http.NewResponseController(w)
	if err := controller.SetWriteDeadline(time.Now().Add(eventWriteTimeout)); err != nil {
		writeError(w, 503, "stream_unavailable", "stream deadlines unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	if err := controller.Flush(); err != nil {
		return
	}
	heartbeat := time.NewTicker(eventHeartbeat)
	defer heartbeat.Stop()
	after := uint64(0)
	for {
		record, changed, err := s.cfg.Store.ObserveRecord(after)
		if err == nil {
			data, err := json.Marshal(record)
			if err != nil {
				return
			}
			if err := controller.SetWriteDeadline(time.Now().Add(eventWriteTimeout)); err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "event: confirmed\ndata: %s\n\n", data); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
			after = record.Height
			select {
			case <-r.Context().Done():
				return
			case <-s.stopping:
				return
			default:
			}
			continue
		}
		if !errors.Is(err, ledger.ErrNotFound) {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-s.stopping:
			return
		case <-changed:
		case <-heartbeat.C:
			if err := controller.SetWriteDeadline(time.Now().Add(eventWriteTimeout)); err != nil {
				return
			}
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		}
	}
}
