// Package ctxlog carries an optional *slog.Logger across an API boundary via
// context.Context. It exists so a diagnostic emitted deep in a call chain (a
// provider dropping a raw error body before it reaches a typed error value,
// for instance) can reach the logger a caller supplied at the top of the
// call, without adding a logger parameter to every signature in between.
package ctxlog

import (
	"context"
	"log/slog"
)

type ctxKey struct{}

// discard is returned by FromContext when no logger was attached. The SDK
// must never fall back to slog.Default() — that would start writing to a
// consumer's stderr without being asked.
var discard = slog.New(slog.DiscardHandler)

// WithLogger returns a copy of ctx that carries l for FromContext to
// retrieve downstream. Passing a nil l is safe: FromContext still returns
// the discarding logger for it.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext returns the logger attached by WithLogger, or a discarding
// logger if ctx carries none (including an explicitly nil logger).
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return discard
}
