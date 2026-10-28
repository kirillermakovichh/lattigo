package wallet

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileRejectsCorruptionAndExposedFiles(t *testing.T) {
	path, c := profileFixture(t)
	for _, raw := range []string{"{", "{}", `{"version":2}`, `{"version":1,"unknown":true}`, `{"version":1} {}`, `{"version":1,"contacts":{"bad":{"address":"bad","network":"bad"}}}`} {
		if err := os.WriteFile(path+".profile.json", []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Contacts(path); err == nil {
			t.Fatalf("invalid profile accepted: %s", raw)
		}
	}
	if err := os.Remove(path + ".profile.json"); err != nil {
		t.Fatal(err)
	}
	if err := PutContact(path, "Alice", c, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path+".profile.json", 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Contacts(path); err == nil {
		t.Fatal("exposed profile accepted")
	}
	if err := os.Chmod(path+".profile.json", 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path+".profile.json", os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(maxProfileBytes + 1); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err := Contacts(path); err == nil {
		t.Fatal("oversized profile accepted")
	}
}
func TestProfileMutationIsSerializedAcrossAliases(t *testing.T) {
	path, c := profileFixture(t)
	if err := PutContact(path, "Alice", c, false); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path + ".profile.json")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := lockPrivate(path + ".profile.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	alias := filepath.Join(filepath.Dir(path), "alias")
	if err = os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if err := PutContact(alias, "Bob", c, false); !errors.Is(err, ErrAccountBusy) {
		t.Fatalf("alias update bypassed profile lock: %v", err)
	}
	for _, name := range []string{"", " leading", "trailing ", "line\nbreak", strings.Repeat("n", 65), string([]byte{255})} {
		if validName(name) {
			t.Fatal("invalid name accepted")
		}
	}
	raw, err := os.ReadFile(path + ".profile.json")
	if err != nil || !bytes.Equal(raw, original) {
		t.Fatal("rejected profile mutation changed recipients")
	}
}
func TestContactCommandValidation(t *testing.T) {
	path, _ := profileFixture(t)
	for _, args := range [][]string{{"--unknown"}, {"extra"}, {"--action", "invalid"}, {"--action", "add"}, {"--action", "import"}, {"--action", "export"}, {"--action", "remove", "--name", "absent"}} {
		command := append([]string{"contacts", "--wallet", path}, args...)
		if err := RunCommand(context.Background(), command, io.Discard, io.Discard); err == nil {
			t.Fatalf("invalid contact command accepted: %v", args)
		}
	}
	if err := RunCommand(context.Background(), []string{"contacts", "--help"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
}
