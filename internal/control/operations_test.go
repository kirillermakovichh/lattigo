package control

import (
	"bytes"
	"context"
	"example.com/go-block-lattice/internal/node"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorRedactsInvalidConfiguration(t *testing.T) {
	p := filepath.Join(t.TempDir(), "node.json")
	secret := "secret-token-never-echo-this-value"
	if err := os.WriteFile(p, []byte(`{"rpc_token":"`+secret+`","unknown":"private-key-path"}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := RunControl(context.Background(), []string{"doctor", "--config", p}, &out, &out)
	if err == nil {
		t.Fatal("invalid configuration accepted")
	}
	if strings.Contains(err.Error()+out.String(), secret) || strings.Contains(err.Error()+out.String(), "private-key-path") {
		t.Fatal("diagnostics exposed input")
	}
}

func TestLiveDoctorUsesBoundedExplicitProbes(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health/ready" {
			io.WriteString(w, `{"ready":true,"syncing":false,"quorum_observed":true,"consensus":{}}`)
		} else {
			io.WriteString(w, `{"height":12,"history_bytes":123,"history_limit_bytes":33554432}`)
		}
	}))
	defer rpc.Close()
	cfg := node.Config{RPCListen: strings.TrimPrefix(rpc.URL, "http://"), P2PListen: listener.Addr().String()}
	report, err := doctorLive(context.Background(), cfg)
	if err != nil || report["p2p_listener_reachable"] != true || report["height"] != uint64(12) {
		t.Fatal(report, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := doctorLive(ctx, cfg); err == nil {
		t.Fatal("cancelled probe succeeded")
	}
	if got := localProbeAddress("[::]:7070"); got != "[::1]:7070" {
		t.Fatal(got)
	}
}
