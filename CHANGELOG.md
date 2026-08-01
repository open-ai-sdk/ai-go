# Changelog

This project follows semantic versioning. While the module remains on `v0.x`, minor releases may
contain documented breaking changes. `v1.0.0` will be considered only after the restructured public
layout and all release gates are stable.

The next core release is `v0.0.24`. The optional `otelagent` package is part of
that module and adapts OpenTelemetry to the new `agent.Tracer` contract.

## Unreleased

This is a clean break in the Go API. The AI SDK v7 UI-message-stream wire contract remains
compatible with `ai@7.0.35` and is exercised by the conformance and Playwright suites.

### Package migration

| Previous import or location | New import or location | Migration |
|---|---|---|
| `aitypes` | `aikit` | Import the dependency-free shared message, event, usage, finish-reason, warning, and error vocabulary from `aikit`. |
| `ai.LanguageModel` | `llm.Model` | Implement or accept the minimal model interface from `llm`; the facade may re-export an alias for common calls. |
| `ai.LanguageModelRequest` | `llm.Request` | Construct normalized model requests with `llm.NewRequest(...).Build()` or an explicit `llm.Request`. |
| `ai.CallSettings` and model request options | `llm.CallSettings` and `llm.RequestBuilder` | Use the lower-level request builder for model-boundary settings and typed provider options. |
| `ai.EmbeddingModel`, `ai.EmbedRequest`, `ai.EmbedResult` | `llm.EmbeddingModel`, `llm.EmbedRequest`, `llm.EmbedResult` | Embedding contracts and execution are owned by `llm`; `ai.Embed` remains the common facade entry point. |
| `ai.ImageModel` and image request/result contracts | `llm.ImageModel` and `llm` image contracts | Import advanced image-model contracts from `llm`. |
| `ai.DefineTool[T]` | `tool.New[In, Out]` | Return typed Go values from the handler; `tool.New` derives input schema and marshals output. Construction now returns an error. |
| `ai.ToolSet` and tool lookup helpers | `tool.Set` | Build a duplicate-safe set with `tool.NewSet`; duplicate names return an error instead of silently winning. |
| `ai` tool context helpers and tool errors | `tool` | Use `tool.ToolContextFrom`, `tool.RuntimeContextFrom`, and errors classifiable with `errors.Is`/`errors.As`. |
| `internal/engine` | `agent` | Use the public `agent.Run`/`agent.Stream` runtime for direct multi-step execution. The package speaks `llm.Request` and emits `aikit.StepEvent`. |
| high-level generation internals formerly in `ai` | `agent/generate` | The thin `ai` facade delegates generation, aggregation, options, and stream-result behavior to this lower package and re-exports aliases for the common surface. |
| `uistream` | `aisdk` | Import AI SDK v7 chunks, inbound UI-message conversion, approval signatures, invariants, and SSE framing from `aisdk`. |
| HTTP/UI helpers formerly mixed into `uistream` or `httputil` | `aisdkhttp` | Serve AI SDK v7 POST/SSE endpoints through the `net/http` handler. |
| client-side HTTP, retry, SSE reader, and provider error helpers from `httputil` | `transport` | Providers share the lower transport package; it has no UI protocol dependency. |
| `provider/internal/openaichat` | `provider/openaicompat` | Implement an OpenAI-compatible provider with the public `openaicompat.Compat` hooks. |
| `provider/openai_compatible` | `provider/openaicompat` for new integrations | The underscore package remains a legacy convenience constructor; new compatibility providers should use the shared public implementation. |
| core Gin helper/dependency | separate `aisdkgin` module | Compose `aisdkgin.Handler(aisdkhttp.Handler(run))`; Gin is absent from the core module graph. |
| core OpenTelemetry integration | optional `otelagent` package | Pass an `agent.Tracer` (or `ai.WithTracer`) and import `otelagent` only when the application chooses OpenTelemetry. |
| orphaned `schema` package | `tool.New` schema derivation | The unused package was removed; tool input schemas are owned and validated by `tool`. |

All hand-written Go filenames now use `snake_case`. Files that stayed in the same package were
renamed mechanically from `kebab-case.go` to `snake_case.go`; this does not change import paths or Go
symbols. JavaScript, TypeScript, and shell filenames continue to use `kebab-case`.
The package table above records the earlier cross-package moves.

### Facade and package boundaries

- `ai` is a thin top-level facade for `GenerateText`, `StreamText`, `GenerateObject`, and `Embed`.
  It reuses lower-package contracts rather than declaring a second model/message vocabulary.
- `aikit` has a standard-library-only dependency closure.
- `aisdk` is independent of agents, providers, and transport.
- providers depend downward on `llm`, `aikit`, and transport, never on `aisdk` or `ai`.
- OpenTelemetry was removed from the runtime and provider package dependency closures. The core
  exposes the optional `agent.Tracer`/`ai.WithTracer` seam with a no-op default; the regular
  `otelagent` package supplies the OpenTelemetry adapter.

### Behavioral changes

- Removed `Writer.WriteSource` and `Writer.WriteSources`. Their `source` and `sources` chunk types
  are not members of the AI SDK v7 union. Emit `source-url` with `WriteSourceURL`, emit
  `source-document` with `WriteSourceDocument`, or let `ChunkProducer` convert `aikit.Source`
  events.
- Finish reasons are translated at the wire boundary: `tool_calls` becomes `tool-calls`,
  `content_filter` becomes `content-filter`, and `unknown` becomes `other`. Internal Go values keep
  the `aikit.FinishReason` vocabulary.
- JSON-decoded provider options are strict. Unknown fields, wrong value types, and option structs
  that do not match the selected provider/model API return `*llm.ProviderOptionsError` before an
  HTTP request instead of being silently ignored. Typed provider option structs are the primary Go
  API.
- JSON numeric provider options such as OpenAI `maxOutputTokens` and Anthropic `thinkingBudget` no
  longer disappear after `encoding/json` decodes them as `float64`.
- `Usage.Raw` now survives provider decoding and reaches public results; the former facade/engine
  conversion layer no longer drops it.
- Tool schema errors are reported when `tool.New` constructs the tool, duplicate tool names are
  rejected, and input, execution, and denial failures have distinct typed errors.

### AI SDK v7 protocol fixes

- Responses now emit the complete five-header AI SDK v7 set, including
  `x-accel-buffering: no`, and flush each chunk promptly.
- Error streams close any open text/reasoning block, emit a redacted error chunk, and terminate with
  `[DONE]`.
- Inbound approvals are read from the UI message-part state posted by `useChat`; the obsolete
  top-level approval-response field was removed.
- Approval requests use the AI SDK v7 HMAC/canonical-JSON contract and are stateless across HTTP
  requests. The signature authenticates the gated tool call and input, not the user's approval
  decision.

### Verification

The release gate covers Go build/vet/race tests, the external `compat-test` module, AI SDK v7 schema
and live-server conformance, Chromium `useChat` flows, lint, dependency boundaries, filename rules,
and retained benchmarks.
