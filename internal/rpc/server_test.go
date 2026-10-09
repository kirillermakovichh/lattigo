package rpc

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthenticationIsExplicitAndReadOnlyByDefault(t *testing.T) {
	for _, tc := range []struct {
		token, header string
		want          bool
		status        int
	}{{"", "", false, 403}, {"0123456789abcdef", "", false, 401}, {"0123456789abcdef", "Bearer wrong", false, 401}, {"0123456789abcdef", "Bearer 0123456789abcdef", true, 200}} {
		s := &Server{cfg: Config{Token: tc.token}}
		request := httptest.NewRequest("POST", "/v1/blocks", nil)
		request.Header.Set("Authorization", tc.header)
		response := httptest.NewRecorder()
		if got := s.authenticated(response, request); got != tc.want {
			t.Errorf("authentication: got %v want %v", got, tc.want)
		}
		if response.Code != tc.status {
			t.Errorf("status %d want %d", response.Code, tc.status)
		}
	}
}
func TestStrictBoundedJSON(t *testing.T) {
	for _, raw := range []string{`{"known":1,"unknown":2}`, `{"known":1} {"known":2}`, strings.Repeat(" ", maxBody+1) + `{"known":1}`} {
		request := httptest.NewRequest("POST", "/v1/blocks", strings.NewReader(raw))
		var value struct {
			Known int `json:"known"`
		}
		if err := decode(httptest.NewRecorder(), request, &value); err == nil {
			t.Error("accepted malformed, extra or oversized input")
		}
	}
}
