package control

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/go-block-lattice/internal/node"
	"example.com/go-block-lattice/internal/protocol"
)

type decisionFailureWriter struct{}

func (decisionFailureWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func decisionConfig(t *testing.T) (string, node.Config) {
	t.Helper()
	dir := t.TempDir()
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	g := protocol.Genesis{Version: 1, Name: "doctor-decision", Supply: 100, Allocations: []protocol.Allocation{{Account: a, Balance: 100, Representative: a}}}
	raw, _ := json.Marshal(g)
	genesis := filepath.Join(dir, "genesis.json")
	if err := os.WriteFile(genesis, raw, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := node.Config{Genesis: genesis, DataDir: filepath.Join(dir, "data"), RPCListen: "127.0.0.1:7070", P2PListen: "127.0.0.1:7071"}
	path := filepath.Join(dir, "node.json")
	raw, _ = json.Marshal(cfg)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path, cfg
}

func TestOperatorCommandsValidateAndPropagateOutput(t *testing.T) {
	ctx := context.Background()
	for _, cmd := range []string{"doctor", "profile", "ready"} {
		if err := RunControl(ctx, []string{cmd, "--help"}, io.Discard, io.Discard); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{cmd, "--unknown"}, {cmd, "position"}, {cmd, "--timeout=0"}} {
			if err := RunControl(ctx, args, io.Discard, io.Discard); err == nil {
				t.Fatal(args)
			}
		}
	}
	path, _ := decisionConfig(t)
	if err := RunControl(ctx, []string{"doctor", "--config", path}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := RunControl(ctx, []string{"doctor", "--config", path}, decisionFailureWriter{}, io.Discard); err == nil {
		t.Fatal("doctor ignored output failure")
	}
	if err := RunControl(ctx, []string{"doctor", "--config", path, "--live", "--timeout=50ms"}, io.Discard, io.Discard); err == nil {
		t.Fatal("absent live node succeeded")
	}
	if err := RunControl(ctx, []string{"doctor", "--config", path, "--live"}, decisionFailureWriter{}, io.Discard); err == nil {
		t.Fatal("live doctor ignored output failure")
	}
	raw := []byte(`{"genesis":"missing.json"}`)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := RunControl(ctx, []string{"doctor", "--config", path}, io.Discard, io.Discard); err == nil {
		t.Fatal("missing genesis accepted")
	}
	for _, args := range [][]string{{"ready", "--rpc", "invalid"}, {"profile", "--rpc", "invalid", "--out", filepath.Join(t.TempDir(), "p")}, {"profile", "--out", filepath.Join(t.TempDir(), "p"), "--token-file", filepath.Join(t.TempDir(), "missing")}} {
		if err := RunControl(ctx, args, io.Discard, io.Discard); err == nil {
			t.Fatal(args)
		}
	}
}

func TestOperatorRemoteReadsAndPrivateProfiles(t *testing.T) {
	ready := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health/ready":
			if !ready {
				w.WriteHeader(503)
			}
			json.NewEncoder(w).Encode(map[string]any{"ready": ready, "consensus": map[string]any{}, "syncing": false, "quorum_observed": true})
		case "/v1/admin/profile":
			io.WriteString(w, "profile-data")
		case "/v1/admin/peers":
			io.WriteString(w, "[]")
		default:
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	ctx := context.Background()
	for _, writer := range []io.Writer{io.Discard, decisionFailureWriter{}} {
		err := RunControl(ctx, []string{"ready", "--rpc", server.URL}, writer, io.Discard)
		if _, fail := writer.(decisionFailureWriter); fail != (err != nil) {
			t.Fatal(err)
		}
	}
	ready = false
	if err := RunControl(ctx, []string{"ready", "--rpc", server.URL}, io.Discard, io.Discard); err == nil {
		t.Fatal("unready returned success")
	}
	for _, url := range []string{server.URL, "http://127.0.0.1:1"} {
		err := RunControl(ctx, []string{"peers", "--rpc", url, "--timeout=50ms"}, io.Discard, io.Discard)
		if (url == server.URL) != (err == nil) {
			t.Fatal(err)
		}
	}
	for _, tokenFile := range []bool{false, true} {
		args := []string{"profile", "--rpc", server.URL, "--out", filepath.Join(t.TempDir(), "profile")}
		if tokenFile {
			token := filepath.Join(t.TempDir(), "token")
			os.WriteFile(token, []byte(strings.Repeat("a", 32)), 0600)
			args = append(args, "--token-file", token)
		}
		if err := RunControl(ctx, args, io.Discard, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	if err := RunControl(ctx, []string{"profile", "--rpc", server.URL, "--kind=unknown", "--out", filepath.Join(t.TempDir(), "profile")}, io.Discard, io.Discard); err == nil {
		t.Fatal("invalid profile succeeded")
	}
}

func TestLiveDoctorFailureBoundaries(t *testing.T) {
	if got := localProbeAddress("0.0.0.0:7070"); got != "127.0.0.1:7070" {
		t.Fatal(got)
	}
	if _, err := doctorLive(context.Background(), node.Config{RPCListen: "bad/host:80"}); err == nil {
		t.Fatal("invalid RPC succeeded")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	for _, mode := range []string{"status-error", "readiness-error", "unready", "p2p-error"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if mode == "status-error" || (mode == "readiness-error" && r.URL.Path == "/health/ready") {
				w.WriteHeader(500)
				return
			}
			if r.URL.Path == "/health/ready" {
				if mode == "unready" {
					w.WriteHeader(503)
				}
				json.NewEncoder(w).Encode(map[string]any{"ready": mode != "unready", "consensus": map[string]any{}, "syncing": false, "quorum_observed": true})
			} else {
				io.WriteString(w, `{"height":1}`)
			}
		}))
		p2p := listener.Addr().String()
		if mode == "p2p-error" {
			p2p = "127.0.0.1:1"
		}
		cfg := node.Config{RPCListen: strings.TrimPrefix(server.URL, "http://"), P2PListen: p2p}
		if _, err := doctorLive(context.Background(), cfg); err == nil {
			t.Fatal(mode)
		}
		server.Close()
	}
	var out bytes.Buffer
	if err := RunControl(context.Background(), []string{"ready", "--rpc", "http://127.0.0.1:1", "--timeout=50ms"}, &out, io.Discard); err == nil {
		t.Fatal("offline readiness succeeded")
	}
}
