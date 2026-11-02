// Package rpc exposes the versioned payment API and authenticated operator endpoints.
package rpc

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"example.com/go-block-lattice/internal/consensus"
	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

const maxBody = 1 << 20

type Config struct {
	Store        *ledger.Store
	Engine       *consensus.Engine
	Token        string
	PublishBlock func(protocol.Block)
	Syncing      func() bool
	PeerStats    func() any
}

type Server struct {
	stopping   chan struct{}
	stopOnce   sync.Once
	cursors    cursorCodec
	cfg        Config
	slots      chan struct{}
	backupSlot chan struct{}
	waitSlots  chan struct{}
	requests   atomic.Uint64
	rejected   atomic.Uint64
}

func New(cfg Config) (*Server, error) {
	if cfg.Store == nil || cfg.Engine == nil {
		return nil, errors.New("RPC requires ledger and consensus engine")
	}
	var key cursorCodec
	if _, err := rand.Read(key[:]); err != nil {
		return nil, err
	}
	return &Server{cfg: cfg, stopping: make(chan struct{}), cursors: key, slots: make(chan struct{}, 32), backupSlot: make(chan struct{}, 1), waitSlots: make(chan struct{}, 8)}, nil
}

// Stop releases long-lived readers without closing shared ledger state.
func (s *Server) Stop() { s.stopOnce.Do(func() { close(s.stopping) }) }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", s.status)
	mux.HandleFunc("GET /v1/genesis", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, s.cfg.Store.Genesis()) })
	mux.HandleFunc("GET /v1/account", s.account)
	mux.HandleFunc("GET /v1/receivables", s.receivables)
	mux.HandleFunc("GET /v1/receivables/page", s.receivablePage)
	mux.HandleFunc("GET /v1/history", s.history)
	mux.HandleFunc("GET /v1/history/page", s.historyPage)
	mux.HandleFunc("GET /v1/block", s.block)
	mux.HandleFunc("GET /v1/confirmation", s.confirmation)
	mux.HandleFunc("POST /v1/blocks", s.submit)
	mux.HandleFunc("POST /v1/query", s.batchRead)
	mux.HandleFunc("GET /v1/admin/backup", s.backup)
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"live": true})
	})
	mux.HandleFunc("GET /health/ready", s.ready)
	mux.HandleFunc("GET /metrics", s.metrics)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		select {
		case s.slots <- struct{}{}:
			defer func() { <-s.slots }()
		default:
			s.rejected.Add(1)
			writeError(w, 503, "overloaded", "RPC concurrency limit reached")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) authenticated(w http.ResponseWriter, r *http.Request) bool {
	if s.cfg.Token == "" {
		writeError(w, 403, "read_only", "configure an RPC token to enable writes and administration")
		return false
	}
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	actual := sha256.Sum256([]byte(provided))
	expected := sha256.Sum256([]byte(s.cfg.Token))
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
		writeError(w, 401, "unauthorized", "valid bearer token required")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func classify(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ledger.ErrCapacity):
		writeError(w, 503, "capacity_reached", err.Error())
	case errors.Is(err, consensus.ErrBusy):
		writeError(w, 503, "overloaded", err.Error())
	case errors.Is(err, consensus.ErrStale):
		writeError(w, 409, "stale", err.Error())
	case errors.Is(err, ledger.ErrNotFound):
		writeError(w, 404, "not_found", err.Error())
	case errors.Is(err, ledger.ErrConflict):
		writeError(w, 409, "conflict", err.Error())
	case errors.Is(err, ledger.ErrDependency):
		writeError(w, 409, "missing_dependency", err.Error())
	case errors.Is(err, ledger.ErrInvalid):
		writeError(w, 400, "invalid", err.Error())
	case errors.Is(err, ledger.ErrClosed):
		writeError(w, 503, "unavailable", "ledger is unavailable")
	default:
		writeError(w, 503, "unavailable", "node cannot safely complete this request; query the block hash before retrying")
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON value")
	}
	return nil
}

func address(r *http.Request) (protocol.Account, error) {
	a := protocol.Account(r.URL.Query().Get("address"))
	_, err := a.PublicKey()
	return a, err
}
func page(r *http.Request) (int, int, error) {
	offset, limit := 0, 100
	var err error
	if value := r.URL.Query().Get("offset"); value != "" {
		offset, err = strconv.Atoi(value)
		if err != nil || offset < 0 || offset > 10000000 {
			return 0, 0, errors.New("offset must be between 0 and 10000000")
		}
	}
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 1000 {
			return 0, 0, errors.New("limit must be between 1 and 1000")
		}
	}
	return offset, limit, nil
}
func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.cfg.Store.Status())
}
func (s *Server) account(w http.ResponseWriter, r *http.Request) {
	a, err := address(r)
	if err != nil {
		writeError(w, 400, "invalid_address", err.Error())
		return
	}
	state, err := s.cfg.Store.Account(a)
	if err != nil {
		classify(w, err)
		return
	}
	writeJSON(w, 200, state)
}
func (s *Server) receivables(w http.ResponseWriter, r *http.Request) {
	a, err := address(r)
	if err != nil {
		writeError(w, 400, "invalid_address", err.Error())
		return
	}
	offset, limit, err := page(r)
	if err != nil {
		writeError(w, 400, "invalid_page", err.Error())
		return
	}
	items := s.cfg.Store.Receivables(a, offset, limit)
	if items == nil {
		items = []ledger.Receivable{}
	}
	writeJSON(w, 200, items)
}
func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	a, err := address(r)
	if err != nil {
		writeError(w, 400, "invalid_address", err.Error())
		return
	}
	offset, limit, err := page(r)
	if err != nil {
		writeError(w, 400, "invalid_page", err.Error())
		return
	}
	items := s.cfg.Store.History(a, offset, limit)
	if items == nil {
		items = []ledger.Record{}
	}
	writeJSON(w, 200, items)
}
func (s *Server) block(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	raw, err := hex.DecodeString(id)
	if err != nil || len(raw) != 32 || strings.ToLower(id) != id {
		writeError(w, 400, "invalid_hash", "id must be a lowercase SHA-256 hash")
		return
	}
	block, err := s.cfg.Store.Block(protocol.Hash(id))
	if err != nil {
		classify(w, err)
		return
	}
	writeJSON(w, 200, block)
}
func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	if !s.authenticated(w, r) {
		return
	}
	var block protocol.Block
	if err := decode(w, r, &block); err != nil {
		writeError(w, 400, "invalid_json", err.Error())
		return
	}
	// Confirmed retries must not be rejected because their predecessor is no longer current.
	if existing, err := s.cfg.Store.Block(block.ID()); err == nil {
		if existing.Signature != block.Signature || block.Verify() != nil || !block.CheckWork(s.cfg.Store.Genesis().WorkBits) {
			writeError(w, 400, "invalid", "invalid duplicate block")
			return
		}
		writeJSON(w, 200, map[string]any{"hash": block.ID(), "status": "confirmed"})
		return
	}
	hash, err := s.cfg.Engine.Submit(block)
	if err != nil {
		classify(w, err)
		return
	}
	if s.cfg.PublishBlock != nil {
		s.cfg.PublishBlock(block)
	}
	writeJSON(w, 202, map[string]any{"hash": hash, "status": "admitted"})
}
func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	if !s.authenticated(w, r) {
		return
	}
	select {
	case s.backupSlot <- struct{}{}:
		defer func() { <-s.backupSlot }()
	default:
		writeError(w, 503, "overloaded", "another backup is in progress")
		return
	}
	file, err := os.CreateTemp("", "bl-ledger-backup-*")
	if err != nil {
		writeError(w, 503, "backup_failed", "cannot create backup staging file")
		return
	}
	defer func() { file.Close(); os.Remove(file.Name()) }()
	// Stage before publishing headers so persistence failures cannot look like HTTP success.
	if err := s.cfg.Store.Backup(file); err != nil {
		writeError(w, 503, "backup_failed", "ledger backup failed")
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writeError(w, 503, "backup_failed", "cannot read backup staging file")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="ledger.backup"`)
	http.ServeContent(w, r, "ledger.backup", time.Time{}, file)
}
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	health := s.cfg.Engine.Health()
	ready := !s.cfg.Store.Status().CapacityReached
	if value, ok := health["ready"].(bool); ok {
		ready = ready && value
	}
	if value, ok := health["error"].(string); ok && value != "" {
		ready = false
	}
	// While work waits, distinguish a responsive process from observed voting quorum.
	observed, _ := health["observed_voting_weight"].(protocol.Amount)
	quorum, _ := health["quorum_weight"].(protocol.Amount)
	quorumObserved := quorum > 0 && observed >= quorum
	if pending, ok := health["pending"].(int); ok && pending > 0 && !quorumObserved {
		ready = false
	}
	syncing := s.cfg.Syncing != nil && s.cfg.Syncing()
	if syncing {
		ready = false
	}
	status := 200
	if !ready {
		status = 503
	}
	writeJSON(w, status, map[string]any{"ready": ready, "syncing": syncing, "consensus": health, "quorum_observed": quorumObserved})
}
func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	state := s.cfg.Store.Status()
	_, _ = fmt.Fprintf(w, "# TYPE bl_ledger_height gauge\nbl_ledger_height %d\n# TYPE bl_rpc_requests_total counter\nbl_rpc_requests_total %d\n# TYPE bl_rpc_rejected_total counter\nbl_rpc_rejected_total %d\n", state.Height, s.requests.Load(), s.rejected.Load())
	_, _ = fmt.Fprintf(w, "bl_ledger_history_bytes %d\nbl_ledger_history_limit_bytes %d\n", state.HistoryBytes, state.HistoryLimitBytes)
	health := s.cfg.Engine.Health()
	for _, name := range []string{"pending", "round", "observed_voting_weight", "quorum_weight"} {
		switch value := health[name].(type) {
		case int:
			_, _ = fmt.Fprintf(w, "bl_consensus_%s %d\n", name, value)
		case uint32:
			_, _ = fmt.Fprintf(w, "bl_consensus_%s %d\n", name, value)
		case protocol.Amount:
			_, _ = fmt.Fprintf(w, "bl_consensus_%s %d\n", name, uint64(value))
		}
	}
	if s.cfg.PeerStats != nil {
		stats := s.cfg.PeerStats()
		if value, ok := stats.(map[string]any); ok {
			for _, name := range []string{"peers", "connected", "dropped", "received", "queued_bytes"} {
				if n, ok := value[name].(int); ok {
					_, _ = fmt.Fprintf(w, "bl_p2p_%s %d\n", name, n)
				}
			}
		}
	}
}
