# Completion and generation

ai-go offers three levels of language-model interaction. Pick the highest
level that still gives your application the control it needs:

| API | Best for | Tool behavior |
| --- | --- | --- |
| `ai.Prompt` / `ai.Chat` | A single text answer | Never executes tools |
| `ai.NewCompletion` | One configured model request and its complete assistant message | Returns tool calls for the application to handle |
| `ai.CompleteObject[T]` | One schema-constrained, typed model request | Never executes tools |
| `ai.Agent` / `ToolLoopAgent` | An agent that can continue after tool calls | Executes configured tools and can make multiple model calls |

All APIs use the same provider-neutral model contract. Providers implement
`ai.LanguageModel`: a model ID and a `Stream` method that emits normalized
`ai.StreamEvent` values. Completion APIs aggregate those events when a full
response is requested, so the provider boundary remains stream-first.

## Simple prompt and chat

`ai.Prompt` is the smallest API for one user message. `ai.Chat` adds the
previous conversation turns before the final user message. Both return only
aggregated text; use a direct completion when you need tool calls, reasoning,
usage, sources, files, or continuation messages.

```go
answer, err := ai.Prompt(ctx, model, "What is the capital of Vietnam?")
if err != nil {
  return err
}
fmt.Println(answer)

answer, err = ai.Chat(ctx, model, "And its population?",
  ai.UserMessage("Tell me about Hanoi."),
  ai.AssistantMessage("Hanoi is the capital of Vietnam."),
)
```

Neither call mutates the history supplied to `Chat`.

## Direct completions

`ai.NewCompletion` sends exactly one request to the language model. It is the
low-level, provider-neutral choice when an application owns the next step—for
example, inspecting a requested tool call and deciding whether or how to
continue the conversation.

```go
completion, err := ai.NewCompletion(model, "Find the capital of Vietnam").
  Instructions("Answer concisely.").
  Temperature(0.2).
  Send(ctx)
if err != nil {
  return err
}

fmt.Println(completion.Text)
```

`CompletionRequestBuilder` is an immutable-style value builder: every method
returns a new top-level value. It can configure instructions, messages, tool
definitions and choice, structured output, sampling settings, typed provider
options, and tool or runtime context.

```go
request := ai.NewCompletion(model, "Summarize the report").
  Instructions("Use three bullets and cite uncertainty.").
  MaxTokens(300).
  StopSequences("\n\nSources:").
  Build()

completion, err := ai.Complete(ctx, model, request)
```

`Messages` replaces the request's conversation; `Prompt` appends a user
message. Use `CompletionFromRequest` when a normalized request is already
available. `Build` copies the builder-owned top-level containers, making it
safe to retain the request value. Pointers and nested values remain shared, so
deep-copy those values before mutating a branched request; `Send` aggregates
the result immediately.

### Streaming a direct completion

Call `Stream` when output must be consumed incrementally. The returned channel
contains normalized text, reasoning, tool-call, usage, source, file, finish,
and error events. Drain the channel to receive terminal metadata and to let
the provider release its response resources; cancel `ctx` to stop early.

```go
events, err := ai.NewCompletion(model, "Explain Go interfaces").Stream(ctx)
if err != nil {
  return err
}
for event := range events {
  if event.Type == ai.StreamEventTextDelta {
    fmt.Print(event.TextDelta)
  }
}
```

## Completion response and continuation

`CompletionResponse` preserves the final assistant `Message` as well as its
text and reasoning conveniences. Its `Message.Content` preserves order across
text, reasoning, and tool-call parts, including provider tool-call IDs,
argument JSON, and thought signatures. It also exposes usage, normalized and
raw finish reasons, warnings, sources, generated files, and provider metadata.

Direct completions never execute tools. To continue after a requested tool
call, execute it in your application, add the assistant message and tool
result message to the next request, then call the model again. For an SDK-run
tool loop, use an agent instead.

## Typed direct completion

`CompleteObject[T]` is the typed counterpart to `Complete`. It derives a JSON
Schema from an exported Go struct, makes exactly one direct model call, and
unmarshals the completion text into `T`. It returns the normalized response as
well, including usage and finish metadata.

```go
type Capital struct {
  City string `json:"city"`
}

request := ai.NewCompletion(model, "What is Vietnam's capital?").
  Instructions("Return JSON only.").
  Build()

result, err := ai.CompleteObject[Capital](ctx, model, request)
if err != nil {
  return err
}
fmt.Println(result.Object.City)
```

The schema is a provider request, not local semantic validation. Validate the
result in your application when that guarantee is required. Use
[`GenerateObject`](/core/structured-output) when the request belongs to the
agent/tool-loop layer instead.

## Agent completions

`ai.Agent` exposes the high-level `ai.Completion` capability. A
`ToolLoopAgent` implements `Prompt` and `Chat` and adds `Completion(prompt)`
for per-request overrides:

```go
agent := ai.NewToolLoopAgent(model,
  ai.WithAgentInstructions("You are a careful travel assistant."),
  ai.WithAgentTools(tools),
  ai.WithAgentStopWhen(ai.IsStepCount(3)),
)

result, err := agent.Completion("Find Hanoi weather").
  Temperature(0.2).
  Send(ctx)
if err != nil {
  return err
}
fmt.Println(result.Text)
```

This builder inherits the agent's tools, approval policy, callbacks, and stop
condition. Its `Send` and `Stream` may execute tools and make multiple model
calls; direct completion's `Send` and `Stream` never do. `Build` shows the
merged `GenerateTextRequest` before execution.

The narrow `ai.Completion` interface requires only `Prompt` and `Chat`, so
custom agents stay small. The richer `Completion(prompt)` builder belongs to
the concrete `ToolLoopAgent`. Per-call request shaping is available through
convenience methods such as `TopP`, `TopK`, `Seed`, `StopSequences`,
`MaxSteps`, `StopWhen`, `ActiveTools`, and contexts; use `Options(ai.With...)`
for any other existing functional option. Options are applied after the
builder's messages, so `WithMessages` can deliberately replace them.

## Structured output and errors

To request schema-constrained output and unmarshal it into a Go value, use
[`GenerateObject`](/core/structured-output). Schema enforcement depends on the
provider and its options, so add application validation when local enforcement
is required. For a runtime-defined schema, set `Output` on a direct completion
or generation request.

`Send` and `Complete` return a partial `CompletionResponse` together with an
error when the provider emits an error after some events. Check both values if
your UI or logs can use partial output. Errors returned before streaming begins
(for example, a missing model) return no response.

## Provider and portability boundary

The normalized request and stream-event contracts deliberately avoid exposing
an opaque provider response. Provider-specific controls belong in typed
`ProviderOption` values (or validated JSON map options), while provider
metadata and raw usage fields remain available through normalized results.
This keeps direct completions portable across ai-go providers without claiming
that every provider supports every optional feature.
