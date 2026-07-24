# ai-go

Go SDK for building AI-powered applications with a unified provider interface.

## Features

- **Multi-provider support** — Gemini, OpenAI (Responses + Chat Completions), and any OpenAI-compatible endpoint
- **Streaming and non-streaming** — `GenerateText` and `StreamText` with automatic tool-loop orchestration
- **Tool use** — Define tools, execute them, and let the engine handle multi-step reasoning
- **Structured output** — JSON schema-constrained generation with automatic validation
- **Provider extensions** — Generic `ProviderOptions` / `ProviderMetadata` / `Warnings` without polluting core types
- **UI streaming** — SSE adapter emitting AI SDK UI message stream protocol chunks
- **Embeddings** — Unified embedding model interface

## Packages

| Package | Description |
|---------|-------------|
| `ai` | Public API: `GenerateText`, `StreamText`, request/result types, model interfaces |
| `provider/gemini` | Google Gemini provider |
| `provider/openai` | OpenAI Responses API + Chat Completions provider |
| `provider/openai_compatible` | Generic provider for any OpenAI-compatible endpoint |
| `uistream` | SSE adapter and Writer for AI SDK UI message stream protocol |
| `schema` | JSON Schema builder for structured output |

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
text := adapter.Stream(stream, &buf)
// buf contains SSE-encoded AI SDK UI message stream chunks
```

### Tool approval

Gate individual tool calls behind a human decision. `ToolApproval` maps a tool
name to a policy that decides whether the call needs approval; `ToolApprovalResponder`
resolves each request (blocking until a decision or context cancellation). A call
proceeds only when the responder approves it — the default is deny.

```go
result, _ := ai.GenerateText(ctx, ai.GenerateTextRequest{
    Model:    model,
    Messages: []ai.Message{ai.UserMessage("Delete stale records")},
    Tools:    tools,
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

### Tool context

Supply per-tool data with `ToolsContext` (keyed by tool name) and run-wide data
with `RuntimeContext`. Both reach the executor through the call `context.Context`;
read them with `ai.ToolContextFrom` / `ai.TypedToolContext[T]` and `ai.RuntimeContextFrom`.
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
    toolCtx, _ := ai.ToolContextFrom(ctx)      // per-tool value for `name`
    runtime := ai.RuntimeContextFrom(ctx)      // shared across all tools
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
`Instructions`, lifecycle callbacks use `OnStepEnd`/`OnEnd`, `StreamResult.Stream()` replaces
`Events()`, and usage is reported as `InputTokens`/`OutputTokens` with nested cache and reasoning
details. Aggregated results expose `Usage` and `FinalStep`.

Images no longer have a dedicated content part. Everything non-text travels as a file part carrying
a `MediaType` — either a full IANA media type (`image/png`) or the bare top-level segment (`image`).
`ImageURLPart`/`ImageDataPart`/`ImageFileIDPart` keep their signatures as conveniences that emit file
parts; `MimeType` fields are now `MediaType`, and tool-result content is `text` or `file`.

UI streams support v7 `tool-approval-response`, `custom`, and `reasoning-file` chunks. MCP HTTP
transports reject redirects by default; set `Redirect: mcp.RedirectFollow` only for trusted endpoints.

Provider-specific v7 defaults (OpenAI Responses reasoning detail, Anthropic cache-creation removal,
Google provider rename, xAI Responses default, and runtime rejection of system messages) remain deferred.

## Requirements

- Go 1.25+

## License

MIT — see [LICENSE](LICENSE) for details.
