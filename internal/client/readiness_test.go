package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadinessStatusAndBody(t *testing.T) {
	for _, tc := range []struct {
		code  int
		body  string
		ok    bool
		ready bool
	}{
		{200, `{"ready":true,"syncing":false,"quorum_observed":true,"consensus":{}}`, true, true},
		{503, `{"ready":false,"syncing":true,"quorum_observed":false,"consensus":{}}`, true, false},
		{503, `{"ready":true,"syncing":false,"quorum_observed":true,"consensus":{}}`, false, false},
		{200, `{}`, false, false},
	} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.code); w.Write([]byte(tc.body)) }))
		c, _ := New(s.URL, "")
		got, err := c.Readiness(context.Background())
		s.Close()
		if (err == nil) != tc.ok || got.Ready != tc.ready {
			t.Fatalf("%d: %+v %v", tc.code, got, err)
		}
	}
}
