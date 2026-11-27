// Package crypto provides envelope encryption for secrets stored at rest, such
// as per-project GitLab tokens. A fresh random data-encryption key (DEK)
// encrypts each value; the DEK is itself encrypted ("wrapped") by a long-lived
// master key-encryption key (KEK) supplied at startup from the secret provider.
// Only wrapped data is persisted, so a database dump never exposes a usable key
// or any plaintext, and rotating the KEK re-wraps DEKs without re-encrypting
// every value. Both layers use AES-256-GCM, which is authenticated: tampering
// with stored ciphertext makes Open fail rather than return corrupt plaintext.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	keySize   = 32 // AES-256
	nonceSize = 12 // GCM standard nonce length
)

// Sentinel errors.
var (
	// ErrInvalidKey is returned when the master key is not 32 bytes.
	ErrInvalidKey = errors.New("crypto: master key must be 32 bytes")
	// ErrMalformedCiphertext is returned when a blob is too short or structurally
	// invalid to be a sealed value.
	ErrMalformedCiphertext = errors.New("crypto: malformed ciphertext")
)

// Sealer seals and opens secrets with envelope encryption under a master key.
// It is safe for concurrent use: the wrapped AEAD is stateless across calls and
// every Seal generates its own DEK and nonces.
type Sealer struct {
	kek cipher.AEAD
}

// NewSealer builds a Sealer from a 32-byte master key.
func NewSealer(masterKey []byte) (*Sealer, error) {
	aead, err := newGCM(masterKey)
	if err != nil {
		return nil, err
	}
	return &Sealer{kek: aead}, nil
}

// NewSealerFromBase64 decodes a base64-encoded 32-byte master key (standard or
// URL alphabet, with or without padding) and builds a Sealer. This is the form
// the key takes when passed through the secret provider.
func NewSealerFromBase64(encoded string) (*Sealer, error) {
	key, err := decodeKey(encoded)
	if err != nil {
		return nil, err
	}
	return NewSealer(key)
}

// Seal envelope-encrypts plaintext: a fresh random DEK encrypts the value, and
// the KEK wraps the DEK. The returned blob carries both and is safe to store.
func (s *Sealer) Seal(plaintext []byte) ([]byte, error) {
	dek := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("crypto: generate dek: %w", err)
	}
	dekAEAD, err := newGCM(dek)
	if err != nil {
		return nil, err
	}

	wrapped, err := sealWith(s.kek, dek)
	if err != nil {
		return nil, fmt.Errorf("crypto: wrap dek: %w", err)
	}
	body, err := sealWith(dekAEAD, plaintext)
	if err != nil {
		return nil, fmt.Errorf("crypto: encrypt value: %w", err)
	}

	// Layout: [uint16 big-endian len(wrapped)] [wrapped DEK] [encrypted value].
	out := make([]byte, 2+len(wrapped)+len(body))
	binary.BigEndian.PutUint16(out[:2], uint16(len(wrapped)))
	copy(out[2:], wrapped)
	copy(out[2+len(wrapped):], body)
	return out, nil
}

// Open reverses Seal, returning the plaintext. It fails if the blob is
// malformed, was sealed under a different key, or has been tampered with.
func (s *Sealer) Open(blob []byte) ([]byte, error) {
	if len(blob) < 2 {
		return nil, ErrMalformedCiphertext
	}
	wrappedLen := int(binary.BigEndian.Uint16(blob[:2]))
	if len(blob) < 2+wrappedLen {
		return nil, ErrMalformedCiphertext
	}
	wrapped := blob[2 : 2+wrappedLen]
	body := blob[2+wrappedLen:]

	dek, err := openWith(s.kek, wrapped)
	if err != nil {
		return nil, fmt.Errorf("crypto: unwrap dek: %w", err)
	}
	dekAEAD, err := newGCM(dek)
	if err != nil {
		return nil, err
	}
	plaintext, err := openWith(dekAEAD, body)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt value: %w", err)
	}
	return plaintext, nil
}

// sealWith prepends a fresh random nonce to the AEAD ciphertext it produces.
func sealWith(aead cipher.AEAD, plaintext []byte) ([]byte, error) {
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, plaintext, nil), nil
}

// openWith splits the nonce from the ciphertext and authenticates-and-decrypts.
func openWith(aead cipher.AEAD, blob []byte) ([]byte, error) {
	if len(blob) < nonceSize {
		return nil, ErrMalformedCiphertext
	}
	nonce, ciphertext := blob[:nonceSize], blob[nonceSize:]
	return aead.Open(nil, nonce, ciphertext, nil)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != keySize {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

// decodeKey accepts a base64 master key in any common alphabet/padding and
// requires it to decode to exactly 32 bytes.
func decodeKey(encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	} {
		if key, err := enc.DecodeString(encoded); err == nil && len(key) == keySize {
			return key, nil
		}
	}
	return nil, ErrInvalidKey
}
