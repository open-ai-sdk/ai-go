# Observability

The SDK is silent by default. Pass a `*slog.Logger` through `ai.WithLogger` (or the corresponding request option) to receive structured diagnostics; the library never writes to `slog.Default()` on its own.

Tracing is provider-neutral through the small `agent.Tracer` interface. Pass it with `ai.WithTracer`. By default, trace spans contain metadata such as model ID, usage, tool name, and finish reason, but not prompt, completion, or tool-argument contents. Enable `ai.WithTraceContent(true)` only when your data-handling policy permits it.

OpenTelemetry support is opt-in in the `otelagent` package, keeping it outside the core runtime and provider dependency closures.
