package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
)

func TestContactCommandRoundTrip(t *testing.T) {
	path, c := profileFixture(t)
	invoke := func(args ...string) []byte {
		t.Helper()
		var out bytes.Buffer
		args = append([]string{"contacts", "--wallet", path}, args...)
		if err := RunCommand(context.Background(), args, &out, io.Discard); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	invoke("--action", "add", "--name", "Alice", "--address", string(c.Address), "--network", string(c.Network))
	var rows []NamedContact
	if err := json.Unmarshal(invoke(), &rows); err != nil || len(rows) != 1 || rows[0].Contact != c {
		t.Fatal("contact command lost recipient")
	}
	invoke("--action", "remove", "--name", "Alice")
	if string(invoke()) != "[]\n" {
		t.Fatal("contact deletion failed")
	}
}
