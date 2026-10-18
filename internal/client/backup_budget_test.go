package client

import (
	"context"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
)

func TestBackupRejectsInvalidBudgetBeforeNetwork(t *testing.T) {
	for _, limit := range []int64{0, -1, math.MaxInt64} {
		c, err := New("http://local.invalid", "")
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("archive"))}, nil
		})
		if err := c.Backup(context.Background(), io.Discard, limit); err == nil {
			t.Fatal("invalid budget accepted")
		}
		if calls != 0 {
			t.Errorf("budget%d started%d backup requests", limit, calls)
		}
	}
}
