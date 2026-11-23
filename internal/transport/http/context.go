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
