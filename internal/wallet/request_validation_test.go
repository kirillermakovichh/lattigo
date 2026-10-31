package wallet

import (
	"context"
	"io"
	"testing"
)

func TestRequestCommandsRejectAmbiguousArguments(t *testing.T) {
	_, c := profileFixture(t)
	for _, args := range [][]string{{"--unknown"}, {"extra"}, {"--timeout", "0s"}, {"--action", "other"}, {"--action", "inspect", "--uri", "bad"}, {"--action", "create", "--uri", "bad"}, {"--address", string(c.Address), "--network", string(c.Network), "--amount", "1", "--format", "other"}} {
		if err := RunCommand(context.Background(), append([]string{"request"}, args...), io.Discard, io.Discard); err == nil {
			t.Fatal("invalid request command accepted")
		}
	}
	if err := RunCommand(context.Background(), []string{"request", "--help"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
}
