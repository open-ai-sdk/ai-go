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
    Model:    model,
    System:   "You are a helpful assistant.",
    Messages: []ai.Message{ai.UserMessage("Hello!")},
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
