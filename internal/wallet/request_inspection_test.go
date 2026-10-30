package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"example.com/go-block-lattice/internal/payment"
)

func TestRequestInspectionPreservesRequestAndRejectsOverrides(t *testing.T) {
	_, c := profileFixture(t)
	r := payment.Request{Address: c.Address, Network: c.Network, Amount: 9, Message: "Order 1"}
	uri, err := r.URI()
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"request", "--action", "inspect", "--uri", uri}
	var out bytes.Buffer
	if err := RunCommand(context.Background(), args, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	var result requestResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Request != r {
		t.Fatal("inspection changed payment fields")
	}
	if err := RunCommand(context.Background(), append(args, "--amount", "1"), io.Discard, io.Discard); err == nil {
		t.Fatal("inspection accepted a conflicting amount")
	}
}
