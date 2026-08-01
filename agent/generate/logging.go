package generate

import (
	"log/slog"

	"github.com/open-ai-sdk/ai-go/agent"
)

// WithLogger sets the logger used for internal diagnostics on this call: a
// panic recovered at a goroutine or callback boundary, and a provider error
// response body that GenerateTextResult's typed error deliberately drops (see
// APIError). Both are logged at debug/error level, never printed elsewhere.
//
// The default is no logger at all — the SDK never writes to slog.Default(),
// because a library claiming a consumer's log stream without being asked is
// exactly the kind of surprise this option exists to prevent.
func WithLogger(l *slog.Logger) Option {
	return func(r *GenerateTextRequest) { r.Logger = l }
}

// WithTracer enables provider-neutral span instrumentation for this call.
func WithTracer(tracer agent.Tracer) Option {
	return func(r *GenerateTextRequest) { r.Tracer = tracer }
}

// WithTraceContent controls whether trace spans (see the module's OTel
// integration) may carry prompt, completion, and tool-argument content.
// Default false: spans carry only metadata — model ID, step number, tool
// name, token usage, finish reason — never the content itself, since spans
// are commonly exported to a third-party backend and prompts/completions may
// hold sensitive data the operator of that backend should not see.
func WithTraceContent(enabled bool) Option {
	return func(r *GenerateTextRequest) { r.TraceContent = enabled }
}
