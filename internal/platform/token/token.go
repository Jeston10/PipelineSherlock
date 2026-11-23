// Package token generates high-entropy secret tokens and verifies them against
// a stored hash. Only the hash is ever persisted; the plaintext token is shown
// to the caller once at creation and never stored or logged.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

// tokenBytes is the entropy of a generated token before hex encoding (32 bytes
// = 256 bits).
const tokenBytes = 32

// Generate returns a new random token as a hex string. The caller is
// responsible for showing it to the user exactly once and persisting only its
// Hash.
func Generate() (string, error) {
	var b [tokenBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// Hash returns the hex-encoded SHA-256 of the token. The same input always
// produces the same output, so it is suitable for equality lookups.
func Hash(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// Verify reports whether plaintext matches the stored hash, using a
// constant-time comparison to avoid timing side channels. An empty storedHash
// (no secret set) never matches.
func Verify(storedHash, plaintext string) bool {
	if storedHash == "" {
		return false
	}
	candidate := Hash(plaintext)
	return subtle.ConstantTimeCompare([]byte(storedHash), []byte(candidate)) == 1
}
