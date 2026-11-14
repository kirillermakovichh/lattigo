package rpc

import (
	"encoding/json"
	"errors"
	"example.com/go-block-lattice/internal/protocol"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"example.com/go-block-lattice/internal/event"
	"example.com/go-block-lattice/internal/ledger"
)

const eventWriteTimeout = 5 * time.Second
const eventHeartbeat = 15 * time.Second
const maxEventData = 16 << 20

func writeEvent(w http.ResponseWriter, controller *http.ResponseController, id string, data []byte) error {
	return writeEventKind(w, controller, id, "confirmed", data)
}
func writeEventKind(w http.ResponseWriter, controller *http.ResponseController, id, kind string, data []byte) error {
	if len(data) > maxEventData {
		return errors.New("event exceeds frame budget")
	}
	if err := controller.SetWriteDeadline(time.Now().Add(eventWriteTimeout)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", id, kind, data); err != nil {
		return err
	}
	return flushEvent(controller)
}

// Clear write deadlines after each completed frame. HTTP/2 treats an expired
// deadline as a stream abort even while no write is in progress.
func flushEvent(controller *http.ResponseController) error {
	if err := controller.Flush(); err != nil {
		return err
	}
	return controller.SetWriteDeadline(time.Time{})
}

// events tails durable records directly, without retaining subscriber queues.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.RawQuery) > 512 {
		writeError(w, 400, "invalid_events", "query exceeds limit")
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, 400, "invalid_events", "invalid query")
		return
	}
	for key, values := range query {
		if (key != "address" && key != "incoming") || len(values) != 1 {
			writeError(w, 400, "invalid_events", "unknown or repeated filter")
			return
		}
	}
	address := protocol.Account(query.Get("address"))
	if address != "" {
		if _, err := address.PublicKey(); err != nil {
			writeError(w, 400, "invalid_address", err.Error())
			return
		}
	}
	incoming := query.Get("incoming") == "true"
	if value := query.Get("incoming"); value != "" && value != "true" && value != "false" {
		writeError(w, 400, "invalid_events", "incoming must be true or false")
		return
	}
	if incoming && address == "" {
		writeError(w, 400, "invalid_events", "incoming requires address")
		return
	}
	network := s.cfg.Store.Status().Network
	after := uint64(0)
	if values := r.Header.Values("Last-Event-ID"); len(values) > 1 {
		writeError(w, 400, "invalid_event_id", "multiple event identifiers")
		return
	}
	if value := r.Header.Get("Last-Event-ID"); value != "" {
		var err error
		after, err = event.ParseID(value, network)
		if err != nil {
			writeError(w, 400, "invalid_event_id", err.Error())
			return
		}
	}
	if _, _, err := s.cfg.Store.ObserveRecord(after); errors.Is(err, ledger.ErrStaleView) {
		writeError(w, 409, "event_ahead", "event height is ahead of this node")
		return
	} else if err != nil && !errors.Is(err, ledger.ErrNotFound) {
		classify(w, err)
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
	if err := flushEvent(controller); err != nil {
		return
	}
	heartbeat := time.NewTicker(eventHeartbeat)
	defer heartbeat.Stop()
	for {
		record, changed, err := s.cfg.Store.ObserveRecord(after)
		if err == nil {
			kind := "confirmed"
			var payload any = record
			match := address == "" || record.Block.Account == address
			if incoming {
				match = record.Block.Kind == "send" && record.Block.Link == string(address)
			}
			if !match {
				kind = "checkpoint"
				payload = struct {
					Height uint64 `json:"height"`
				}{record.Height}
			}
			data, err := json.Marshal(payload)
			if err != nil {
				return
			}
			if err := writeEventKind(w, controller, event.ID(network, record.Height), kind, data); err != nil {
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
			if err := flushEvent(controller); err != nil {
				return
			}
		}
	}
}
