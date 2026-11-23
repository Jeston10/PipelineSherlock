package token

import "testing"

func TestGenerate_UniqueAndHex(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		tok, err := Generate()
		if err != nil {
			t.Fatalf("Generate() error: %v", err)
		}
		if len(tok) != tokenBytes*2 {
			t.Fatalf("token length = %d, want %d hex chars", len(tok), tokenBytes*2)
		}
		if seen[tok] {
			t.Fatalf("Generate() returned a duplicate token: %s", tok)
		}
		seen[tok] = true
	}
}

func TestHash_Deterministic(t *testing.T) {
	a := Hash("secret-token")
	b := Hash("secret-token")
	if a != b {
		t.Errorf("Hash not deterministic: %q != %q", a, b)
	}
	if a == Hash("different") {
		t.Errorf("Hash collision for different inputs")
	}
	if a == "secret-token" {
		t.Errorf("Hash returned the plaintext")
	}
}

func TestVerify(t *testing.T) {
	tok, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	stored := Hash(tok)

	if !Verify(stored, tok) {
		t.Errorf("Verify() = false for the matching token")
	}
	if Verify(stored, tok+"x") {
		t.Errorf("Verify() = true for a non-matching token")
	}
	if Verify("", tok) {
		t.Errorf("Verify() = true for an empty stored hash (no secret set)")
	}
}
