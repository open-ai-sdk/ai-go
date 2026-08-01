# ai-go

`ai-go` is a standalone Go SDK for provider-neutral generation, typed tools, multi-step agents, and
AI SDK v7 UI-message streams.

The top-level `ai` package is a thin ergonomic facade. It keeps common operations and aliases in one
place while their implementations and canonical contracts live in focused lower packages.

## Features

- OpenAI Responses and Chat Completions, Anthropic, Gemini, Kie, and OpenAI-compatible providers
- Streaming and aggregated text generation with multi-step tool execution
- Generic typed tools with construction-time JSON Schema validation
- Structured output, embeddings, image generation, provider metadata, warnings, and typed errors
- Stateless human approval with AI SDK v7-compatible signatures
- AI SDK v7 SSE production and a tested `net/http` chat boundary
- Opt-in structured logging and a provider-neutral tracing seam

## Requirements

- Go 1.25 or newer
- Bun 1.3.13 and Chromium only when running the JavaScript/browser conformance gates

The module is still pre-1.0 and remains on `v0.x` while the restructured package layout settles.
See [CHANGELOG.md](CHANGELOG.md) before upgrading.

## Quick start

Install the module:

```sh
go get github.com/open-ai-sdk/ai-go
```

Use the facade for the common path:

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/open-ai-sdk/ai-go/ai"
	"github.com/open-ai-sdk/ai-go/provider/openai"
)

func main() {
	model := openai.NewLanguageModel("gpt-5", openai.Config{
		APIKey: os.Getenv("OPENAI_API_KEY"),
	})

	result, err := ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model:        model,
		Instructions: "Answer concisely.",
		Messages:     []ai.Message{ai.UserMessage("Why is the sky blue?")},
		ProviderOptions: map[string]any{
			"openai": openai.ProviderOptions{ReasoningEffort: "low"},
		},
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(result.Text)
}
```

Typed provider option structs are the primary Go API. If provider options originate as decoded
JSON, the map fallback is strict: unknown fields, wrong types, or an option struct that does not
match the selected provider/model API returns `*llm.ProviderOptionsError` rather than being ignored.

`GenerateText` defaults to a single step. Set `StopWhen: ai.IsStepCount(n)` for an explicit
multi-step tool loop, or use the direct `agent` runtime when the application needs event-level
control.

### Streaming

```go
stream := ai.StreamText(ctx, ai.GenerateTextRequest{
	Model:    model,
	Messages: []ai.Message{ai.UserMessage("Explain Go channels")},
})

for event, err := range stream.Events() {
	if err != nil {
		return err
	}
	if event.Type == ai.StepEventTextDelta {
		fmt.Print(event.TextDelta)
	}
}
```

Cancel the context passed to `StreamText` to stop the underlying run. Leaving an iterator view does
not cancel the run because another stream view or `Consume` call may still be active.

### Typed tools

```go
weather, err := tool.New(
	"get_weather",
	"Get weather for a city",
	func(ctx context.Context, input struct {
		City string `json:"city" description:"City name"`
	}) (struct {
		Summary string `json:"summary"`
	}, error) {
		return struct {
			Summary string `json:"summary"`
		}{Summary: lookup(input.City)}, nil
	},
)
if err != nil {
	return err
}

tools, err := tool.NewSet(weather)
if err != nil { // duplicate names are rejected
	return err
}
```

Native and MCP-discovered tools use the same `tool.Set` path. Tool input, execution, missing-tool,
and policy-denial failures are typed and support `errors.Is`/`errors.As`.

## Chat server quickstart

The checked-in server is a runnable conformance fixture for text, tool, error, approval, and denial
flows. It uses the real `aisdkhttp` boundary and is also what the browser suite boots.

```sh
go run ./examples/chat-server
curl -i http://127.0.0.1:8787/healthz
```

For an application endpoint, compose the lower packages directly:

```go
func chatRun(model llm.Model) aisdkhttp.RunFunc {
	return func(ctx context.Context, messages []aikit.Message) (<-chan aikit.StepEvent, error) {
		return agent.Stream(ctx, agent.RunParams{
			Model: model,
			Request: llm.Request{
				Messages: messages,
			},
		}), nil
	}
}

http.Handle("/chat", aisdkhttp.Handler(chatRun(model)))
```

`aisdkhttp` parses the v7 request, streams all five required response headers, flushes every chunk,
and propagates disconnect cancellation. Pre-stream failures use HTTP status codes; mid-stream
failures become redacted protocol error chunks.

Gin support is isolated in the separately versioned `aisdkgin` module and wraps this same
`net/http` handler, so Gin is not part of the core dependency graph.

## AI SDK v7 compatibility

The wire layer targets the AI SDK v7 UI-message-stream contract. The conformance project pins:

- `ai@7.0.35`
- `@ai-sdk/react@4.0.38`
- React 19.2.1
- Vite+ 0.2.6

It validates the generated chunk union, replays recorded Go SSE fixtures through the real
`readUIMessageStream`, boots `examples/chat-server`, and drives text, tool-result, approval, and
denial flows through a real React `useChat` client in Chromium.

```sh
bun --cwd conformance install --frozen-lockfile
bun --cwd conformance run check
bun --cwd conformance run test
bunx playwright install chromium
bun --cwd conformance run test:browser
```

Run these gates whenever changing `aisdk`, `aisdkhttp`, SSE framing or headers, fixture bytes, or
browser-facing tool and approval behavior. The Go API may make documented `v0.x` clean breaks; the
v7 chunk types, fields, finish-reason spellings, and `data: [DONE]` framing are treated as frozen.

## Package map

| Package | Owns |
|---|---|
| `ai` | Thin facade: `GenerateText`, `StreamText`, `GenerateObject`, `Embed`, and aliases |
| `agent/generate` | High-level generation, aggregation, request options, and stream results behind the facade |
| `agent` | Low-level multi-step loop, approval, stop conditions, callbacks, and `aikit.StepEvent` production |
| `aikit` | Dependency-free messages, events, usage, finish reasons, warnings, and error vocabulary |
| `llm` | Model contracts, request builder, typed provider options, embeddings, and images |
| `tool` | Typed/dynamic tools, schema derivation, registry, typed errors, and tool context |
| `transport` | Provider HTTP policy, retry, SSE reading, cancellation, and API error mapping |
| `provider/*` | Provider-specific constructors and wire codecs |
| `aisdk` | Frozen AI SDK v7 chunks, SSE framing, UI-message conversion, invariants, and approval signatures |
| `aisdkhttp` | AI SDK v7 `net/http` request/SSE boundary |
| `mcp` | MCP clients and dynamic tool integration |

The dependency graph points downward: `aikit` is the leaf, no package imports `ai`, providers do not
import the UI protocol, and `aisdk` does not import agents or providers.

## Observability

The SDK is silent by default. A caller-provided `*slog.Logger` receives structured diagnostics;
the library does not write to `slog.Default()` on its own. Tracing defaults to a true no-op and uses
the small `agent.Tracer` interface (`ai.WithTracer` on facade calls).

OpenTelemetry support lives in the optional `otelagent` package. The core runtime and provider
package dependency closures do not import OTel, so applications compile the adapter only when they
explicitly choose it. Prompt, completion, and tool-argument content is excluded from traces unless
the caller explicitly enables content tracing.

## License

MIT. See [LICENSE](LICENSE).
