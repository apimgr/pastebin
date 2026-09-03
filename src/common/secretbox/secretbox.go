// Package secretbox provides authenticated at-rest encryption using
// AES-256-GCM keyed directly by the server's 32-byte encryption_key
// (PART 11 → "Cryptographic Keys"). Unlike the backup encryptor, no Argon2id
// derivation is applied: the key is already high-entropy random material, so
// the raw 32 bytes are used as the AES key. The nonce is random per message
// and prepended to the ciphertext.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"errors"
	"fmt"
	"io"
)

// ErrKeySize is returned when the supplied key is not exactly 32 bytes.
var ErrKeySize = errors.New("secretbox: key must be 32 bytes (AES-256)")

// ErrShortCiphertext is returned when a ciphertext is too short to contain a
// nonce and an authentication tag.
var ErrShortCiphertext = errors.New("secretbox: ciphertext too short")

// Seal encrypts plaintext with AES-256-GCM under key and returns
// nonce || ciphertext || tag. key must be exactly 32 bytes.
func Seal(key, plaintext []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(crand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("secretbox: nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Open decrypts data produced by Seal. key must be exactly 32 bytes.
func Open(key, data []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return nil, ErrShortCiphertext
	}
	nonce, ciphertext := data[:ns], data[ns:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("secretbox: open: %w", err)
	}
	return plaintext, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, ErrKeySize
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: gcm: %w", err)
	}
	return gcm, nil
}
