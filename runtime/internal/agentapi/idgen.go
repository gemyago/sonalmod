package agentapi

import (
	"fmt"

	"github.com/google/uuid"
)

// UUID is a universally unique identifier for session and correlation ids.
// Use it in application code instead of referencing github.com/google/uuid directly at boundaries.
type UUID = uuid.UUID

// IDGen defines the contract for generating unique identifiers.
type IDGen interface {
	// MustNewV7 generates a new UUIDv7 identifier or panics on failure.
	MustNewV7() UUID

	// NewV7 generates a new UUIDv7 identifier and allows error handling.
	NewV7() (UUID, error)
}

// DefaultIDGen uses UUIDv7 from the standard google/uuid implementation.
type DefaultIDGen struct{}

var _ IDGen = (*DefaultIDGen)(nil)

// NewDefaultIDGen creates a new DefaultGenerator.
func NewDefaultIDGen() *DefaultIDGen {
	return &DefaultIDGen{}
}

func (g *DefaultIDGen) MustNewV7() UUID {
	id, err := g.NewV7()
	if err != nil {
		panic(fmt.Errorf("failed to generate UUIDv7: %w", err))
	}
	return id
}

func (g *DefaultIDGen) NewV7() (UUID, error) {
	return uuid.NewV7()
}
