# Changelog

This project follows semantic versioning. While the module remains on `v0.x`, minor releases may
contain documented breaking changes. `v1.0.0` will be considered only after the restructured public
layout and all release gates are stable.

`v0.0.24` is already tagged. The next core release continues to include the
optional `otelagent` package, which adapts OpenTelemetry to the
`agent.Tracer` contract.

## Unreleased

### Streaming

| Changed behavior | Replacement |
| --- | --- |
| `CompletionRequestBuilder.Stream` returned a bare `<-chan aikit.StreamEvent`, so the aggregated `*CompletionResponse` was unreachable. **Removed.** | `StreamSend(ctx)` returns a `*llm.StreamingResponse`: the same events as `iter.Seq2`, plus `Response()` after drain. `llm.Model.Stream` still returns a channel and is unchanged — it is the provider contract. |
| A direct streaming completion could not report usage, sources, files, or finish metadata without a second aggregating pass. | `StreamingResponse.Response()` returns everything `Send` returns, with no extra model call. |
| A streamed agent run could not surface its `*agent.Result`. | `Runner.StreamRun(ctx)` returns an `*agent.StepStream` carrying both. `Runner.Stream` is unchanged and now forwards to it. |
| Breaking a streamed run on `StepEventDone` recorded `context.Canceled` internally and passed it to `RunFinishedHook`. | Stopping on the terminal event is a normal exit: nil error, `StreamCompleted`. Stopping before it still reports cancellation with the partial aggregate. |
| Direct completions appended tool-call arguments unconditionally, so a provider that re-sent complete arguments produced invalid JSON. | Argument folding is shared with the agent in `aikit.ToolCallFold` and stops once the arguments parse. |
| The agent discarded a tool ID or name that arrived after the first delta, which loses the name on OpenAI-compatible providers that send it late. | The same shared fold adopts a non-empty ID or name from any delta. |

- New: `llm.StreamPrompt`, `llm.StreamChat`, `llm.Streaming`, and the
  `ai` forwarders; `Agent.StreamPrompt`, `Agent.StreamChat`,
  `Agent.StreamCompletion`.
- New: `aikit.Stream`, `aikit.StreamingPrompt`, `aikit.StreamingChat`, and
  `aikit.StreamingCompletion` — implemented at both the model and agent layers,
  so a helper can accept either.
- `StreamCompleted` reports that nothing failed, not that every event was seen.
  Several providers report usage *after* the finish event, so breaking there
  drops those counts; range to the end when the aggregate matters.

### Structured output

| Changed behavior | Replacement |
| --- | --- |
| A normal agent structured-output run made a second finishing request and could replace `Result.Text` with JSON. | The final constrained turn is parsed directly; consume `Result.StructuredOutput`, `RunObject[T]`, or `Extractor[T]`. |
| `CompleteObject[T]` decoded raw text without fence handling or schema validation. | It now uses the shared parse → validate → decode pipeline and strict output schema. |
| Structured JSON could leak as UI text deltas. | Structured output is server-side only through `StepEventStructuredOutput` until a later AI SDK data-chunk mapping. |

### Hooks and rich tools

- Tools now support additive `ResultInvokable`/`ExecutionResult` rich results.
  `Invokable.Invoke` remains source-compatible. Text, explicit JSON, and image
  content retain their type and order; metadata is host-only.
- Tool failures now use safe model-facing `tool.Details` output. Error causes
  remain available to operators through `errors.Is`/`errors.As` and are no
  longer copied into provider history by default.
- Tool invocation receives isolated request `ToolsContext`, `RuntimeContext`,
  and a tool-call ID. MCP discovery now has context-aware paginated snapshot
  constructors and preserves mixed result content without placeholders.
- `tool.Typed[Args, Output]` and `tool.Adapt` support struct-based typed tools
  with state and hand-authored provider definitions, while `tool.New` remains
  the concise function-based constructor.
- Hooks now cover completion responses, completed model turns, text/tool-call
  deltas, and stream finish in addition to completion request, tool, invalid
  call, and run-finished phases. `HookFuncs` exposes matching function fields;
  direct hook implementations opt into only the capability interfaces they
  need.
- `ModelTurnHook` may repeat a tool-free turn with `Repeat()` or append safe
  corrective feedback with `RetryWithFeedback(...)`. Each retry consumes the
  existing `MaxTurns` budget; turns containing tool calls are rejected at this
  lifecycle boundary. Runs with a model-turn hook buffer one turn until it is
  accepted, so rejected content cannot leak to Results or AI SDK v7 streams.
- High-frequency delta hooks advertise `HookInterest` through `InterestedHook`.
  `HookFuncs` derives this automatically from its delta/finish callbacks.
- Hooks retain a run-local race-safe scratchpad, request tool-choice patching,
  and `ToolResultHook`, which separates cloned raw execution facts from the
  mutable model presentation. Existing `BeforeCompletion`, `BeforeTool`,
  `AfterTool`, `InvalidToolCall`, `OnStreamEvent`, and `OnRunFinished` hook
  capabilities remain available during this pre-release API period.

This is a clean break in the Go API. The AI SDK v7 UI-message-stream wire contract remains
compatible with `ai@7.0.35` and is exercised by the conformance and Playwright suites.

### Agent and Runner rewrite

The Agent API now has one canonical owner and lifecycle:

```text
agent.New(model) Builder -> immutable *agent.Agent -> per-run agent.Runner
```

This is a source-breaking replacement, not a deprecation cycle. No aliases,
adapters, or compatibility shims are provided for the removed Agent APIs.

| Removed API or behavior | Replacement |
|---|---|
| `ai.NewToolLoopAgent`, `agent/generate.ToolLoopAgent`, and the broad Agent interface | Build a concrete reusable Agent with `agent.New(model).Build()`. Define a narrow application-local interface when substitution is needed. |
| `AgentOption`, `Option`, `WithAgent*`, and Agent request `With*` functions | Configure long-lived defaults with `agent.Builder` methods and invocation-specific overrides with `agent.Runner` methods. |
| `GenerateText`, `StreamText`, `GenerateTextRequest`, and duplicate request builders/results | Use `Runner.Run` for an aggregated `*agent.Result` or `Runner.Stream` for `iter.Seq2[aikit.StepEvent,error]`. Use `llm.NewCompletion` for one direct model call. |
| Public `agent.RunParams`, `agent.Run`, and `agent.Stream` | Build an Agent, then call `Agent.Runner().Messages(...).Run/Stream`. |
| `MaxSteps`, `IsStepCount` as a budget, zero/unbounded sentinels, and differing one/20-step defaults | `MaxTurns` is the single positive total model-call budget, defaults to `1`, and counts initial calls, continuations, and retries. `StopWhen` can only stop earlier. |
| Successful termination with pending continuation after a budget limit | `*agent.MaxTurnsError` returns the partial Result and full Transcript. Streaming yields committed events followed by that terminal error and no successful done event. |
| Fan-out `StreamResult`, `TextStream`, `Events`, `Consume`, and `DrainUnused` | Range the Runner's single-use, single-owner iterator once. Breaking iteration cancels and releases its provider/tool work. |
| `Result.Response.Messages` plus a separately stored Transcript | `agent.Result.Transcript` is the one independently owned full history; `GeneratedMessages()` derives the continuation view. |
| Mutable `tool.Set` fields and raw executor seams | Construct a validated immutable ordered registry with `tool.NewSet`; definitions and invokers are snapshotted together. |

`Runner.Messages` replaces the complete ordered input sequence;
`Runner.Message` and `Runner.Prompt` append. Full message history supports
multimodal content, preceding tool calls/results, and approval responses, and
is validated before provider I/O. `Build` and Runner preflight also reject nil
models, invalid budgets, duplicate or impossible tool configuration, and
invalid approval settings synchronously.

`aisdkhttp.RunFunc` now consumes the Runner's
`iter.Seq2[aikit.StepEvent,error]` directly. The AI SDK v7 chunk union, SSE
framing, headers, approval signatures, finish-reason mappings, and `[DONE]`
termination are unchanged; `aisdk` remains independent of `agent`.

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
| `internal/engine`, `agent/generate`, and high-level Agent internals formerly in `ai` | `agent` | Build an immutable Agent and use a per-run Runner. Package `ai` no longer aliases Agent contracts, and `agent/generate` was removed. |
| `uistream` | `aisdk` | Import AI SDK v7 chunks, inbound UI-message conversion, approval signatures, invariants, and SSE framing from `aisdk`. |
| HTTP/UI helpers formerly mixed into `uistream` or `httputil` | `aisdkhttp` | Serve AI SDK v7 POST/SSE endpoints through the `net/http` handler. |
| client-side HTTP, retry, SSE reader, and provider error helpers from `httputil` | `transport` | Providers share the lower transport package; it has no UI protocol dependency. |
| `provider/internal/openaichat` | `provider/openaicompat` | Implement an OpenAI-compatible provider with the public `openaicompat.Compat` hooks. |
| `provider/openai_compatible` | `provider/openaicompat` for new integrations | The underscore package remains a legacy convenience constructor; new compatibility providers should use the shared public implementation. |
| core Gin helper/dependency | separate `aisdkgin` module | Compose `aisdkgin.Handler(aisdkhttp.Handler(run))`; Gin is absent from the core module graph. |
| core OpenTelemetry integration | optional `otelagent` package | Pass an `agent.Tracer` through `Builder.Tracer` or `Runner.Tracer`, and import `otelagent` only when the application chooses OpenTelemetry. |
| orphaned `schema` package | `tool.New` schema derivation | The unused package was removed; tool input schemas are owned and validated by `tool`. |

All hand-written Go filenames now use `snake_case`. Files that stayed in the same package were
renamed mechanically from `kebab-case.go` to `snake_case.go`; this does not change import paths or Go
symbols. JavaScript, TypeScript, and shell filenames continue to use `kebab-case`.
The package table above records the earlier cross-package moves.

### Facade and package boundaries

- `ai` contains only non-Agent convenience operations. `agent` is the sole
  owner of multi-turn Agent configuration and execution.
- `aikit` has a standard-library-only dependency closure.
- `aisdk` is independent of agents, providers, and transport.
- providers depend downward on `llm`, `aikit`, and transport, never on `aisdk` or `ai`.
- OpenTelemetry was removed from the runtime and provider package dependency closures. The core
  exposes the optional `agent.Tracer` seam through Agent Builder/Runner methods with a no-op default; the regular
  `otelagent` package supplies the OpenTelemetry adapter.

### Behavioral changes

- Direct completions now preserve provider-native successful payloads through
  `CompletionResponse.RawResponse`, with `RawResponseAs[T]` for checked access.
  The optional `llm.CompletionModel` capability leaves stream-only custom
  providers source-compatible.
- Messages now carry provider assistant IDs and explicit image, audio,
  document, and video content kinds. Role-aware validation and deep clone
  helpers are public; legacy generic file parts remain supported.
- Agent results expose `Transcript` as the one full independently owned
  conversation. `GeneratedMessages()` derives its continuation-only view.
- Usage aggregation now retains cache, reasoning, and tool-use prompt counters
  and exposes `HasValues`, `Add`, and `Accumulate`.
- Completion, prompt, and structured-output failures have stable typed
  classifications that preserve their causes and partial state through
  `errors.Is`/`errors.As`.
- Tool results can contain explicit ordered text, JSON, and image parts.
  JSON-looking text is never reinterpreted implicitly.

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
