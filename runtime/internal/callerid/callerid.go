// Package callerid provides the CallerIdentity interface and context helpers.
// It is intentionally kept dependency-free so it can be imported by both
// the public httpapi package and the internal agentapi package without cycles.
package callerid

import "context"

// Identity represents the authenticated caller making a request.
type Identity interface {
	UserID() string
}

type contextKey struct{}

// ContextWith returns a new context with the given Identity stored in it.
func ContextWith(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// FromContext retrieves the Identity from the context.
// Returns nil if no identity is present.
func FromContext(ctx context.Context) Identity { //nolint:ireturn
	id, _ := ctx.Value(contextKey{}).(Identity)
	return id
}
