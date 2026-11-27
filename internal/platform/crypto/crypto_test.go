package crypto

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, keySize)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return k
}

func TestSealOpenRoundTrip(t *testing.T) {
	s, err := NewSealer(testKey(t))
	if err != nil {
		t.Fatalf("new sealer: %v", err)
	}
	plaintext := []byte("glpat-super-secret-token")

	blob, err := s.Seal(plaintext)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if bytes.Contains(blob, plaintext) {
		t.Fatal("sealed blob contains the plaintext")
	}

	got, err := s.Open(blob)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round-trip = %q, want %q", got, plaintext)
	}
}

func TestSealProducesDistinctCiphertexts(t *testing.T) {
	s, _ := NewSealer(testKey(t))
	a, _ := s.Seal([]byte("x"))
	b, _ := s.Seal([]byte("x"))
	if bytes.Equal(a, b) {
		t.Fatal("two seals of the same plaintext are identical (DEK/nonce reuse)")
	}
}

func TestOpenWithWrongKeyFails(t *testing.T) {
	s1, _ := NewSealer(testKey(t))
	s2, _ := NewSealer(testKey(t))
	blob, _ := s1.Seal([]byte("secret"))
	if _, err := s2.Open(blob); err == nil {
		t.Fatal("expected an error opening with a different master key")
	}
}

func TestOpenTamperedCiphertextFails(t *testing.T) {
	s, _ := NewSealer(testKey(t))
	blob, _ := s.Seal([]byte("secret"))
	blob[len(blob)-1] ^= 0xff
	if _, err := s.Open(blob); err == nil {
		t.Fatal("expected an error opening tampered ciphertext")
	}
}

func TestOpenMalformedFails(t *testing.T) {
	s, _ := NewSealer(testKey(t))
	if _, err := s.Open([]byte{0x00}); err == nil {
		t.Fatal("expected an error for a too-short blob")
	}
}

func TestNewSealerRejectsBadKeySize(t *testing.T) {
	if _, err := NewSealer([]byte("too-short")); err == nil {
		t.Fatal("expected ErrInvalidKey for a short key")
	}
}

func TestNewSealerFromBase64(t *testing.T) {
	key := testKey(t)
	s, err := NewSealerFromBase64(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("from base64: %v", err)
	}
	blob, _ := s.Seal([]byte("v"))
	if _, err := s.Open(blob); err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := NewSealerFromBase64("not-valid-base64!!!"); err == nil {
		t.Fatal("expected an error for an invalid base64 key")
	}
}
