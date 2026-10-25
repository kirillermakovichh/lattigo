//go:build faultaudit

package ledger

import (
	"bytes"
	"example.com/go-block-lattice/internal/faultprobe"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreRejectsRemovedOrReplacedDirectoryLock(t *testing.T) {
	state, _, _ := fixture(t)
	s, err := Open(t.TempDir(), state.genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var archive bytes.Buffer
	if err = s.Backup(&archive); err != nil {
		t.Fatal(err)
	}
	for _, replace := range []bool{false, true} {
		t.Run(fmt.Sprint(replace), func(t *testing.T) {
			target := t.TempDir()
			faultprobe.Before("store.go/RestoreWithOptions/os.ReadDir/2", 1, func() {
				path := filepath.Join(target, "LOCK")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if replace {
					if err := os.WriteFile(path, nil, 0600); err != nil {
						t.Fatal(err)
					}
				}
			})
			defer faultprobe.Arm("", 0)
			err := Restore(target, bytes.NewReader(archive.Bytes()), state.genesis)
			if !faultprobe.Reached() || err == nil {
				t.Fatalf("restore accepted a removed or replaced lock: %v", err)
			}
			if _, err = os.Stat(filepath.Join(target, "ledger.wal")); !os.IsNotExist(err) {
				t.Fatal("restore published data without the acquired directory lock")
			}
		})
	}
}
