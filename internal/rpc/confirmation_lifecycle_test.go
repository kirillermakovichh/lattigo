package rpc

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/go-block-lattice/internal/protocol"
)

func TestConfirmationWaitReleasesSlotsOnCancellationAndShutdown(t *testing.T) {
	for _, mode := range []string{"context", "server"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := branchServer(t)
			id := protocol.Digest([]byte("unknown"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			request := httptest.NewRequest("GET", "/v1/confirmation?id="+string(id)+"&wait_ms=15000", nil).WithContext(ctx)
			out := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { defer close(done); s.Handler().ServeHTTP(out, request) }()
			deadline := time.After(2 * time.Second)
			for len(s.waitSlots) == 0 {
				select {
				case <-deadline:
					t.Fatal("wait was not admitted")
				case <-time.After(time.Millisecond):
				}
			}
			if mode == "context" {
				cancel()
			} else {
				s.Stop()
				s.Stop()
			}
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("long-lived reader retained resources")
			}
			if len(s.waitSlots) != 0 || len(s.slots) != 0 {
				t.Fatal("canceled wait retained admission")
			}
			if mode == "server" {
				var result confirmationResult
				if err := json.Unmarshal(out.Body.Bytes(), &result); err != nil || result.Confirmed || result.Hash != id {
					t.Fatal("shutdown fabricated confirmation")
				}
			}
		})
	}
}
func TestConfirmationAdmissionValidationAndTimeout(t *testing.T) {
	s, _ := branchServer(t)
	id := protocol.Digest([]byte("unknown"))
	base := "/v1/confirmation?id=" + string(id)
	for _, query := range []string{"/v1/confirmation?id=bad", base + "&wait_ms=15001", base + "&wait_ms=-1", base + "&other=1", base + "&wait_ms=1&wait_ms=1", base + "&wait_ms=%", base + "&wait_ms=" + strings.Repeat("1", 513)} {
		out := httptest.NewRecorder()
		s.Handler().ServeHTTP(out, httptest.NewRequest("GET", query, nil))
		if out.Code != 400 {
			t.Fatal("invalid confirmation wait admitted")
		}
	}
	for i := 0; i < cap(s.waitSlots); i++ {
		s.waitSlots <- struct{}{}
	}
	out := httptest.NewRecorder()
	s.Handler().ServeHTTP(out, httptest.NewRequest("GET", base+"&wait_ms=1", nil))
	if out.Code != 503 {
		t.Fatal("wait capacity exceeded")
	}
	for i := 0; i < cap(s.waitSlots); i++ {
		<-s.waitSlots
	}
	out = httptest.NewRecorder()
	s.Handler().ServeHTTP(out, httptest.NewRequest("GET", base+"&wait_ms=1", nil))
	var result confirmationResult
	if err := json.Unmarshal(out.Body.Bytes(), &result); err != nil || out.Code != 200 || result.Confirmed || len(s.waitSlots) != 0 {
		t.Fatal("expired wait changed confirmation or leaked admission")
	}
}
