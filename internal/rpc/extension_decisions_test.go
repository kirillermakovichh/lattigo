package rpc

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"example.com/go-block-lattice/internal/protocol"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestExtensionRPCRejections(t *testing.T) {
	s, key := branchServer(t)
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	call := func(path string, auth bool) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", path, nil)
		if auth {
			r.Header.Set("Authorization", "Bearer private-branch-token")
		}
		s.Handler().ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/v1/events?" + strings.Repeat("x", 513), "/v1/events?%xx", "/v1/events?unknown=1", "/v1/events?address=bad", "/v1/events?incoming=bad", "/v1/events?incoming=true", "/v1/history/page?%xx", "/v1/history/page", "/v1/receivables/page", "/v1/confirmation"} {
		if w := call(path, false); w.Code != 400 {
			t.Fatal(path, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "/v1/events", nil)
	r.Header.Add("Last-Event-ID", "one")
	r.Header.Add("Last-Event-ID", "two")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	for _, path := range []string{"/v1/admin/profile?" + strings.Repeat("x", 129), "/v1/admin/profile?kind=heap&bad=1", "/v1/admin/profile?kind=heap&kind=cpu", "/v1/admin/profile"} {
		if w := call(path, true); w.Code != 400 {
			t.Fatal(path, w.Code)
		}
	}
	for _, q := range []readQuery{{Operation: "status", Address: a}, {Operation: "account", Address: "bad"}, {Operation: "block", Hash: "bad"}} {
		if s.readOne(q).Error == nil {
			t.Fatal(q)
		}
	}
	if validQueryID("a\nb") {
		t.Fatal("control ID")
	}
	var list queryList
	for _, raw := range []string{"", `[1]`, `[`, `[] {}`} {
		if list.UnmarshalJSON([]byte(raw)) == nil {
			t.Fatal(raw)
		}
	}
	if w := call("/v1/account?address="+string(a), false); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := call("/v1/block?id="+strings.Repeat("a", 64), false); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := call("/v1/block?id=bad", false); w.Code != 400 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/v1/blocks", strings.NewReader(`{}`)))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	s.cfg.Store.Close()
	if result := s.readOne(readQuery{Operation: "account", Address: a}); result.Error == nil || result.Error.Code != "unavailable" {
		t.Fatal(result)
	}
	for _, path := range []string{"/v1/events", "/v1/confirmation?id=" + strings.Repeat("a", 64), "/v1/history/page?address=" + string(a), "/v1/receivables/page?address=" + string(a)} {
		if w := call(path, false); w.Code != 503 {
			t.Fatal(path, w.Code)
		}
	}
}

func TestExtensionProfileLifecycle(t *testing.T) {
	for _, mode := range []string{"normal", "cancel", "stop", "already-active"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := branchServer(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			if mode == "stop" {
				s.Stop()
			}
			if mode == "already-active" {
				if err := pprof.StartCPUProfile(io.Discard); err != nil {
					t.Fatal(err)
				}
				defer pprof.StopCPUProfile()
			}
			r := httptest.NewRequest("GET", "/v1/admin/profile?kind=cpu&seconds=1", nil).WithContext(ctx)
			r.Header.Set("Authorization", "Bearer private-branch-token")
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if mode == "normal" && (w.Code != 200 || w.Body.Len() == 0) {
				t.Fatal(w.Code)
			}
			if mode == "already-active" && w.Code != 503 {
				t.Fatal(w.Code)
			}
		})
	}
}

var _ http.ResponseWriter = (*httptest.ResponseRecorder)(nil)

type stagedEventWriter struct {
	eventWriter
	deadlines                   int
	failAtDeadline, failAtFlush int
	beforeWrite                 func([]byte)
	afterHeader                 func()
}

func (w *stagedEventWriter) SetWriteDeadline(at time.Time) error {
	w.deadlines++
	if w.afterHeader != nil && w.deadlines == 1 {
		w.afterHeader()
	}
	if w.deadlines == w.failAtDeadline {
		return errors.New("deadline unavailable")
	}
	return w.eventWriter.SetWriteDeadline(at)
}
func (w *stagedEventWriter) FlushError() error {
	if w.flushes+1 == w.failAtFlush {
		return errors.New("flush unavailable")
	}
	return w.eventWriter.FlushError()
}
func (w *stagedEventWriter) Write(p []byte) (int, error) {
	if w.beforeWrite != nil {
		w.beforeWrite(p)
	}
	return w.eventWriter.Write(p)
}

func TestExtensionEventLifecycleEdges(t *testing.T) {
	for _, mode := range []string{"cancel-after-frame", "stop-after-frame", "closed-after-headers", "checkpoint", "initial-deadline", "clear-deadline"} {
		t.Run(mode, func(t *testing.T) {
			s, a, _ := pageFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			w := &stagedEventWriter{}
			path := "/v1/events"
			switch mode {
			case "cancel-after-frame":
				w.beforeWrite = func([]byte) { cancel() }
			case "stop-after-frame":
				w.beforeWrite = func([]byte) { s.Stop() }
			case "closed-after-headers":
				w.afterHeader = func() { s.cfg.Store.Close() }
			case "checkpoint":
				key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, 32))
				other := protocol.Address(key.Public().(ed25519.PublicKey))
				if other == a {
					t.Fatal("fixture address collision")
				}
				path += "?address=" + string(other)
				w.beforeWrite = func(p []byte) {
					if !bytes.Contains(p, []byte("event: checkpoint")) {
						t.Error("unrelated confirmation was exposed")
					}
					cancel()
				}
			case "initial-deadline":
				w.failAtDeadline = 1
			case "clear-deadline":
				w.failAtDeadline = 2
			}
			s.events(w, httptest.NewRequest("GET", path, nil).WithContext(ctx))
			if len(s.eventSlots) != 0 {
				t.Fatal("event admission retained")
			}
		})
	}
}

func TestExtensionEventHeartbeatEdges(t *testing.T) {
	for _, mode := range []string{"deadline", "write", "flush", "success"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			s, _ := branchServer(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			w := &stagedEventWriter{}
			switch mode {
			case "deadline":
				w.failAtDeadline = 3
			case "write":
				w.failWrite = true
			case "flush":
				w.failAtFlush = 2
			case "success":
				w.beforeWrite = func(p []byte) {
					if !bytes.Equal(p, []byte(": heartbeat\n\n")) {
						t.Error("unexpected heartbeat")
					}
					cancel()
				}
			}
			s.events(w, httptest.NewRequest("GET", "/v1/events", nil).WithContext(ctx))
			if ctx.Err() != nil && mode != "success" {
				t.Fatal("heartbeat branch did not terminate within budget")
			}
			if len(s.eventSlots) != 0 {
				t.Fatal("heartbeat retained subscriber")
			}
		})
	}
}

func TestExtensionTerminalPagesAndCursorScope(t *testing.T) {
	s, a, _ := pageFixture(t)
	for _, path := range []string{"/v1/history/page?address=", "/v1/receivables/page?address="} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", path+string(a)+"&limit=100", nil))
		if w.Code != 200 || strings.Contains(w.Body.String(), `"next"`) {
			t.Fatal("terminal page", w.Code, w.Body.String())
		}
	}
	if err := (pageCursor{Version: 1, Network: s.cfg.Store.Status().Network, Account: a, Tip: s.cfg.Store.Status().Tip, Kind: "receivables", Limit: 1}).validate(); err != nil {
		t.Fatal(err)
	}
}

// observationContext coordinates a state transition exactly when a waiter has
// captured its atomic observation and evaluates cancellation before sleeping.
type observationContext struct {
	context.Context
	once       sync.Once
	transition func()
}

func (c *observationContext) Done() <-chan struct{} { c.once.Do(c.transition); return nil }

func TestExtensionConfirmationTerminalRereads(t *testing.T) {
	for _, mode := range []string{"shutdown", "timeout", "closed"} {
		t.Run(mode, func(t *testing.T) {
			// Both ready select arms are legal: waking on the durable channel or a
			// terminal signal must return the same confirmed block in every schedule.
			for i := 0; i < 16; i++ {
				s, key := branchServer(t)
				a := s.cfg.Store.Genesis().Allocations[0].Account
				b := protocol.Block{Version: 1, Network: s.cfg.Store.Status().Network, Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(a)}
				if err := b.Sign(key); err != nil {
					t.Fatal(err)
				}
				ctx := &observationContext{Context: context.Background(), transition: func() {
					if mode == "closed" {
						s.cfg.Store.Close()
						return
					}
					if _, err := s.cfg.Engine.Submit(b); err != nil {
						t.Fatal(err)
					}
					if mode == "shutdown" {
						s.Stop()
					} else {
						timer := time.NewTimer(2 * time.Millisecond)
						<-timer.C
					}
				}}
				w := httptest.NewRecorder()
				s.confirmation(w, httptest.NewRequest("GET", "/v1/confirmation?id="+string(b.ID())+"&wait_ms=1", nil).WithContext(ctx))
				if mode == "closed" {
					if w.Code != 503 {
						t.Fatal(w.Code)
					}
				} else if w.Code != 200 || !strings.Contains(w.Body.String(), `"confirmed":true`) {
					t.Fatal(w.Code, w.Body.String())
				}
			}
		})
	}
}

func TestPageEndpointRejectsMalformedSignedCursor(t *testing.T) {
	s, a, _ := pageFixture(t)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/v1/history/page?address="+string(a)+"&cursor=malformed", nil))
	if w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
}
