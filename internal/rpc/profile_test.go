package rpc

import (
	"bytes"
	"net/http/httptest"
	"testing"
)

func TestProfileBufferBound(t *testing.T) {
	var b profileBuffer
	if _, err := b.Write(make([]byte, maxProfileBytes)); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte{1}); err == nil || b.Len() != maxProfileBytes {
		t.Fatal("profile budget not enforced")
	}
}
func TestProfileAuthenticationAndCollection(t *testing.T) {
	s := &Server{cfg: Config{Token: "test-token"}, profileSlots: make(chan struct{}, 1), stopping: make(chan struct{})}
	for _, tc := range []struct {
		query, token string
		code         int
	}{{"kind=heap", "", 401}, {"kind=unknown", "test-token", 400}, {"kind=heap&seconds=1", "test-token", 400}, {"kind=heap", "test-token", 200}} {
		r := httptest.NewRequest("GET", "/v1/admin/profile?"+tc.query, nil)
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		s.profile(w, r)
		if w.Code != tc.code {
			t.Fatalf("%s: %d", tc.query, w.Code)
		}
		if tc.code == 200 && !bytes.HasPrefix(w.Body.Bytes(), []byte{0x1f, 0x8b}) {
			t.Fatal("profile is not gzip pprof")
		}
	}
	s.profileSlots <- struct{}{}
	r := httptest.NewRequest("GET", "/v1/admin/profile?kind=heap", nil)
	r.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	s.profile(w, r)
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}
