package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestErrorEnvelopeAndRedirectPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/status" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"error":{"code":"unavailable","message":"syncing"}}`))
			return
		}
		http.Redirect(w, r, "https://example.invalid", 302)
	}))
	defer server.Close()
	c, err := New(server.URL, "secret")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Status(context.Background())
	var e *Error
	if !errors.As(err, &e) || e.Code != "unavailable" {
		t.Fatalf("error contract: %v", err)
	}
	_, err = c.Genesis(context.Background())
	if err == nil {
		t.Fatal("followed RPC redirect")
	}
	if _, err = New(server.URL+"/unexpected/path", ""); err == nil {
		t.Fatal("accepted RPC path")
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type spacesReader struct{}

func (spacesReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	return len(p), nil
}

func TestCertificatePageResponseBudget(t *testing.T) {
	const metadata = `{"history_bytes":17,"history_limit_bytes":268435456,"capacity_reached":true}`
	for _, extra := range []int64{0, 1} {
		t.Run(map[int64]string{0: "at_limit", 1: "over_limit"}[extra], func(t *testing.T) {
			c, err := New("http://local.invalid", "")
			if err != nil {
				t.Fatal(err)
			}
			c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
				reader := io.MultiReader(strings.NewReader(metadata), io.LimitReader(spacesReader{}, int64(MaxResponseBytes-len(metadata))+extra))
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(reader)}, nil
			})
			status, err := c.Status(context.Background())
			if extra == 1 {
				if err == nil {
					t.Fatal("accepted oversized RPC response")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if status.HistoryBytes != 17 || status.HistoryLimitBytes != 268435456 || !status.CapacityReached {
				t.Fatalf("capacity metadata lost: %+v", status)
			}
		})
	}
}
