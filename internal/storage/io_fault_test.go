//go:build faultaudit

package storage

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJournalIOFailures(t *testing.T) {
	for _, point := range _auditPoints {
		switch point.Function {
		case "open", "Append", "Backup", "Signing", "SaveSigning", "AtomicWrite", "SyncDirectory", "Close":
		default:
			continue
		}
		t.Run(point.ID, func(t *testing.T) {
			dir := t.TempDir()
			header := []byte("genesis")
			var j *Journal
			var err error
			if point.Function != "open" && point.Function != "AtomicWrite" && point.Function != "SyncDirectory" {
				j, err = Open(dir, header, func([]byte) error { return nil })
				if err != nil {
					t.Fatal(err)
				}
				defer j.Close()
			}
			if point.Function == "Signing" && !strings.Contains(point.ID, "os.Stat/") {
				if err = j.SaveSigning([]byte("intent")); err != nil {
					t.Fatal(err)
				}
			}
			if point.Function == "open" && (strings.Contains(point.ID, "os.OpenFile/3") || strings.Contains(point.ID, "j.file.Seek") || strings.Contains(point.ID, "j.file.Truncate") || strings.Contains(point.ID, "j.file.Sync")) {
				initial, err := Open(dir, header, func([]byte) error { return nil })
				if err != nil {
					t.Fatal(err)
				}
				if err = initial.Close(); err != nil {
					t.Fatal(err)
				}
				file, err := os.OpenFile(filepath.Join(dir, "ledger.wal"), os.O_WRONLY|os.O_APPEND, 0600)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = file.Write([]byte{1}); err != nil {
					t.Fatal(err)
				}
				if err = file.Close(); err != nil {
					t.Fatal(err)
				}
			}
			_auditArm(point.ID, 1)
			defer _auditArm("", 0)
			switch point.Function {
			case "open":
				var opened *Journal
				opened, err = Open(dir, header, func([]byte) error { return nil })
				if opened != nil {
					opened.Close()
				}
			case "Append":
				err = j.Append([]byte("record"))
			case "Backup":
				err = j.Backup(io.Discard)
			case "Signing":
				_, err = j.Signing()
			case "SaveSigning":
				err = j.SaveSigning([]byte("intent"))
			case "AtomicWrite":
				err = AtomicWrite(dir, "output", []byte("payload"))
			case "SyncDirectory":
				err = SyncDirectory(dir)
			case "Close":
				err = j.Close()
			}
			if !_auditReached() {
				t.Fatal("fault site not reached")
			}
			if err == nil || (!errors.Is(err, _auditInjected) && point.ID != "journal.go/open/os.Lstat/1") {
				t.Fatalf("journal I/O error lost: %v", err)
			}
			if point.Function == "Append" && j.Append([]byte("retry")) == nil {
				t.Fatal("failed journal resumed append")
			}
			if point.Function == "SaveSigning" && j.SaveSigning([]byte("retry")) == nil {
				t.Fatal("failed signer resumed publication")
			}
		})
	}
}

func TestInitialJournalDependencyFailures(t *testing.T) {
	for _, point := range _auditPoints {
		if point.Function != "Append" && point.Function != "SyncDirectory" {
			continue
		}
		t.Run(point.ID, func(t *testing.T) {
			dir := t.TempDir()
			_auditArm(point.ID, 1)
			defer _auditArm("", 0)
			j, err := Open(dir, []byte("genesis"), func([]byte) error { return nil })
			if j != nil {
				j.Close()
			}
			if !_auditReached() || !errors.Is(err, _auditInjected) {
				t.Fatalf("initial journal error lost: %v", err)
			}
		})
	}
}
