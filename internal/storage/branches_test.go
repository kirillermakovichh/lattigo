package storage

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type rejectWriter struct{}

func (rejectWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestJournalFramingRejectionBranches(t *testing.T) {
	valid, err := frame([]byte("record"))
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func([]byte) []byte{
		"empty": func(b []byte) []byte { return nil }, "partial header": func(b []byte) []byte { return b[:3] }, "partial body": func(b []byte) []byte { return b[:41] }, "length": func(b []byte) []byte { b[0] ^= 1; return b }, "marker": func(b []byte) []byte { b[len(b)-1] ^= 1; return b }, "checksum": func(b []byte) []byte { b[8] ^= 1; return b },
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readFrame(bytes.NewReader(mutate(append([]byte{}, valid...)))); err == nil {
				t.Fatal("invalid frame accepted")
			}
		})
	}
	for _, payload := range [][]byte{nil, make([]byte, MaxFrame+1)} {
		if _, err := frame(payload); err == nil {
			t.Fatal("invalid frame size accepted")
		}
	}
}
func TestJournalOpenAndAtomicWriteBranches(t *testing.T) {
	header := []byte("genesis")
	for _, name := range []string{"bad magic", "missing header", "empty existing", "missing existing", "bad genesis", "application rejects", "partial strict", "symlink lock", "symlink wal", "non-directory"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			wal := filepath.Join(dir, "ledger.wal")
			apply := func([]byte) error { return nil }
			switch name {
			case "bad magic":
				writeFixture(t, wal, []byte("BADWAL01"), 0600)
			case "missing header":
				writeFixture(t, wal, []byte(magic), 0600)
			case "empty existing":
				writeFixture(t, wal, nil, 0600)
			case "missing existing":
				writeFixture(t, filepath.Join(dir, "LOCK"), nil, 0600)
			case "bad genesis":
				b, _ := frame([]byte("other"))
				writeFixture(t, wal, append([]byte(magic), b...), 0600)
			case "application rejects":
				a, _ := frame(header)
				b, _ := frame([]byte("bad record"))
				writeFixture(t, wal, append(append([]byte(magic), a...), b...), 0600)
				apply = func([]byte) error { return errors.New("invalid record") }
			case "partial strict":
				a, _ := frame(header)
				writeFixture(t, wal, append(append([]byte(magic), a...), 1), 0600)
			case "symlink lock", "symlink wal":
				target := filepath.Join(t.TempDir(), "target")
				writeFixture(t, target, nil, 0600)
				path := filepath.Join(dir, "LOCK")
				if name == "symlink wal" {
					path = wal
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "non-directory":
				dir = filepath.Join(dir, "file")
				writeFixture(t, dir, []byte("x"), 0600)
			}
			j, err := OpenStrict(dir, header, apply)
			if err == nil {
				j.Close()
				t.Fatal("invalid journal accepted")
			}
		})
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if AtomicWrite(dir, "target", []byte("data")) == nil {
		t.Fatal("directory replaced")
	}
	if AtomicWrite(filepath.Join(dir, "missing"), "file", nil) == nil {
		t.Fatal("missing directory accepted")
	}
	if SyncDirectory(filepath.Join(dir, "missing")) == nil {
		t.Fatal("missing directory synced")
	}
	j, err := Open(t.TempDir(), header, func([]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if j.Backup(rejectWriter{}) == nil {
		t.Fatal("backup writer failure ignored")
	}
	if j.Append(nil) == nil {
		t.Fatal("empty record appended")
	}
	if j.SaveSigning(nil) == nil {
		t.Fatal("empty signing state accepted")
	}
	if _, err = j.Signing(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new signer:%v", err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	if j.Append([]byte("x")) == nil || j.Backup(io.Discard) == nil {
		t.Fatal("closed journal accepted work")
	}
}
func TestSigningEnvelopeAndFilesystemRejectionBranches(t *testing.T) {
	for _, name := range []string{"directory", "oversized", "magic", "checksum", "missing after marker", "bad marker"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			j, err := Open(dir, []byte("genesis"), func([]byte) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			defer j.Close()
			path := filepath.Join(dir, "signing.json")
			switch name {
			case "directory":
				os.Mkdir(path, 0700)
			case "oversized":
				f, err := os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				if err = f.Truncate(MaxSigningState + 41); err != nil {
					t.Fatal(err)
				}
				f.Close()
			case "magic":
				writeFixture(t, path, make([]byte, 40), 0600)
			case "checksum":
				writeFixture(t, path, append([]byte("BLSIGN01"), make([]byte, 32)...), 0600)
			case "missing after marker":
				writeFixture(t, filepath.Join(dir, "SIGNER"), []byte("init"), 0600)
			case "bad marker":
				os.Symlink("SIGNER", filepath.Join(dir, "SIGNER"))
			}
			if _, err = j.Signing(); err == nil {
				t.Fatal("invalid signing state accepted")
			}
		})
	}
	dir := t.TempDir()
	j, err := Open(dir, []byte("g"), func([]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err = os.Mkdir(filepath.Join(dir, "signing.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if j.SaveSigning([]byte("intent")) == nil {
		t.Fatal("signing publication failure ignored")
	}
	if j.Append([]byte("record")) == nil {
		t.Fatal("journal continued after signing failure")
	}
}

func writeFixture(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}
