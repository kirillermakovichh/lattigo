package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"testing"
)

func TestAccountNamesPreserveExplicitIndexes(t *testing.T) {
	path, _ := profileFixture(t)
	if err := NameAccount(path, "Savings", math.MaxUint64, false); err != nil {
		t.Fatal(err)
	}
	if err := NameAccount(path, "Savings", 0, false); !errors.Is(err, ErrNameExists) {
		t.Fatal("account label overwritten")
	}
	index, err := AccountIndex(path, "Savings")
	if err != nil || index != math.MaxUint64 {
		t.Fatal("account label truncated index")
	}
	if err := NameAccount(path, "Savings", 0, true); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunCommand(context.Background(), []string{"names", "--wallet", path}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	var rows []NamedAccount
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil || len(rows) != 1 || rows[0].Index != 0 {
		t.Fatal("account names command lost index")
	}
	if err := ForgetAccountName(path, "Savings"); err != nil {
		t.Fatal(err)
	}
	if _, err := AccountIndex(path, "Savings"); !errors.Is(err, ErrNameNotFound) {
		t.Fatal("deleted account label resolved")
	}
}
