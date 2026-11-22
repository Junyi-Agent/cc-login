// Package seal encrypts the file of cc-login export with a passphrase, and
// decrypts it for import (ADR 0021): PBKDF2-SHA256 makes the key, AES-256-GCM
// encrypts. It knows bytes and a passphrase, not logins.
package seal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
)

// Iterations is the PBKDF2 work of a new file. Tests lower it; a file says
// its own, so any one opens.
var Iterations = 600_000

// The iterations Open accepts: a file cannot make import compute for long.
const (
	minIterations = 100_000
	maxIterations = 10_000_000
)

const (
	format = 1
	kdf    = "pbkdf2-sha256"
)

// ErrOpen means the passphrase is wrong, or the file was changed.
var ErrOpen = errors.New("wrong passphrase, or the file was changed")

// envelope is the file: what decryption needs, and the encrypted data.
type envelope struct {
	Format     int    `json:"cc_login_export"`
	KDF        string `json:"kdf"`
	Iterations int    `json:"iterations"`
	Salt       []byte `json:"salt"`  // base64 in JSON
	Nonce      []byte `json:"nonce"` // base64 in JSON
	Data       []byte `json:"data"`  // base64 in JSON
}

// Seal encrypts plain with passphrase into the content of an export file.
func Seal(passphrase string, plain []byte) ([]byte, error) {
	e := envelope{Format: format, KDF: kdf, Iterations: Iterations, Salt: make([]byte, 16)}
	if _, err := rand.Read(e.Salt); err != nil {
		return nil, err
	}

	aead, err := e.aead(passphrase)
	if err != nil {
		return nil, err
	}

	e.Nonce = make([]byte, aead.NonceSize())
	if _, err = rand.Read(e.Nonce); err != nil {
		return nil, err
	}
	e.Data = aead.Seal(nil, e.Nonce, plain, e.additional())

	b, err := json.MarshalIndent(e, "", "  ")
	return append(b, '\n'), err
}

// Open decrypts the content of an export file with passphrase. A wrong
// passphrase and a changed file are both ErrOpen.
func Open(passphrase string, sealed []byte) ([]byte, error) {
	var e envelope
	if err := json.Unmarshal(sealed, &e); err != nil || e.Format == 0 {
		return nil, errors.New("not a file of cc-login export")
	}

	if err := e.check(); err != nil {
		return nil, err
	}

	aead, err := e.aead(passphrase)
	if err != nil {
		return nil, err
	}

	if len(e.Nonce) != aead.NonceSize() {
		return nil, ErrOpen
	}
	plain, err := aead.Open(nil, e.Nonce, e.Data, e.additional())
	if err != nil {
		return nil, ErrOpen
	}
	return plain, nil
}

// check refuses a file this code cannot or will not open.
func (e *envelope) check() error {
	if e.Format != format || e.KDF != kdf {
		return fmt.Errorf("an export file of format %d, %s; this cc-login reads format %d, %s", e.Format, e.KDF, format, kdf)
	}
	if e.Iterations < minIterations || e.Iterations > maxIterations {
		return fmt.Errorf("an export file with %d PBKDF2 iterations; cc-login accepts %d to %d", e.Iterations, minIterations, maxIterations)
	}
	if len(e.Salt) < 16 {
		return ErrOpen
	}
	return nil
}

// aead is AES-256-GCM with the key PBKDF2 makes from passphrase.
func (e *envelope) aead(passphrase string) (cipher.AEAD, error) {
	key, err := pbkdf2.Key(sha256.New, passphrase, e.Salt, e.Iterations, 32)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// additional binds the parameters to the data: changing one makes Open fail.
func (e *envelope) additional() []byte {
	return fmt.Appendf(nil, "cc-login-export %d %s %d", e.Format, e.KDF, e.Iterations)
}
