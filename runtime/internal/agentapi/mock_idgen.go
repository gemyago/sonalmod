package agentapi

import (
	"fmt"

	"github.com/google/uuid"
)

// MockIDGen is a test double that returns a known sequence of ids by
// shifting values from an upstream generator (same approach as tools/firecrawl/internal/system/ident).
type MockIDGen struct {
	upstream      IDGen
	lastGenerated UUID
	nextGenerated UUID
}

var _ IDGen = (*MockIDGen)(nil)

// NewMockIDGen creates a MockGenerator with a default upstream DefaultGenerator.
func NewMockIDGen() *MockIDGen {
	upstream := NewDefaultIDGen()
	return &MockIDGen{
		upstream:      upstream,
		nextGenerated: upstream.MustNewV7(),
	}
}

func (m *MockIDGen) MustNewV7() UUID {
	id, err := m.NewV7()
	if err != nil {
		panic(fmt.Errorf("failed to generate UUIDv7: %w", err))
	}
	return id
}

func (m *MockIDGen) NewV7() (UUID, error) {
	id, err := m.upstream.NewV7()
	if err != nil {
		return uuid.Nil, err
	}
	result := m.nextGenerated
	m.lastGenerated = result
	m.nextGenerated = id
	return result, nil
}

// MockIDGenLastGenerated returns the last id returned from NewV7 / MustNewV7.
func MockIDGenLastGenerated(g IDGen) UUID {
	mg, ok := g.(*MockIDGen)
	if !ok {
		panic("provided Generator is not a *MockGenerator")
	}
	return mg.lastGenerated
}

// MockIDGenNextGenerated returns the id that the next successful NewV7 will return.
func MockIDGenNextGenerated(g IDGen) UUID {
	mg, ok := g.(*MockIDGen)
	if !ok {
		panic("provided Generator is not a *MockGenerator")
	}
	return mg.nextGenerated
}
