// Package domain holds the core entities and business rules of
// PipelineSherlock. It has no dependency on HTTP, databases or external
// GitLab clients: those live in the adapter layers and depend on this package,
// never the other way around.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// Validation errors are sentinels so callers (and tests) can match on them
// with errors.Is in addition to reading the human-readable message.
var (
	// ErrInvalidProject is the umbrella error for a project that fails its
	// invariants. Specific problems are wrapped underneath it.
	ErrInvalidProject = errors.New("invalid project")

	ErrGitLabProjectID = fmt.Errorf("%w: gitlab project id must be positive", ErrInvalidProject)
	ErrEmptyName       = fmt.Errorf("%w: name must not be empty", ErrInvalidProject)
	ErrNameTooLong     = fmt.Errorf("%w: name exceeds maximum length", ErrInvalidProject)
	ErrEmptyNamespace  = fmt.Errorf("%w: namespace must not be empty", ErrInvalidProject)
	ErrNamespaceFormat = fmt.Errorf("%w: namespace contains invalid characters", ErrInvalidProject)
)

// Field limits. These are deliberately conservative; GitLab itself allows
// longer values, but bounding input protects storage and display.
const (
	maxNameLength      = 255
	maxNamespaceLength = 255
)

// ProjectID is the internal identifier assigned by the persistence layer.
// An empty ProjectID means the project has not been persisted yet.
type ProjectID string

// Project is a GitLab project registered with PipelineSherlock so its pipeline
// and job events can be ingested and analyzed.
//
// A Project is only ever constructed through NewProject, which enforces its
// invariants. Fields are exported for persistence mapping but the type carries
// no setters: updates produce new values rather than mutating in place.
type Project struct {
	// ID is assigned when the project is persisted. Zero until then.
	ID ProjectID
	// GitLabProjectID is the numeric project id in the GitLab instance.
	GitLabProjectID int64
	// Name is the human-readable project name.
	Name string
	// Namespace is the GitLab group/namespace path (e.g. "team/platform").
	Namespace string
	// WebhookSecretHash is the hash of the project's webhook token. Empty means
	// no secret has been set and webhook delivery must be rejected. The
	// plaintext token is never stored on the entity.
	WebhookSecretHash string
	// CreatedAt is set by the persistence layer at insert time. Zero until then.
	CreatedAt time.Time
}

// NewProject validates its inputs and returns an unpersisted Project (ID and
// CreatedAt are left zero for the persistence layer to assign). It aggregates
// every invariant violation into a single error so a caller sees all problems
// at once.
func NewProject(gitlabProjectID int64, name, namespace string) (Project, error) {
	name = strings.TrimSpace(name)
	namespace = strings.TrimSpace(namespace)

	var problems []error
	if gitlabProjectID <= 0 {
		problems = append(problems, ErrGitLabProjectID)
	}
	switch {
	case name == "":
		problems = append(problems, ErrEmptyName)
	case len(name) > maxNameLength:
		problems = append(problems, ErrNameTooLong)
	}
	switch {
	case namespace == "":
		problems = append(problems, ErrEmptyNamespace)
	case len(namespace) > maxNamespaceLength:
		problems = append(problems, ErrNamespaceFormat)
	case !validNamespace(namespace):
		problems = append(problems, ErrNamespaceFormat)
	}

	if len(problems) > 0 {
		return Project{}, errors.Join(problems...)
	}

	return Project{
		GitLabProjectID: gitlabProjectID,
		Name:            name,
		Namespace:       namespace,
	}, nil
}

// WithPersistence returns a copy of the project stamped with the identity and
// creation time assigned by the persistence layer. The receiver is not
// modified, keeping Project values immutable.
func (p Project) WithPersistence(id ProjectID, createdAt time.Time) Project {
	p.ID = id
	p.CreatedAt = createdAt
	return p
}

// IsPersisted reports whether the project has been assigned a stable identity.
func (p Project) IsPersisted() bool { return p.ID != "" }

// WithWebhookSecretHash returns a copy of the project carrying the given
// webhook secret hash. The receiver is not modified.
func (p Project) WithWebhookSecretHash(hash string) Project {
	p.WebhookSecretHash = hash
	return p
}

// HasWebhookSecret reports whether a webhook secret has been configured.
func (p Project) HasWebhookSecret() bool { return p.WebhookSecretHash != "" }

// validNamespace accepts GitLab-style namespace paths: one or more segments
// separated by "/", each segment made of letters, digits, '.', '_' or '-'.
func validNamespace(ns string) bool {
	segments := strings.Split(ns, "/")
	for _, seg := range segments {
		// Reject empty segments and the "." / ".." path-traversal segments.
		// A segment made entirely of dots is never a valid namespace component.
		if seg == "" || strings.Trim(seg, ".") == "" {
			return false
		}
		for _, r := range seg {
			if !isNamespaceRune(r) {
				return false
			}
		}
	}
	return true
}

func isNamespaceRune(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	switch r {
	case '.', '_', '-':
		return true
	}
	return false
}
