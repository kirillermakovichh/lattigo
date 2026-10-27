//go:build faultaudit

package wallet

import (
	"errors"
	"path/filepath"
	"testing"

	"example.com/go-block-lattice/internal/faultprobe"
)

func TestPasswordReplacementFailurePreservesReadableCustody(t *testing.T) {
	old := []byte("old fault passphrase")
	next := []byte("new fault passphrase")
	for _, site := range []string{"journal.go/AtomicWrite/os.Rename/1", "journal.go/SyncDirectory/os.Open/1"} {
		t.Run(site, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "wallet")
			w, err := Import(path, make([]byte, 32), old)
			if err != nil {
				t.Fatal(err)
			}
			w.Close()
			faultprobe.Arm(site, 1)
			defer faultprobe.Arm("", 0)
			err = ChangePassword(path, old, next)
			if !faultprobe.Reached() || !errors.Is(err, faultprobe.Injected) {
				t.Fatalf("replacement fault: %v", err)
			}
			password := old
			if site == "journal.go/SyncDirectory/os.Open/1" {
				password = next
			}
			restored, err := Open(path, password)
			if err != nil {
				t.Fatal("replacement lost readable custody")
			}
			restored.Close()
		})
	}
}
