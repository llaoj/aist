package app

import "context"

type contextKey struct{}

// WithContext returns a context that carries the application.
func WithContext(ctx context.Context, a *App) context.Context {
	return context.WithValue(ctx, contextKey{}, a)
}

// FromContext returns the application stored in ctx, if any.
func FromContext(ctx context.Context) *App {
	a, _ := ctx.Value(contextKey{}).(*App)
	return a
}
