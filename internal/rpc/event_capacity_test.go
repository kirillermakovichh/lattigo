package rpc

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEventSubscribersReleaseCapacityOnDisconnect(t *testing.T) {
	api, _, _ := pageFixture(t)
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	defer api.Stop()
	responses := make([]*http.Response, 0, 8)
	for i := 0; i < cap(api.eventSlots); i++ {
		resp, err := server.Client().Get(server.URL + "/v1/events")
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Fatal(resp.StatusCode)
		}
		responses = append(responses, resp)
	}
	defer func() {
		for _, r := range responses {
			r.Body.Close()
		}
	}()
	resp, err := server.Client().Get(server.URL + "/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatal("subscriber budget exceeded", resp.StatusCode)
	}
	responses[0].Body.Close()
	deadline := time.Now().Add(time.Second)
	for len(api.eventSlots) == cap(api.eventSlots) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(api.eventSlots) != cap(api.eventSlots)-1 {
		t.Fatal("disconnected subscriber retained slot")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/v1/events", nil)
	resumed, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Body.Close()
	if resumed.StatusCode != 200 {
		t.Fatal(resumed.StatusCode)
	}
	api.Stop()
	for _, response := range responses[1:] {
		_, _ = io.Copy(io.Discard, response.Body)
	}
	_, _ = io.Copy(io.Discard, resumed.Body)
	if len(api.eventSlots) != 0 {
		t.Fatal("shutdown retained subscriber slots")
	}
}

func TestEventWriterFailureReleasesAdmission(t *testing.T) {
	api, _, _ := pageFixture(t)
	writer := &eventWriter{failWrite: true}
	api.Handler().ServeHTTP(writer, httptest.NewRequest("GET", "/v1/events", nil))
	if len(api.eventSlots) != 0 || len(api.slots) != 0 {
		t.Fatal("write failure retained admission")
	}
	writer = &eventWriter{failFlush: true}
	api.Handler().ServeHTTP(writer, httptest.NewRequest("GET", "/v1/events", nil))
	if len(api.eventSlots) != 0 || len(api.slots) != 0 {
		t.Fatal("flush failure retained admission")
	}
}
