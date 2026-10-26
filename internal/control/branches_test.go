package control

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

type failedOutput struct{}

func (failedOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestAdministrationArgumentAndFileBranches(t *testing.T) {
	t.Setenv("BL_RPC_TOKEN", "")
	ctx := context.Background()
	dir := t.TempDir()
	for _, args := range [][]string{{"unknown"}, {"token"}, {"token", "--unknown"}, {"token", "extra"}, {"genesis", "--work-bits", "25"}, {"genesis", "--balance", "bad"}, {"genesis", "--name", "bad", "--balance", "0"}, {"genesis", "--allocations", "absent", "--account", "other"}, {"genesis", "--allocations", "absent"}, {"config", "--token-file", "missing"}, {"status", "--timeout", "0s"}, {"status", "--token-file", "missing"}, {"status", "--rpc", "bad"}, {"backup", "--max-bytes", "0"}, {"backup", "--out", filepath.Join(dir, "backup"), "--max-bytes", "0"}, {"backup", "--out", filepath.Join(dir, "backup"), "--max-bytes", "9223372036854775807"}, {"restore", "--genesis", "missing"}} {
		if RunControl(ctx, args, io.Discard, io.Discard) == nil {
			t.Errorf("invalid command accepted:%v", args)
		}
	}
	if RunControl(ctx, []string{"help"}, failedOutput{}, io.Discard) == nil {
		t.Fatal("help output failure ignored")
	}
	if RunControl(ctx, []string{"token", "--help"}, io.Discard, io.Discard) != nil {
		t.Fatal("command help failed")
	}
	token := filepath.Join(dir, "token")
	if err := RunControl(ctx, []string{"token", "--out", token}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	secret, err := controlSecret(token)
	if err != nil || len(secret) != 64 {
		t.Fatal("generated token invalid")
	}
	for _, path := range []string{dir, filepath.Join(dir, "missing")} {
		if _, err := controlSecret(path); err == nil {
			t.Fatal("invalid token file accepted")
		}
	}
	large := filepath.Join(dir, "large")
	if err = os.WriteFile(large, make([]byte, 16<<10+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = controlSecret(large); err == nil {
		t.Fatal("oversized token accepted")
	}
	exposed := filepath.Join(dir, "exposed")
	if err = os.WriteFile(exposed, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = controlSecret(exposed); err == nil {
		t.Fatal("exposed token accepted")
	}
	errorPath := filepath.Join(dir, "write-error")
	if writeNew(errorPath, func(io.Writer) error { return errors.New("upstream failure") }) == nil {
		t.Fatal("writer failure ignored")
	}
	if _, err = os.Stat(errorPath); !os.IsNotExist(err) {
		t.Fatal("failed output published")
	}
	if writeNew(filepath.Join(token, "nested"), func(io.Writer) error { return nil }) == nil {
		t.Fatal("file used as directory")
	}
	if controlOutput(failedOutput{}, map[string]int{"x": 1}) == nil {
		t.Fatal("structured output failure ignored")
	}
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	other := protocol.Address(ed25519.NewKeyFromSeed([]byte(strings.Repeat("x", 32))).Public().(ed25519.PublicKey))
	allocations := filepath.Join(dir, "allocations.json")
	cases := [][]byte{[]byte("{"), []byte("[]{}"), []byte("[]")}
	overflow, _ := json.Marshal([]protocol.Allocation{{Account: a, Balance: protocol.Amount(math.MaxUint64), Representative: a}, {Account: other, Balance: 1, Representative: a}})
	cases = append(cases, overflow)
	for _, raw := range cases {
		if err = os.WriteFile(allocations, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if RunControl(ctx, []string{"genesis", "--name", "bad", "--allocations", allocations, "--out", filepath.Join(dir, "genesis.json")}, io.Discard, io.Discard) == nil {
			t.Fatal("invalid allocations accepted")
		}
	}
	var peers peerFlags
	if err = peers.Set("http://127.0.0.1:7071"); err != nil || peers.String() != "http://127.0.0.1:7071" {
		t.Fatal("repeatable peer flag failed")
	}
}
func TestAdministrationHTTPAndRestoreOutcomes(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	g := protocol.Genesis{Version: 1, Name: "control-branches", Supply: 100, Allocations: []protocol.Allocation{{Account: a, Balance: 100, Representative: a}}}
	dir := t.TempDir()
	manifest := filepath.Join(dir, "genesis.json")
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(manifest, raw, 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"restore", "--genesis", manifest}, {"restore", "--genesis", manifest, "--backup", "missing"}} {
		if RunControl(context.Background(), args, io.Discard, io.Discard) == nil {
			t.Fatal("invalid restore accepted")
		}
	}
	store, err := ledger.Open(filepath.Join(dir, "data"), g)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	token := filepath.Join(dir, "token")
	if err = os.WriteFile(token, []byte(strings.Repeat("a", 32)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 32) {
			http.Error(w, "unauthorized", 401)
			return
		}
		if r.URL.Path == "/v1/status" {
			json.NewEncoder(w).Encode(store.Status())
			return
		}
		if err := store.Backup(w); err != nil {
			http.Error(w, "backup failed", 503)
		}
	}))
	defer server.Close()
	args := []string{"status", "--rpc", server.URL, "--token-file", token}
	var out bytes.Buffer
	if err = RunControl(context.Background(), args, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), string(g.ID())) {
		t.Fatal("status result lost")
	}
	if RunControl(context.Background(), args, failedOutput{}, io.Discard) == nil {
		t.Fatal("status output failure ignored")
	}
	backup := filepath.Join(dir, "backup")
	if err = RunControl(context.Background(), []string{"backup", "--rpc", server.URL, "--token-file", token, "--out", backup}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if RunControl(context.Background(), []string{"backup", "--rpc", server.URL, "--out", filepath.Join(dir, "unauthorized")}, io.Discard, io.Discard) == nil {
		t.Fatal("unauthorized backup accepted")
	}
	server.Close()
	if RunControl(context.Background(), args, io.Discard, io.Discard) == nil {
		t.Fatal("unavailable server accepted")
	}
}

func TestAdministrationExplicitInputsAndPublicationFailures(t *testing.T) {
	dir := t.TempDir()
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	token := filepath.Join(dir, "token")
	if err := os.WriteFile(token, []byte(strings.Repeat("t", 32)), 0600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "node.json")
	if err := RunControl(context.Background(), []string{"config", "--token-file", token, "--out", config}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := RunControl(context.Background(), []string{"config", "--token-file", token, "--out", config}, io.Discard, io.Discard); err == nil {
		t.Fatal("config overwritten")
	}
	manifest := filepath.Join(dir, "genesis.json")
	args := []string{"genesis", "--name", "explicit-representative", "--account", string(a), "--representative", string(a), "--balance", "100", "--out", manifest}
	if err := RunControl(context.Background(), args, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := RunControl(context.Background(), args, io.Discard, io.Discard); err == nil {
		t.Fatal("genesis overwritten")
	}
	if err := RunControl(context.Background(), []string{"token", "--out", token}, io.Discard, io.Discard); err == nil {
		t.Fatal("token overwritten")
	}
	if err := writeJSONNew(filepath.Join(dir, "unsupported"), make(chan int)); err == nil {
		t.Fatal("unencodable output published")
	}
}

func TestRestoreCommandRejectsUnverifiedArchive(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	g := protocol.Genesis{Version: 1, Name: "invalid-restore", Supply: 1, Allocations: []protocol.Allocation{{Account: a, Balance: 1, Representative: a}}}
	dir := t.TempDir()
	manifest := filepath.Join(dir, "genesis")
	backup := filepath.Join(dir, "backup")
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(manifest, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(backup, []byte("invalid archive"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = RunControl(context.Background(), []string{"restore", "--genesis", manifest, "--backup", backup, "--data-dir", filepath.Join(dir, "data")}, io.Discard, io.Discard); err == nil {
		t.Fatal("unverified archive restored")
	}
}
