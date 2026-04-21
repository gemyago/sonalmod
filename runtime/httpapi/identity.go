package httpapi

import (
	"context"

	"github.com/gemyago/sonalmod/runtime/internal/callerid"
)

// CallerIdentity represents the authenticated caller making a request.
type CallerIdentity = callerid.Identity

// ContextWithCallerIdentity returns a new context with the given CallerIdentity stored in it.
func ContextWithCallerIdentity(ctx context.Context, id CallerIdentity) context.Context {
	return callerid.ContextWith(ctx, id)
}

// CallerIdentityFromContext retrieves the CallerIdentity from the context.
// Returns nil if no identity is present.
func CallerIdentityFromContext(ctx context.Context) CallerIdentity { //nolint:ireturn
	return callerid.FromContext(ctx)
}
