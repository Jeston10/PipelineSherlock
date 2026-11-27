package http

import "context"

// RequestIDFromContext returns the request id assigned by requestIDMiddleware,
// or an empty string if none is present.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

const ownerIDKey contextKey = "owner_id"

// contextWithOwnerID stores the authenticated caller's owner (tenant) id so
// handlers can scope their work to it.
func contextWithOwnerID(ctx context.Context, ownerID string) context.Context {
	return context.WithValue(ctx, ownerIDKey, ownerID)
}

// OwnerIDFromContext returns the authenticated caller's owner id, or an empty
// string when the request was not authenticated.
func OwnerIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ownerIDKey).(string); ok {
		return v
	}
	return ""
}
