package wallet

import (
	"errors"
	"fmt"
	"path/filepath"

	"example.com/go-block-lattice/internal/storage"
)

// ChangePassword authenticates the old envelope and atomically replaces its
// encryption. Derived accounts are unchanged; existing backups keep their passwords.
func ChangePassword(path string, oldPassword, newPassword []byte) error {
	if len(newPassword) < 12 {
		return errors.New("wallet passphrase must contain at least 12 bytes")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	lock, err := lockPrivate(resolved + ".custody.lock")
	if err != nil {
		return fmt.Errorf("wallet custody lock: %w", err)
	}
	defer lock.Close()
	w, err := Open(resolved, oldPassword)
	if err != nil {
		return err
	}
	defer w.Close()
	data, err := encrypt(w.seed[:], newPassword)
	if err != nil {
		return err
	}
	return storage.AtomicWrite(filepath.Dir(resolved), filepath.Base(resolved), data)
}
