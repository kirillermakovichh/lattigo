package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestNamedAccountSelectionUsesRecordedIndex(t *testing.T) {
	t.Setenv("BL_WALLET_PASSWORD", "private named account passphrase")
	path := filepath.Join(t.TempDir(), "wallet")
	w, err := Import(path, make([]byte, 32), []byte(os.Getenv("BL_WALLET_PASSWORD")))
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	if err := NameAccount(path, "Savings", 7, false); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	args := []string{"accounts", "--wallet", path, "--account-name", "Savings"}
	if err := RunCommand(context.Background(), args, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Index uint64 `json:"index"`
	}
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil || len(rows) != 1 || rows[0].Index != 7 {
		t.Fatal("named account selected a different index")
	}
	for _, extra := range [][]string{{"--index", "0"}, {"--account-name", "absent"}} {
		if err := RunCommand(context.Background(), append(args, extra...), io.Discard, io.Discard); err == nil {
			t.Fatal("ambiguous or absent account label accepted")
		}
	}
}
