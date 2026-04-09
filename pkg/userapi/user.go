// Package userapi provides the user management HTTP API: types, store
// interface, handlers, and middleware.
package userapi

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// User is the core domain type. IDs are immutable; names are display-only
// (invariant #11 — IDENTITY).
type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateRequest is the payload for POST /api/v1/users.
type CreateRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// UpdateRequest is the payload for PUT /api/v1/users/{id}.
type UpdateRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Validate returns an error if the create request is missing required fields
// or contains an invalid email address.
func (r CreateRequest) Validate() error {
	if r.Name == "" {
		return errors.New("name is required")
	}
	if r.Email == "" {
		return errors.New("email is required")
	}
	if !validEmail(r.Email) {
		return errors.New("invalid email format")
	}
	return nil
}

// Validate returns an error if the update request is missing required fields
// or contains an invalid email address.
func (r UpdateRequest) Validate() error {
	if r.Name == "" {
		return errors.New("name is required")
	}
	if r.Email == "" {
		return errors.New("email is required")
	}
	if !validEmail(r.Email) {
		return errors.New("invalid email format")
	}
	return nil
}

// validEmail performs a basic structural check: local@domain where domain
// contains at least one dot. Intentionally simple — full RFC 5322 validation
// is overkill for a user API.
func validEmail(email string) bool {
	at := strings.IndexByte(email, '@')
	if at < 1 || at >= len(email)-1 {
		return false
	}
	domain := email[at+1:]
	dot := strings.IndexByte(domain, '.')
	return dot > 0 && dot < len(domain)-1
}

// NewID generates a new unique user ID.
func NewID() string {
	return uuid.New().String()
}

// Store is the persistence interface for users. Implementations must be
// safe for concurrent use.
type Store interface {
	Create(u User) error
	Get(id string) (User, error)
	List() ([]User, error)
	Update(u User) error
	Delete(id string) error
}

// ErrNotFound is returned when a user ID does not exist in the store.
var ErrNotFound = errors.New("user not found")

// ErrConflict is returned when a user with the same email already exists.
var ErrConflict = errors.New("email already exists")
