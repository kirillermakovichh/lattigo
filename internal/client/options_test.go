package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestClientOptionsPreserveTransportAndRedirectPolicy(t *testing.T) {
	visited := false
	tr := transportFunc(func(r *http.Request) (*http.Response, error) {
		visited = true
		if r.Header.Get("User-Agent") != "Custom/1" {
			t.Error("user agent ignored")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})
	c, err := NewWithOptions("http://local.invalid", "", WithTimeout(time.Second), WithTransport(tr), WithUserAgent("Custom/1"))
	if err != nil {
		t.Fatal(err)
	}
	if c.http.Timeout != time.Second || c.http.CheckRedirect == nil {
		t.Fatal("client options removed redirect or timeout policy")
	}
	if _, err := c.Status(context.Background()); err != nil || !visited {
		t.Fatal("configured transport unused")
	}
	c.CloseIdleConnections()
	for _, option := range []Option{nil, WithTimeout(0), WithTimeout(11 * time.Minute), WithTransport(nil), WithUserAgent(""), WithUserAgent("bad\nagent"), WithUserAgent(strings.Repeat("x", 129))} {
		if _, err := NewWithOptions("http://local.invalid", "", option); err == nil {
			t.Fatal("invalid client option accepted")
		}
	}
}
