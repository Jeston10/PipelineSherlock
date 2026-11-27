package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNewAPIKey(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		keyHash string
		wantErr error
	}{
		{"valid", "ci-dashboard", "abc123hash", nil},
		{"trims label", "  ci-dashboard  ", "abc123hash", nil},
		{"empty name", "", "abc123hash", ErrEmptyAPIKeyName},
		{"blank name", "   ", "abc123hash", ErrEmptyAPIKeyName},
		{"name too long", strings.Repeat("x", maxAPIKeyNameLen+1), "abc123hash", ErrAPIKeyNameTooLong},
		{"empty hash", "ci-dashboard", "", ErrEmptyAPIKeyHash},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, err := NewAPIKey(tc.label, "acme", tc.keyHash)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				if !errors.Is(err, ErrInvalidAPIKey) {
					t.Errorf("error %v does not wrap ErrInvalidAPIKey", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if key.Name != strings.TrimSpace(tc.label) {
				t.Errorf("name = %q, want trimmed %q", key.Name, strings.TrimSpace(tc.label))
			}
			if key.KeyHash != tc.keyHash {
				t.Errorf("key hash = %q, want %q", key.KeyHash, tc.keyHash)
			}
			if key.OwnerID != "acme" {
				t.Errorf("owner = %q, want acme", key.OwnerID)
			}
		})
	}
}

func TestNewAPIKey_MissingOwner(t *testing.T) {
	if _, err := NewAPIKey("ci-dashboard", "  ", "abc123hash"); !errors.Is(err, ErrEmptyAPIKeyOwner) {
		t.Fatalf("err = %v, want ErrEmptyAPIKeyOwner", err)
	}
}
