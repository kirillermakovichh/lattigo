package node

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateFilesDoesNotCreateRuntimeState(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{Genesis: filepath.Join(dir, "missing.json"), DataDir: filepath.Join(dir, "data")}
	cfg.defaults()
	if err := cfg.ValidateFiles(); err == nil {
		t.Fatal("missing genesis accepted")
	}
	if _, err := os.Stat(cfg.DataDir); !os.IsNotExist(err) {
		t.Fatal("validation created data directory")
	}
}
