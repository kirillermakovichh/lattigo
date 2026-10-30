package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"example.com/go-block-lattice/internal/payment"
)

func TestRequestCommandDoesNotNeedKeysForExplicitRecipient(t *testing.T) {
	_, c := profileFixture(t)
	t.Setenv("BL_WALLET_PASSWORD", "")
	path := filepath.Join(t.TempDir(), "request-uri")
	var out bytes.Buffer
	if err := RunCommand(context.Background(), []string{"request", "--address", string(c.Address), "--network", string(c.Network), "--amount", "25", "--out", path}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	var result requestResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	parsed, err := payment.ParseURI(result.URI)
	if err != nil || parsed.Amount != 25 || parsed.Address != c.Address {
		t.Fatal("request changed recipient or exact amount")
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != result.URI+"\n" {
		t.Fatal("request output changed URI")
	}
}
