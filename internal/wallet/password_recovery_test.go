package wallet

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestPasswordRotationRejectsConflictsWithoutChangingEnvelope(t *testing.T) {
	old := []byte("old recovery passphrase")
	next := []byte("new recovery passphrase")
	dir := t.TempDir()
	path := filepath.Join(dir, "wallet")
	w, err := Import(path, make([]byte, 32), old)
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ old, next []byte }{{[]byte("wrong passphrase"), next}, {old, []byte("short")}} {
		if err := ChangePassword(path, item.old, item.next); err == nil {
			t.Fatal("invalid rotation accepted")
		}
		raw, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(raw, original) {
			t.Fatal("rejected rotation changed custody")
		}
	}
	lock, err := lockPrivate(path + ".custody.lock")
	if err != nil {
		t.Fatal(err)
	}
	if err := ChangePassword(path, old, next); !errors.Is(err, ErrAccountBusy) {
		t.Fatalf("concurrent rotation: %v", err)
	}
	lock.Close()
	alias := filepath.Join(dir, "alias")
	if err = os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if err = ChangePassword(alias, old, next); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(alias); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("rotation replaced wallet alias")
	}
}
func TestPasswordCommandRejectsInvalidInvocation(t *testing.T) {
	t.Setenv("BL_WALLET_PASSWORD", "")
	t.Setenv("BL_WALLET_NEW_PASSWORD", "")
	for _, args := range [][]string{{"password"}, {"password", "extra"}, {"password", "--unknown"}, {"password", "--timeout", "0s"}} {
		if err := RunCommand(context.Background(), args, io.Discard, io.Discard); err == nil {
			t.Fatal("invalid rotation invocation accepted")
		}
	}
	if err := RunCommand(context.Background(), []string{"password", "--help"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := RunCommand(ctx, []string{"password"}, io.Discard, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatal("rotation ignored canceled context")
	}
}
