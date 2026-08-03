# Observability

Observability is part of the Agent execution contract, but remains opt-in. The
SDK is silent by default and never writes to `slog.Default()` on its own.

Set a `*slog.Logger` and provider-neutral `agent.Tracer` as long-lived Agent
defaults:

```go
assistant, err := agent.New(model).
  Instructions("Answer concisely.").
  Logger(logger).
  Tracer(tracer).
  Build()
if err != nil { return err }

result, err := assistant.Runner().Prompt(prompt).Run(ctx)
```

Use `Runner.Logger` or `Runner.Tracer` to override either value for one
invocation without mutating the immutable Agent. Hooks can observe lifecycle
events and run synchronously in registration order; they use the same shared
driver and Result reducer as `Run` and `Stream`.

Spans contain metadata such as model ID, turn, tool name, usage, and finish
reason, but exclude prompts, completions, and tool arguments by default. Enable
content tracing explicitly only when the application's data-handling policy
permits it:

```go
result, err := assistant.Runner().
  Prompt(prompt).
  TraceContent(true).
  Run(ctx)
```

Treat content tracing as a security and privacy decision rather than a
debugging convenience. Provider-native raw responses may also contain
sensitive data and are never logged automatically.

OpenTelemetry support is opt-in through the `otelagent` package, keeping its
dependencies outside the core runtime and provider packages. Pass the adapter
to `Builder.Tracer` or `Runner.Tracer`; package `ai` has no Agent tracing
options or forwarding API.
