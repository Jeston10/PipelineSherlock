package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewProject_Valid(t *testing.T) {
	p, err := NewProject(42, "  checkout-service  ", "team/platform")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.GitLabProjectID != 42 {
		t.Errorf("GitLabProjectID = %d, want 42", p.GitLabProjectID)
	}
	if p.Name != "checkout-service" {
		t.Errorf("Name = %q, want trimmed %q", p.Name, "checkout-service")
	}
	if p.Namespace != "team/platform" {
		t.Errorf("Namespace = %q, want %q", p.Namespace, "team/platform")
	}
	if p.IsPersisted() {
		t.Errorf("IsPersisted() = true for freshly constructed project, want false")
	}
}

func TestNewProject_Invariants(t *testing.T) {
	tests := []struct {
		name            string
		gitlabProjectID int64
		projectName     string
		namespace       string
		wantErrs        []error // sentinels expected via errors.Is
	}{
		{
			name:            "non-positive id",
			gitlabProjectID: 0,
			projectName:     "svc",
			namespace:       "team",
			wantErrs:        []error{ErrGitLabProjectID},
		},
		{
			name:            "empty name",
			gitlabProjectID: 1,
			projectName:     "   ",
			namespace:       "team",
			wantErrs:        []error{ErrEmptyName},
		},
		{
			name:            "name too long",
			gitlabProjectID: 1,
			projectName:     strings.Repeat("a", maxNameLength+1),
			namespace:       "team",
			wantErrs:        []error{ErrNameTooLong},
		},
		{
			name:            "empty namespace",
			gitlabProjectID: 1,
			projectName:     "svc",
			namespace:       "",
			wantErrs:        []error{ErrEmptyNamespace},
		},
		{
			name:            "namespace bad chars",
			gitlabProjectID: 1,
			projectName:     "svc",
			namespace:       "team/../etc",
			wantErrs:        []error{ErrNamespaceFormat},
		},
		{
			name:            "namespace with space",
			gitlabProjectID: 1,
			projectName:     "svc",
			namespace:       "team platform",
			wantErrs:        []error{ErrNamespaceFormat},
		},
		{
			name:            "multiple problems aggregated",
			gitlabProjectID: -5,
			projectName:     "",
			namespace:       "",
			wantErrs:        []error{ErrGitLabProjectID, ErrEmptyName, ErrEmptyNamespace},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewProject(tc.gitlabProjectID, tc.projectName, tc.namespace)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errors.Is(err, ErrInvalidProject) {
				t.Errorf("error %v does not wrap ErrInvalidProject", err)
			}
			for _, want := range tc.wantErrs {
				if !errors.Is(err, want) {
					t.Errorf("error %v does not match expected sentinel %v", err, want)
				}
			}
		})
	}
}

func TestProject_WithPersistence_IsImmutable(t *testing.T) {
	original, err := NewProject(7, "svc", "team")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)

	persisted := original.WithPersistence("proj_123", now)

	// Original must be untouched (value semantics / immutability).
	if original.IsPersisted() {
		t.Errorf("original project was mutated by WithPersistence")
	}
	if !persisted.IsPersisted() {
		t.Errorf("persisted copy IsPersisted() = false, want true")
	}
	if persisted.ID != "proj_123" || !persisted.CreatedAt.Equal(now) {
		t.Errorf("persisted copy = %+v, want ID proj_123 and CreatedAt %s", persisted, now)
	}
}
