# ai-go

Go SDK for building AI-powered applications with a unified provider interface.

## Features

- **Multi-provider support** — Gemini, OpenAI (Responses + Chat Completions), and any OpenAI-compatible endpoint
- **Streaming and non-streaming** — `GenerateText` and `StreamText` with automatic tool-loop orchestration
- **Tool use** — Define tools, execute them, and let the agent runtime handle multi-step reasoning
- **Structured output** — JSON schema-constrained generation with automatic validation
- **Provider extensions** — Generic `ProviderOptions` / `ProviderMetadata` / `Warnings` without polluting core types
- **UI streaming** — SSE adapter emitting AI SDK UI message stream protocol chunks
- **Embeddings** — Unified embedding model interface

## AI SDK v7 conformance

The Bun-managed `conformance/` harness pins `ai@7.0.35` and uses Vite+ 0.2.6 on the
Vite 8.1.5 core to check every recorded Go SSE
fixture against the real UI-message-chunk schema and `readUIMessageStream`.
It also starts a live Go server and drives text, tool, approval, and denial
flows through a real React `useChat` client in Chromium.

```sh
cd conformance
bun install --frozen-lockfile
bun run check
bun run test
bunx playwright install chromium
bun run test:browser
```

Run both conformance commands whenever changing `uistream`, response headers,
SSE framing, fixture bytes, or browser-facing tool/approval behavior.

## Packages

| Package | Description |
|---------|-------------|
| `ai` | Public API: `GenerateText`, `StreamText`, request/result types, model interfaces |
| `agent` | Public multi-step tool-loop runtime, stop conditions, approval gate, and lifecycle callbacks |
| `aikit` | Dependency-free shared messages, events, usage, and error vocabulary |
| `llm` | Language-model contracts and request builder |
| `tool` | Typed tools and duplicate-safe runtime registry |
| `transport` | Shared HTTP, SSE, retry, and provider-error transport |
| `provider/gemini` | Google Gemini provider |
| `provider/openai` | OpenAI Responses API + Chat Completions provider |
| `provider/openai_compatible` | Generic provider for any OpenAI-compatible endpoint |
| `uistream` | SSE adapter and Writer for AI SDK UI message stream protocol |

## Quick Start

```go
import (
    "github.com/open-ai-sdk/ai-go/ai"
    "github.com/open-ai-sdk/ai-go/provider/openai"
)

// OpenAI Chat Completions
model := openai.NewChatLanguageModel("gpt-4o", openai.Config{
    APIKey: os.Getenv("OPENAI_API_KEY"),
})

result, err := ai.GenerateText(ctx, ai.GenerateTextRequest{
    Model:        model,
    Instructions: "You are a helpful assistant.",
    Messages:     []ai.Message{ai.UserMessage("Hello!")},
})
fmt.Println(result.Text)
```

### OpenAI Responses API

```go
model := openai.NewLanguageModel("gpt-4o", openai.Config{
    APIKey: os.Getenv("OPENAI_API_KEY"),
})

result, _ := ai.GenerateText(ctx, ai.GenerateTextRequest{
    Model:    model,
    Messages: []ai.Message{ai.UserMessage("Search the web for Go 1.25 features")},
    ProviderOptions: map[string]any{
        "openai": openai.ProviderOptions{
            EnableWebSearch: true,
            IncludeSources:  true,
        },
    },
})
```

### Gemini

```go
import "github.com/open-ai-sdk/ai-go/provider/gemini"

model := gemini.NewLanguageModel("gemini-2.0-flash", gemini.Config{
    APIKey: os.Getenv("GEMINI_API_KEY"),
})
```

### OpenAI-Compatible Provider

```go
import "github.com/open-ai-sdk/ai-go/provider/openai_compatible"

model := openai_compatible.NewLanguageModel("deepseek-chat", openai_compatible.Config{
    BaseURL: "https://api.deepseek.com/v1",
    APIKey:  os.Getenv("DEEPSEEK_API_KEY"),
})
```

### Reasoning / Thinking (Claude, o-series, gpt-5)

Enable extended thinking for reasoning-capable models by setting `ReasoningEffort`
in provider options. This sends `reasoning_effort` in the request body, which proxy
servers like CLIProxyAPI translate to provider-native thinking parameters.

```go
// Chat Completions path — enable thinking:
model := openai.NewChatLanguageModel("claude-sonnet-4-20250514", openai.Config{
    BaseURL: "http://localhost:8080/v1",  // e.g. CLIProxyAPI
    APIKey:  os.Getenv("API_KEY"),
})

result, _ := ai.StreamText(ctx, ai.GenerateTextRequest{
    Model:    model,
    Messages: []ai.Message{ai.UserMessage("Explain quantum entanglement")},
    ProviderOptions: map[string]any{
        "openai": openai.ChatProviderOptions{
            ReasoningEffort: "high",  // "low", "medium", "high"
        },
    },
})
```

When displaying the stream in a React UI via the `uistream` package, set
`SendReasoning: true` to forward thinking chunks to the frontend:

```go
chunks := uistream.ToUIMessageStream(streamResult, "msg-1", uistream.ToUIStreamOptions{
    SendReasoning: true,  // Required for thinking/reasoning display
    SendSources:   true,
})
```

### Streaming with UI Protocol

```go
stream := ai.StreamText(ctx, req)

var buf bytes.Buffer
adapter := uistream.NewAdapter("msg-1")
text := adapter.Stream(stream.Stream(), &buf)
// buf contains SSE-encoded AI SDK UI message stream chunks
```

### Typed tools

Create native tools with `tool.New[In, Out]`. The input JSON Schema is derived
from `In` when the tool is constructed, and `Out` is JSON-marshaled after the
handler returns. Unsupported input fields fail during construction rather than
during the first call.

```go
import "github.com/open-ai-sdk/ai-go/tool"

type weatherInput struct {
    Location string `json:"location" description:"City and state"`
    Unit     string `json:"unit" enum:"celsius,fahrenheit"`
}

type weatherOutput struct {
    Summary string `json:"summary"`
}

weather, err := tool.New(
    "getWeather",
    "Get the current weather",
    func(ctx context.Context, input weatherInput) (weatherOutput, error) {
        return weatherOutput{Summary: lookupWeather(input.Location, input.Unit)}, nil
    },
)
if err != nil {
    return err
}

tools, err := tool.NewSet(weather)
if err != nil { // duplicate tool names are rejected
    return err
}
```

Runtime-described tools use `tool.NewDynamic`; MCP clients expose those through
the same `tool.Set` and agent-loop invocation path.

### Tool approval

Gate individual tool calls behind a human decision. `ToolApproval` maps a tool
name to a policy that decides whether the call needs approval. An optional
`ToolApprovalResponder` resolves the request inside the current invocation. If
it is omitted, the invocation suspends after returning the pending tool call;
resume it by sending the returned response messages plus a
`ToolApprovalResponsePart` in the next request. The runtime reconstructs the
pending call from history and retains no cross-request state.

```go
result, _ := ai.GenerateText(ctx, ai.GenerateTextRequest{
    Model:    model,
    Messages: []ai.Message{ai.UserMessage("Delete stale records")},
    Tools:    tools,
    ToolApprovalKey: approvalKey, // 32+ server-side random bytes
    ToolApproval: map[string]ai.ToolApprovalFunc{
        "deleteRecords": func(name string, args json.RawMessage) ai.ApprovalDecision {
            return ai.ApprovalRequired
        },
    },
    ToolApprovalResponder: func(ctx context.Context, req ai.ToolApprovalRequest) (ai.ToolApprovalResponse, error) {
        approved := promptOperator(req.ToolName, req.Args) // your UI/CLI decision
        return ai.ToolApprovalResponse{ApprovalID: req.ApprovalID, Approved: approved}, nil
    },
})
```

For a UI or other asynchronous approval flow, omit the responder and resume
after the operator decides. Configure the same random key, replay guard, and
approval policy on both invocations. Derive a distinct server-side key for each
authenticated tenant or conversation audience; never send that key to the
client. The AI SDK v7 signature binds the approval ID, tool call ID, tool name,
and canonical arguments so edited or fabricated history is rejected before any
tool runs. It proves that the server gated the exact call, not that a user chose
`Approved`; authenticate and authorize the user who submits that decision.
Non-I-JSON input containing invalid UTF-8 or unpaired surrogate escapes fails
closed instead of being silently repaired differently by Go and JavaScript.

Keep approval responses in a dedicated user message with no other content. An
approved signature is an at-most-once capability: the replay guard reserves it,
marks it complete after a tool attempt, and releases unstarted claims on
cancellation. `NewMemoryToolApprovalReplayGuard` is for tests, development, and
bounded-lifetime jobs; production needs a bounded shared transactional store
with claim leases/fencing. Preserve the returned, runtime-signed tool result
messages on later continuations. Side-effecting executors should also use
`tool.ToolCallIDFromContext` as their idempotency key for client retries and
cancellation reconciliation.

```go
approvalKey := make([]byte, 32)
_, _ = rand.Read(approvalKey) // persist/share this key across server instances
approvalPolicy := map[string]ai.ToolApprovalFunc{
    "deleteRecords": func(string, json.RawMessage) ai.ApprovalDecision {
        return ai.ApprovalRequired
    },
}
replayGuard := ai.NewMemoryToolApprovalReplayGuard() // use a durable shared guard in a cluster

result, err := ai.GenerateText(ctx, ai.GenerateTextRequest{
    Model: model, Messages: request.Messages, Tools: tools,
    ToolApproval: approvalPolicy, ToolApprovalKey: approvalKey,
})
pending := result.ToolApprovalRequests[0]
history := append(append([]ai.Message{}, request.Messages...), result.Response.Messages...)
history = append(history, ai.Message{
    Role: ai.RoleUser,
    Content: []ai.ContentPart{
        ai.ToolApprovalResponsePart(pending.ApprovalID, pending.Signature, true, "approved"),
    },
})

result, err = ai.GenerateText(ctx, ai.GenerateTextRequest{
    Model: model, Messages: history, Tools: tools,
    ToolApproval: approvalPolicy, ToolApprovalKey: approvalKey,
    ToolApprovalReplayGuard: replayGuard,
})
```

### Tool context

Supply per-tool data with `ToolsContext` (keyed by tool name) and run-wide data
with `RuntimeContext`. Both reach the executor through the call `context.Context`;
read them with `tool.ToolContextFrom` / `tool.TypedContext[T]` and
`tool.RuntimeContextFrom`.
When a tool defines a `ContextSchema`, its `ToolsContext` entry is required and the
call fails fast if it is missing (in both `GenerateText` and `StreamText`).

```go
ai.GenerateTextRequest{
    Model:          model,
    Tools:          tools,
    ToolsContext:   ai.ToolsContext{"lookup": map[string]any{"tenant": "acme"}},
    RuntimeContext: ai.RuntimeContext{"requestID": "req-123"},
}

// Inside the tool executor:
func (e myExec) Execute(ctx context.Context, name, args string) (string, error) {
    toolCtx, _ := tool.ToolContextFrom(ctx) // per-tool value for `name`
    runtime := tool.RuntimeContextFrom(ctx) // shared across all tools
    _ = toolCtx
    _ = runtime
    // ...
}
```

### Agents

`ai.Agent` binds a model, tools, and step-loop defaults, then delegates each call to
`GenerateText`/`StreamText` — it never runs a loop of its own. `ai.ToolLoopAgent` is the reference
implementation; consumers may implement `Agent` themselves (four methods, all in terms of the
existing public types).

```go
agent := ai.NewToolLoopAgent(model,
    ai.WithAgentTools(tools),
    ai.WithAgentInstructions("You are a helpful assistant."),
    // Unlike a bare GenerateText call (see "Breaking change" below), an agent
    // defaults to a real multi-step loop: stopWhen=IsStepCount(20).
)

result, err := agent.Generate(ctx, ai.WithMessages(ai.UserMessage("Hello!")))
```

`Generate`/`Stream` accept the same `ai.Option` values as the bare functions; scalar options (model,
tools, stop condition, ...) override the agent's defaults for that call only, while `OnStepEnd`/
`OnEnd`/`OnChunk`/`OnError` set at the agent level and at the call level both fire — concurrently,
with either side's panic swallowed — instead of one replacing the other.

Stream an agent over SSE with the free functions in `uistream` (never methods on the agent, to keep
`uistream`'s existing one-way dependency on `ai`):

```go
chunks, err := uistream.AgentStream(ctx, agent, "msg-1", ai.WithMessages(ai.UserMessage("Hello!")))

// Or serve it directly:
http.Handle("/chat", uistream.AgentHandler(agent))
```

### Breaking change: tool-loop step count now has no implicit default

`GenerateText`/`StreamText` used to fall back to an implicit 10-step budget whenever neither
`MaxSteps` nor `StopWhen` was set. That implicit cap is gone: `MaxSteps` now means "unbounded" when
zero, and `StopWhen` is the only gate, matching `ai-sdk-node` (which has no `maxSteps` concept at
all). `generateText`/`streamText` default to `StopWhen: IsStepCount(1)` — a single step; a tool call
in that step still executes, but the loop no longer continues into a follow-up model call unless you
set a wider `StopWhen` (or use `ai.NewToolLoopAgent`, which defaults to `IsStepCount(20)`).

If your code relied on the old implicit multi-step behavior without setting `StopWhen`, add one
explicitly — `ai.IsStepCount(n)` or `ai.Never()` (run until the model stops calling tools, bounded
only by `MaxSteps` if you set one).

## v7 protocol migration

This release intentionally breaks compatibility with the earlier API: request `System` is now
`Instructions`, lifecycle callbacks use `OnStepEnd`/`OnEnd`, `StreamResult` gained an `Events()`
iterator alongside `Stream()`, and usage is reported as `InputTokens`/`OutputTokens` with nested
cache and reasoning details. Aggregated results expose `Usage` and `FinalStep`.

Images no longer have a dedicated content part. Everything non-text travels as a file part carrying
a `MediaType` — either a full IANA media type (`image/png`) or the bare top-level segment (`image`).
`ImageURLPart`/`ImageDataPart`/`ImageFileIDPart` keep their signatures as conveniences that emit file
parts; `MimeType` fields are now `MediaType`, and tool-result content is `text` or `file`.

UI streams support v7 `tool-approval-response`, `custom`, and `reasoning-file` chunks. MCP HTTP
transports reject redirects by default; set `Redirect: mcp.RedirectFollow` only for trusted endpoints.

Provider-specific v7 defaults (OpenAI Responses reasoning detail, Anthropic cache-creation removal,
Google provider rename, xAI Responses default, and runtime rejection of system messages) remain deferred.

## Streaming views

`StreamText` returns a `*StreamResult` that fans out to as many views as a caller registers; each
view gets its own branch of the underlying event stream, so using more than one (e.g. `TextStream()`
for display and `Consume()` for the final result) does not race or steal events from the others.

| View | Returns | Use for |
|---|---|---|
| `Stream()` | `<-chan StepEvent` | Raw engine events, one channel |
| `Events()` | `iter.Seq2[StepEvent, error]` | `for ev, err := range sr.Events()` — idiomatic Go 1.23+ iteration |
| `TextStream()` | `<-chan string` | Text deltas only, e.g. printing to a terminal |
| `Consume()` | `(*GenerateTextResult, error)` | Block for the full aggregated result |

```go
sr := ai.StreamText(ctx, req)
for ev, err := range sr.Events() {
    if err != nil {
        // err is the stream's terminal error (including a mid-stream provider failure);
        // ranging simply stops after this — no further events follow.
        break
    }
    if ev.Type == ai.StepEventTextDelta {
        fmt.Print(ev.TextDelta)
    }
}
```

Breaking out of a `range` early unregisters only that view — it does not cancel the run, since other
views (or a later `Consume()`) may still be reading. To stop the run itself, cancel the `context.Context`
passed to `StreamText`.

## Error handling

Provider HTTP failures surface as `*ai.APIError` (an alias of the shared `aikit.APIError`), never as
an opaque string. The raw response body is deliberately never embedded — it has, in other SDKs, leaked
credentials and internal identifiers through error logs — but the fields a caller actually needs
(status code, provider error code/message, request ID, `Retry-After`) are parsed out and kept:

```go
result, err := ai.GenerateText(ctx, req)
if err != nil {
    var apiErr *ai.APIError
    if errors.As(err, &apiErr) {
        fmt.Println(apiErr.Provider, apiErr.StatusCode, apiErr.RequestID)
    }
    switch {
    case errors.Is(err, ai.ErrRateLimited):
        // apiErr.RetryAfter holds the provider's Retry-After value, when sent.
    case errors.Is(err, ai.ErrContextLength):
    case errors.Is(err, ai.ErrUnauthorized):
    }
}
```

`tool.ErrNoSuchTool` classifies a tool call for a name the request's `tool.Set`
does not define, the same way — use `errors.Is(err, tool.ErrNoSuchTool)` rather
than matching an error string. Input, execution, and policy-denial failures are
similarly classified by `tool.ErrInput`, `tool.ErrExecution`, and
`tool.ErrDenied`. Executed calls retain that typed value in
`ToolResult.Error`; `ToolResult.Output` remains the raw model/wire string.

## Observability

By default the SDK produces no logs and no trace spans: instrumentation is opt-in, never assumed.

`WithLogger` injects a `*slog.Logger` that receives structured diagnostics — a panic recovered at a
goroutine or callback boundary, and a provider error body that `*APIError` deliberately drops (see
"Error handling" above) — at debug/error level. Leaving it unset is silent; the SDK never writes to
`slog.Default()`, since a library taking over a consumer's log stream without being asked is exactly
the surprise this option exists to prevent.

```go
logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
result, err := ai.GenerateText(ctx, ai.GenerateTextRequest{
    Model:    model,
    Messages: []ai.Message{ai.UserMessage("Hello!")},
    Logger:   logger,
})
```

Tracing follows the standard OpenTelemetry pattern: the SDK always instruments run/step/model-call/
tool-call spans through the `go.opentelemetry.io/otel/trace` API, but until *your application* calls
`otel.SetTracerProvider(...)` with a real SDK, that global provider is OTel's own no-op — spans cost a
few interface calls and nothing is exported. No `ai-go`-specific option is needed to turn tracing on;
wiring up an exporter is the application's concern, not this SDK's.

Span attributes are metadata only by default — model ID, step number, tool name, token usage, finish
reason — never prompt, completion, or tool-argument content, since spans are commonly exported to a
third-party backend. Opt into content with `ai.WithTraceContent(true)` on the request:

```go
result, err := ai.GenerateText(ctx, ai.GenerateTextRequest{
    Model:        model,
    Messages:     []ai.Message{ai.UserMessage("Hello!")},
    TraceContent: true, // spans now include prompt/completion/tool-argument text
})
```

## Requirements

- Go 1.25+

## License

MIT — see [LICENSE](LICENSE) for details.
