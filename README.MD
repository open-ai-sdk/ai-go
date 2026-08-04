# ai-go

`ai-go` is a standalone Go SDK for provider-neutral completions, typed tools, reusable multi-step
agents, and AI SDK v7 UI-message streams.

Canonical contracts live in focused packages: `llm` owns direct model calls, `agent` owns the
Builder/Agent/Runner lifecycle, `aikit` owns messages and events, and `tool` owns typed tools.
The top-level `ai` package contains only non-Agent convenience operations.

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

Use the canonical direct-completion builder for the common one-call path:

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/provider/openai"
)

func main() {
	client, err := openai.NewClient(openai.Config{
		APIKey: os.Getenv("OPENAI_API_KEY"),
	})
	if err != nil {
		panic(err)
	}
	model := client.CompletionModel("gpt-5")

	result, err := llm.NewCompletion(model, "Why is the sky blue?").
		Instructions("Answer concisely.").
		With(openai.ProviderOptions{ReasoningEffort: "low"}).
		Send(context.Background())
	if err != nil {
		panic(err)
	}

	fmt.Println(result.Text)
}
```

Typed provider option structs are the primary Go API. If provider options originate as decoded
JSON, the map fallback is strict: unknown fields, wrong types, or an option struct that does not
match the selected provider/model API returns `*llm.ProviderOptionsError` rather than being ignored.

### Direct completions

Use `llm.NewCompletion` for one direct model call when the application owns any
tool-call follow-up. The response preserves the ordered assistant
message and ID, usage, sources, generated files, finish reason, provider
metadata, and—when the provider has a native completion path—its untranslated
successful payload.

```go
completion, err := llm.NewCompletion(model, "Find the capital of Vietnam").
	Instructions("Answer in one sentence.").
	Temperature(0.2).
	Send(ctx)
if err != nil {
	return err
}
fmt.Println(completion.Text)
fmt.Printf("input=%d output=%d total=%d cached=%d reasoning=%d\\n",
	completion.Usage.InputTokens,
	completion.Usage.OutputTokens,
	completion.Usage.TotalTokens,
	completion.Usage.InputTokenDetails.CacheReadTokens,
	completion.Usage.OutputTokenDetails.ReasoningTokens,
)
```

Direct completions do not execute tools. Use the `Message` in the result to
build an explicit continuation. Their `Usage` is the one provider-call
snapshot; it includes cache-read/cache-write, reasoning, and tool-prompt
details when a provider reports them. Use `Usage.Add` to aggregate explicitly
across separate calls. See [the completion token-usage reference](docs/core/completions.md#token-usage)
for the complete field mapping. Inspect a native payload only when diagnosing
provider-specific behavior:

```go
raw, ok := llm.RawResponseAs[*openai.ResponsesResponse](completion)
```

Raw responses may contain sensitive provider data and are never logged by
ai-go automatically.

For tool execution and model continuations, build a reusable Agent and run it
through `Agent.Runner`; the Agent surface is owned only by package `agent`.

### Streaming

```go
stream, err := llm.NewCompletion(model, "Explain Go channels").Stream(ctx)
if err != nil {
	return err
}

for event := range stream {
	if event.Type == aikit.StreamEventError {
		return event.Error
	}
	if event.Type == aikit.StreamEventTextDelta {
		fmt.Print(event.TextDelta)
	}
}
```

Cancel the supplied context to stop the direct provider call. Agent streaming has a different,
single-owner iterator contract described below.

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

Native and MCP-discovered tools use the same `tool.Set` path. `ResultInvokable`
adds ordered text/JSON/image output and host-only metadata without changing the
released raw `Invokable.Invoke` API. Tool input, execution, missing-tool, and
policy-denial failures are typed and support `errors.Is`/`errors.As`; safe
model feedback never includes arbitrary operator cause text.

### Reusable Agents

Package `agent` has one canonical lifecycle: a value-style Builder validates long-lived defaults,
the resulting Agent is immutable, and each value-style Runner owns one invocation and its
overrides.

```go
assistant, err := agent.New(model).
	ID("travel-assistant").
	Instructions("Answer carefully and state uncertainty.").
	Tools(tools).
	Build()
if err != nil {
	return err
}

result, err := assistant.Runner().
	Prompt("Find the weather in Hanoi").
	Temperature(0.2).
	ActiveTools("get_weather").
	MaxTurns(4).
	Run(ctx)
if err != nil {
	var exhausted *agent.MaxTurnsError
	if errors.As(err, &exhausted) {
		return savePartial(exhausted.Result)
	}
	return err
}

fmt.Println(result.Text)
```

`MaxTurns` defaults to `1`, must be positive, and counts every model call, including tool
continuations and retries. Exhaustion is a typed terminal error carrying the partial Result and its
full Transcript; it is not a successful stop. `StopWhen` may stop earlier but cannot exceed the
budget.

For existing chat or multimodal history, `Messages(...)` replaces the Runner input, while
`Message(...)` and `Prompt(...)` append. All input and configuration are snapshotted so creating or
overriding one Runner never mutates the reusable Agent.

```go
events, err := assistant.Runner().Messages(history...).Stream(ctx)
if err != nil {
	return err
}
for event, err := range events {
	if err != nil {
		return err
	}
	if event.Type == aikit.StepEventTextDelta {
		fmt.Print(event.TextDelta)
	}
}
```

The `iter.Seq2[aikit.StepEvent, error]` is single-use and single-owner. Breaking iteration cancels
and releases work owned by the run. Use `Runner.Run` for aggregation; there are no fan-out or late
stream views. See [the Agent guide](docs/core/agents.md) for reusable configuration and
[Agent Runner](docs/core/agent-runner.md) for ordered messages and complete result
semantics, then [Hooks](docs/core/hooks.md) and [Tools](docs/core/tools.md) for
lifecycle steering, rich output, context, and approvals.

## Chat server quickstart

The checked-in server is a runnable conformance fixture for text, tool, error, approval, and denial
flows. It uses the real `aisdkhttp` boundary and is also what the browser suite boots.

```sh
go run ./examples/chat-server
curl -i http://127.0.0.1:8787/healthz
```

For an application endpoint, compose the lower packages directly:

```go
func chatRun(assistant *agent.Agent) aisdkhttp.RunFunc {
	return func(
		ctx context.Context,
		messages []aikit.Message,
	) (iter.Seq2[aikit.StepEvent, error], error) {
		return assistant.Runner().
			Messages(messages...).
			Stream(ctx)
	}
}

http.Handle("/chat", aisdkhttp.Handler(chatRun(assistant)))
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
| `ai` | Non-Agent convenience operations such as objects, embeddings, images, and cost helpers |
| `agent` | Agent Builder, immutable Agent, per-run Runner, results, hooks, approvals, and multi-turn execution |
| `aikit` | Messages, events, usage, finish reasons, warnings, and error vocabulary; standard library plus approved internal JSON cloning |
| `llm` | Model contracts, request builder, typed provider options, embeddings, and images |
| `tool` | Typed/dynamic tools, rich canonical results, safe typed errors, immutable registry, and tool context |
| `transport` | Provider HTTP policy, retry, SSE reading, cancellation, and API error mapping |
| `provider/*` | Provider-specific constructors and wire codecs |
| `aisdk` | Frozen AI SDK v7 chunks, SSE framing, UI-message conversion, invariants, and approval signatures |
| `aisdkhttp` | AI SDK v7 `net/http` request/SSE boundary |
| `mcp` | MCP clients and dynamic tool integration |

The dependency graph points downward: `aikit` remains the foundation package and may use only the
standard library plus its approved internal JSON-cloning utility; no package imports `ai`, providers
do not import the UI protocol, and `aisdk` does not import agents or providers.

## Observability

The SDK is silent by default. A caller-provided `*slog.Logger` receives structured diagnostics;
the library does not write to `slog.Default()` on its own. Tracing defaults to a true no-op and uses
the small `agent.Tracer` interface (`Builder.Tracer` or a per-run `Runner.Tracer`).

OpenTelemetry support lives in the optional `otelagent` package. The core runtime and provider
package dependency closures do not import OTel, so applications compile the adapter only when they
explicitly choose it. Prompt, completion, and tool-argument content is excluded from traces unless
the caller explicitly enables content tracing.

## License

MIT. See [LICENSE](LICENSE).
