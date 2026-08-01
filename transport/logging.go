package transport

import (
	"context"
	"log/slog"
)

type loggerContextKey struct{}

var discardLogger = slog.New(slog.DiscardHandler)

// WithLogger returns a context carrying the optional diagnostics logger used by
// transport error mapping. A nil logger preserves the default no-output policy.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey{}, logger)
}

// LoggerFromContext returns the attached logger or a discard-backed logger.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerContextKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return discardLogger
}
