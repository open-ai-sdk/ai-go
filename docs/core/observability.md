# Observability

Observability is part of the agent execution contract, but it remains opt-in.
The SDK is silent by default and never writes to `slog.Default()` on its own.

Set a `*slog.Logger` on an explicit request, or use `ai.WithLogger` with a
request builder or agent call, to receive structured runtime diagnostics:

```go
request.Logger = logger
result, err := ai.GenerateText(ctx, request)
```

Tracing is provider-neutral through the small `agent.Tracer` interface. Set
`GenerateTextRequest.Tracer` directly or attach one with `ai.WithTracer` on a
builder. Spans contain metadata such as model ID, step, tool name, usage, and
finish reason, but exclude prompts, completions, and tool arguments by default.

Enable `ai.WithTraceContent(true)` only when the application's data-handling
policy permits recording model content. Treat this as a security and privacy
decision rather than a debugging convenience.

OpenTelemetry support is opt-in through the `otelagent` package, keeping its
dependencies outside the core runtime and provider packages.
