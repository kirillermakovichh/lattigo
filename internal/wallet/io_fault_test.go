//go:build faultaudit

package wallet

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateFileIOFailures(t *testing.T) {
	for _, point := range _auditPoints {
		if point.Function != "writeExclusive" && point.Function != "readPrivate" && point.Function != "lockAccount" {
			continue
		}
		t.Run(point.ID, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "private")
			if err := os.WriteFile(path, []byte("private"), 0600); err != nil {
				t.Fatal(err)
			}
			_auditArm(point.ID, 1)
			defer _auditArm("", 0)
			var err error
			switch point.Function {
			case "writeExclusive":
				err = WritePrivate(filepath.Join(dir, "new"), []byte("payload"))
			case "readPrivate":
				_, err = ReadSecret(path)
			case "lockAccount":
				_, err = lockAccount(path, 0)
			}
			if !_auditReached() {
				t.Fatal("fault site not reached")
			}
			if !errors.Is(err, _auditInjected) {
				t.Fatalf("I/O error not propagated: %v", err)
			}
			leftovers, _ := filepath.Glob(filepath.Join(dir, ".bl-private-*"))
			if len(leftovers) != 0 {
				t.Fatal("temporary custody files leaked")
			}
		})
	}
	w := &Wallet{}
	w.Close()
	if _, err := keyAccount(w, 0); err == nil {
		t.Fatal("closed identity derived")
	}
	if err := w.ExportAccountSeed(filepath.Join(t.TempDir(), "seed"), 0); err == nil {
		t.Fatal("closed identity exported")
	}
}
