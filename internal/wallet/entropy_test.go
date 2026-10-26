package wallet

import (
	"crypto/rand"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type failedEntropy struct{ remaining int }

func (r *failedEntropy) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, errors.New("entropy unavailable")
	}
	n := min(len(p), r.remaining)
	clear(p[:n])
	r.remaining -= n
	return n, nil
}

func TestEntropyFailureNeverPublishesWallet(t *testing.T) {
	if mode := os.Getenv("BL_TEST_ENTROPY_CHILD"); mode != "" {
		reader := &failedEntropy{}
		if mode == "nonce" {
			reader.remaining = 16
		}
		rand.Reader = reader
		path := os.Getenv("BL_TEST_ENTROPY_PATH")
		if mode == "seed" {
			Create(path, []byte("private entropy passphrase"))
		} else {
			Import(path, make([]byte, 32), []byte("private entropy passphrase"))
		}
		os.Exit(0)
	}
	for _, mode := range []string{"seed", "salt", "nonce"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "wallet")
			command := exec.Command(os.Args[0], "-test.run=^TestEntropyFailureNeverPublishesWallet$")
			command.Env = append(os.Environ(), "BL_TEST_ENTROPY_CHILD="+mode, "BL_TEST_ENTROPY_PATH="+path)
			output, err := command.CombinedOutput()
			// Go 1.26 crypto/rand.Read terminates the process on entropy failure; its
			// returned error is always nil. The defensive caller errors are unreachable.
			if err == nil || !strings.Contains(string(output), "crypto/rand") {
				t.Fatalf("entropy failure did not stop process: %v %s", err, output)
			}
			if _, err = os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("entropy failure published a wallet")
			}
		})
	}
}
