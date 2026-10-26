package wallet

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

var ErrAccountBusy = errors.New("another payment operation for this wallet account is in progress")

// lockAccount serializes the entire read/check/remove/create outbox sequence.
// Atomic creation alone cannot protect removal of an older confirmed outbox.
func lockAccount(path string, index uint64) (*os.File, error) {
	return lockPrivate(fmt.Sprintf("%s.account-%d.lock", path, index))
}

func lockPrivate(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		file.Close()
		return nil, errors.New("wallet lock must be a private regular file")
	}
	if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrAccountBusy
		}
		return nil, err
	}
	return file, nil
}
