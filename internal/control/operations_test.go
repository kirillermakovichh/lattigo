package control

import (
	"bytes"
	"context"
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
