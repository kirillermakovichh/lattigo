package node

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestShutdownClosesAdmissionAndJoinsHandlers(t *testing.T) {
	group := &requestGroup{}
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	handler := group.handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-release; w.WriteHeader(204) }))
	go func() {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
		close(finished)
	}()
	<-started
	group.stop()
	rejected := httptest.NewRecorder()
	handler.ServeHTTP(rejected, httptest.NewRequest("GET", "/", nil))
	if rejected.Code != 503 {
		t.Fatalf("admission remained open: %d", rejected.Code)
	}
	done := make(chan struct{})
	go func() { group.wait(); close(done) }()
	select {
	case <-done:
		t.Fatal("shutdown completed before handler")
	default:
	}
	close(release)
	<-finished
	<-done
}
