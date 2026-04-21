//go:build !release

package httpapi

import (
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
)

func TestCallerIdentity(t *testing.T) {
	fake := faker.New()

	t.Run("ContextWithCallerIdentity and CallerIdentityFromContext", func(t *testing.T) {
		t.Run("round-trip: set identity and read it back", func(t *testing.T) {
			userID := fake.Internet().User()
			identity := &mockCallerIdentity{userID: userID}

			ctx := ContextWithCallerIdentity(t.Context(), identity)
			got := CallerIdentityFromContext(ctx)

			assert.Equal(t, identity, got)
			assert.Equal(t, userID, got.UserID())
		})

		t.Run("absent: returns nil when no identity in context", func(t *testing.T) {
			got := CallerIdentityFromContext(t.Context())
			assert.Nil(t, got)
		})
	})
}

type mockCallerIdentity struct {
	userID string
}

func (m *mockCallerIdentity) UserID() string {
	return m.userID
}
