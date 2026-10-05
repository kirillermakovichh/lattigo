// Package wallet owns local encrypted spending keys. It never sends private keys to a node.
package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	fileVersion = 1
	iterations  = 600000
	maxFileSize = 16 << 10
)

var ErrPassword = errors.New("wallet password or encrypted data is invalid")

// Wallet holds one deterministic seed. Call Close when finished with the wallet.
type Wallet struct {
	seed   [32]byte
	closed bool
}

type envelope struct {
	Version    int    `json:"version"`
	KDF        string `json:"kdf"`
	Iterations int    `json:"iterations"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// Create creates a new wallet and refuses to replace an existing path.
func Create(path string, password []byte) (*Wallet, error) {
	var seed [32]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return nil, fmt.Errorf("generate wallet seed: %w", err)
	}
	return Import(path, seed[:], password)
}

// Import writes an explicitly supplied 32-byte seed into encrypted custody.
func Import(path string, seed, password []byte) (*Wallet, error) {
	if len(seed) != 32 {
		return nil, errors.New("wallet seed must contain exactly 32 bytes")
	}
	if len(password) < 12 {
		return nil, errors.New("wallet passphrase must contain at least 12 bytes")
	}
	data, err := encrypt(seed, password)
	if err != nil {
		return nil, err
	}
	if err = writeExclusive(path, data); err != nil {
		return nil, err
	}
	w := &Wallet{}
	copy(w.seed[:], seed)
	return w, nil
}

// Open authenticates and decrypts a wallet. The file must not be group/world accessible.
func Open(path string, password []byte) (*Wallet, error) {
	data, err := readPrivate(path)
	if err != nil {
		return nil, err
	}
	seed, err := decrypt(data, password)
	if err != nil {
		return nil, err
	}
	defer clear(seed)
	w := &Wallet{}
	copy(w.seed[:], seed)
	return w, nil
}

// PrivateKey derives a deterministic account key without modifying the wallet.
func (w *Wallet) PrivateKey(index uint64) (ed25519.PrivateKey, error) {
	if w == nil || w.closed {
		return nil, errors.New("wallet is closed")
	}
	mac := hmac.New(sha256.New, w.seed[:])
	mac.Write([]byte("BL/wallet/v1"))
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], index)
	mac.Write(encoded[:])
	seed := mac.Sum(nil)
	defer clear(seed)
	return ed25519.NewKeyFromSeed(seed), nil
}

// Close overwrites the wallet seed. Go does not guarantee erasure of previous copies.
func (w *Wallet) Close() {
	if w != nil {
		clear(w.seed[:])
		w.closed = true
	}
}

// Backup verifies the password before copying encrypted bytes to an exclusive private file.
func Backup(source, destination string, password []byte) error {
	data, err := readPrivate(source)
	if err != nil {
		return err
	}
	seed, err := decrypt(data, password)
	if err != nil {
		return err
	}
	clear(seed)
	return writeExclusive(destination, data)
}

// Restore verifies a backup and restores it without overwriting an existing wallet.
func Restore(backup, destination string, password []byte) error {
	return Backup(backup, destination, password)
}

// ExportAccountSeed intentionally exports one signing identity to a private file.
// It does not expose the master seed or other accounts.
func (w *Wallet) ExportAccountSeed(path string, index uint64) error {
	key, err := w.PrivateKey(index)
	if err != nil {
		return err
	}
	defer clear(key)
	seed := key.Seed()
	defer clear(seed)
	return writeExclusive(path, []byte(hex.EncodeToString(seed)+"\n"))
}

// WritePrivate creates a durable private file without overwriting any existing path.
func WritePrivate(path string, data []byte) error { return writeExclusive(path, data) }

// ReadSecret reads bounded secret input from a private regular file.
func ReadSecret(path string) ([]byte, error) { return readPrivate(path) }

func encrypt(seed, password []byte) ([]byte, error) {
	salt := make([]byte, 16)
	nonce := make([]byte, 12)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	key, err := pbkdf2.Key(sha256.New, string(password), salt, iterations, 32)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ciphertext := aead.Seal(nil, nonce, seed, []byte("BL/wallet/envelope/v1"))
	return json.MarshalIndent(envelope{fileVersion, "pbkdf2-sha256", iterations, hex.EncodeToString(salt), hex.EncodeToString(nonce), hex.EncodeToString(ciphertext)}, "", "  ")
}

func decrypt(data, password []byte) ([]byte, error) {
	var e envelope
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&e); err != nil {
		return nil, errors.New("invalid wallet envelope")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, errors.New("wallet envelope contains trailing data")
	}
	if e.Version != fileVersion || e.KDF != "pbkdf2-sha256" || e.Iterations != iterations {
		return nil, errors.New("unsupported wallet format or KDF parameters")
	}
	salt, err := hex.DecodeString(e.Salt)
	if err != nil || len(salt) != 16 {
		return nil, errors.New("invalid wallet salt")
	}
	nonce, err := hex.DecodeString(e.Nonce)
	if err != nil || len(nonce) != 12 {
		return nil, errors.New("invalid wallet nonce")
	}
	ciphertext, err := hex.DecodeString(e.Ciphertext)
	if err != nil || len(ciphertext) != 48 {
		return nil, errors.New("invalid wallet ciphertext")
	}
	key, err := pbkdf2.Key(sha256.New, string(password), salt, iterations, 32)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	seed, err := aead.Open(nil, nonce, ciphertext, []byte("BL/wallet/envelope/v1"))
	if err != nil {
		return nil, ErrPassword
	}
	return seed, nil
}

func readPrivate(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open private file: %w", err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
		return nil, errors.New("secret input must be a regular file with permissions 0600 or stricter")
	}
	if st.Size() > maxFileSize {
		return nil, errors.New("private file exceeds size limit")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileSize {
		return nil, errors.New("private file exceeds size limit")
	}
	return data, nil
}

func writeExclusive(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create private directory: %w", err)
	}
	f, err := os.CreateTemp(dir, ".bl-private-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// Linking is an atomic create-if-absent. Rename would silently overwrite existing keys.
	if err = os.Link(temp, path); err != nil {
		return fmt.Errorf("create private file without overwrite: %w", err)
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	if err = d.Sync(); err != nil {
		return fmt.Errorf("sync private directory: %w", err)
	}
	return nil
}
