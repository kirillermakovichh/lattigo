// Package storage implements a checksummed, fsynced, exclusively locked journal.
package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

const MaxFrame = 16 << 20
const magic = "BLWAL001"

var marker = []byte("BLCOMMIT")

type Journal struct {
	directory string
	file      *os.File
	lock      *os.File
	failed    error
}

func Open(directory string, header []byte, apply func([]byte) error) (*Journal, error) {
	return open(directory, header, apply, true)
}
func OpenStrict(directory string, header []byte, apply func([]byte) error) (*Journal, error) {
	return open(directory, header, apply, false)
}
func open(directory string, header []byte, apply func([]byte) error, repair bool) (j *Journal, err error) {
	if e := os.MkdirAll(directory, 0700); e != nil {
		return nil, e
	}
	info, e := os.Lstat(directory)
	if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("storage directory must be a real directory")
	}
	_, lockStatErr := os.Lstat(filepath.Join(directory, "LOCK"))
	lockExisted := lockStatErr == nil
	lock, e := os.OpenFile(filepath.Join(directory, "LOCK"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		lock.Close()
		return nil, fmt.Errorf("data directory already locked: %w", e)
	}
	j = &Journal{directory: directory, lock: lock}
	opened := j
	defer func() {
		if err != nil {
			opened.Close()
		}
	}()
	created := true
	j.file, e = os.OpenFile(filepath.Join(directory, "ledger.wal"), os.O_CREATE|os.O_EXCL|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if errors.Is(e, os.ErrExist) {
		created = false
		j.file, e = os.OpenFile(filepath.Join(directory, "ledger.wal"), os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	}
	if e != nil {
		return nil, e
	}
	info, e = j.file.Stat()
	if e != nil {
		return nil, e
	}
	if info.Size() == 0 {
		if created && lockExisted {
			return nil, errors.New("WAL is missing from an existing data directory")
		}
		if !created {
			return nil, errors.New("existing WAL is empty or truncated")
		}
		if _, e = j.file.Write([]byte(magic)); e != nil {
			return nil, e
		}
		if e = j.Append(header); e != nil {
			return nil, e
		}
		if e = SyncDirectory(directory); e != nil {
			return nil, e
		}
		return j, nil
	}
	buf := make([]byte, len(magic))
	if _, e = io.ReadFull(j.file, buf); e != nil || string(buf) != magic {
		return nil, errors.New("invalid WAL schema or magic")
	}
	first := true
	for {
		start, e := j.file.Seek(0, io.SeekCurrent)
		if e != nil {
			return nil, e
		}
		payload, e := readFrame(j.file)
		if e == io.EOF {
			break
		}
		if errors.Is(e, io.ErrUnexpectedEOF) && repair && !first {
			if e = j.file.Truncate(start); e != nil {
				return nil, e
			}
			if e = j.file.Sync(); e != nil {
				return nil, e
			}
			break
		}
		if e != nil {
			return nil, fmt.Errorf("WAL at offset %d: %w", start, e)
		}
		if first {
			if !bytes.Equal(payload, header) {
				return nil, errors.New("WAL genesis/network mismatch")
			}
			first = false
			continue
		}
		if e = apply(payload); e != nil {
			return nil, fmt.Errorf("invalid committed record at offset %d: %w", start, e)
		}
	}
	if first {
		return nil, errors.New("WAL missing genesis")
	}
	_, err = j.file.Seek(0, io.SeekEnd)
	return j, err
}
func frame(payload []byte) ([]byte, error) {
	if len(payload) == 0 || len(payload) > MaxFrame {
		return nil, errors.New("journal frame size out of range")
	}
	b := make([]byte, 0, len(payload)+48)
	b = binary.BigEndian.AppendUint32(b, uint32(len(payload)))
	b = binary.BigEndian.AppendUint32(b, ^uint32(len(payload)))
	h := sha256.Sum256(payload)
	b = append(b, h[:]...)
	b = append(b, payload...)
	b = append(b, marker...)
	return b, nil
}
func readFrame(r io.Reader) ([]byte, error) {
	header := make([]byte, 40)
	n, e := io.ReadFull(r, header)
	if e == io.EOF && n == 0 {
		return nil, io.EOF
	}
	if e != nil {
		return nil, io.ErrUnexpectedEOF
	}
	size := binary.BigEndian.Uint32(header)
	if binary.BigEndian.Uint32(header[4:]) != ^size || size == 0 || size > MaxFrame {
		return nil, errors.New("corrupt journal frame length")
	}
	body := make([]byte, int(size)+len(marker))
	if _, e = io.ReadFull(r, body); e != nil {
		return nil, io.ErrUnexpectedEOF
	}
	if !bytes.Equal(body[size:], marker) {
		return nil, errors.New("invalid journal commit marker")
	}
	h := sha256.Sum256(body[:size])
	if !bytes.Equal(h[:], header[8:]) {
		return nil, errors.New("journal checksum mismatch")
	}
	return body[:size], nil
}
func (j *Journal) Append(payload []byte) error {
	if j.failed != nil {
		return j.failed
	}
	if j.file == nil {
		return errors.New("journal closed")
	}
	b, e := frame(payload)
	if e != nil {
		return e
	}
	if _, e = j.file.Write(b); e == nil {
		e = j.file.Sync()
	}
	if e != nil {
		j.failed = fmt.Errorf("journal durability failure: %w", e)
		return j.failed
	}
	return nil
}
func (j *Journal) Backup(w io.Writer) error {
	if j.file == nil {
		return errors.New("journal closed")
	}
	if j.failed != nil {
		return j.failed
	}
	info, e := j.file.Stat()
	if e != nil {
		return e
	}
	_, e = io.Copy(w, io.NewSectionReader(j.file, 0, info.Size()))
	return e
}
func (j *Journal) Signing() ([]byte, error) {
	f, err := os.OpenFile(filepath.Join(j.directory, "signing.json"), os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	var b []byte
	if err == nil {
		info, statErr := f.Stat()
		if statErr != nil {
			f.Close()
			return nil, statErr
		}
		if !info.Mode().IsRegular() || info.Size() > MaxFrame+40 {
			f.Close()
			return nil, errors.New("invalid signing state file size or type")
		}
		b, err = io.ReadAll(io.LimitReader(f, MaxFrame+41))
		err = errors.Join(err, f.Close())
	}
	if errors.Is(err, os.ErrNotExist) {
		if _, markerErr := os.Stat(filepath.Join(j.directory, "SIGNER")); markerErr == nil {
			return nil, errors.New("signing state lost after signer initialization; rollback recovery refused")
		} else if !errors.Is(markerErr, os.ErrNotExist) {
			return nil, markerErr
		}
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	if len(b) < 40 || len(b) > MaxFrame+40 || string(b[:8]) != "BLSIGN01" {
		return nil, errors.New("invalid signing state envelope")
	}
	checksum := sha256.Sum256(b[40:])
	if !bytes.Equal(checksum[:], b[8:40]) {
		return nil, errors.New("signing state checksum mismatch")
	}
	return b[40:], nil
}
func (j *Journal) SaveSigning(b []byte) error {
	if j.failed != nil {
		return j.failed
	}
	if len(b) == 0 || len(b) > MaxFrame {
		return errors.New("invalid signing state size")
	}
	// A permanent initialization marker makes later disappearance fail closed.
	if _, e := os.Stat(filepath.Join(j.directory, "SIGNER")); errors.Is(e, os.ErrNotExist) {
		if e = AtomicWrite(j.directory, "SIGNER", []byte("BL/signer/v1\n")); e != nil {
			j.failed = fmt.Errorf("signer marker durability failure: %w", e)
			return j.failed
		}
	} else if e != nil {
		j.failed = e
		return e
	}
	checksum := sha256.Sum256(b)
	envelope := append([]byte("BLSIGN01"), checksum[:]...)
	envelope = append(envelope, b...)
	if e := AtomicWrite(j.directory, "signing.json", envelope); e != nil {
		j.failed = fmt.Errorf("signing durability failure: %w", e)
	}
	return j.failed
}
func (j *Journal) Close() error {
	var errs []error
	if j.file != nil {
		errs = append(errs, j.file.Close())
		j.file = nil
	}
	if j.lock != nil {
		errs = append(errs, syscall.Flock(int(j.lock.Fd()), syscall.LOCK_UN))
		errs = append(errs, j.lock.Close())
		j.lock = nil
	}
	return errors.Join(errs...)
}
func SyncDirectory(directory string) error {
	f, e := os.Open(directory)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func AtomicWrite(directory, name string, b []byte) error {
	f, e := os.CreateTemp(directory, ".atomic-*")
	if e != nil {
		return e
	}
	path := f.Name()
	defer os.Remove(path)
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	e = errors.Join(e, f.Close())
	if e != nil {
		return e
	}
	if e = os.Rename(path, filepath.Join(directory, name)); e != nil {
		return e
	}
	return SyncDirectory(directory)
}
