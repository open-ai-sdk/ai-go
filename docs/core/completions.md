# Direct completions

`ai.NewCompletion` is the low-level, provider-neutral API for exactly one
language-model request. It is useful when an application needs to inspect tool
calls or manage continuation itself.

For the simple text-only paths, `ai.Prompt(ctx, model, prompt)` and
`ai.Chat(ctx, model, prompt, history...)` return the aggregated text directly.

```go
result, err := ai.NewCompletion(model, "What is the capital of Vietnam?").
  Instructions("Answer concisely.").
  Temperature(0.2).
  Send(ctx)
if err != nil {
  return err
}

fmt.Println(result.Text)
```

The builder accepts the same normalized request controls as `llm.Request`:
messages, instructions, tool definitions and choice, output schema, common
sampling settings, typed provider options, and tool/runtime contexts. Call
`Build()` to retain a request for later, `Send(ctx)` to aggregate it, or
`Stream(ctx)` for normalized `aikit.StreamEvent` values.

## Response

`CompletionResponse` contains the text and reasoning conveniences plus an
ordered assistant `Message`. Tool-call parts keep their provider ID, name,
argument deltas, and thought signature. The response also carries the final
usage snapshot, finish reason, warnings, sources, files, and provider metadata.

This API does not execute tools. Use its `Message` to construct an explicit
follow-up request after your application has handled any tool calls. Choose
`ai.GenerateText` instead when you want the SDK's multi-step tool loop.

## Agent completions

`ai.Agent` includes the high-level `ai.Completion` capability; `ToolLoopAgent`
implements it. Its
`Prompt` and `Chat` methods run the configured multi-step tool loop, while
`agent.Completion(prompt)` returns an agent-bound builder for request-level
control:

```go
result, err := agent.Completion("Look up Hanoi weather").
  Temperature(0.2).
  Send(ctx)
```

Unlike `ai.NewCompletion`, this path inherits the agent's default tools,
approval policy, callbacks, and stop condition; tool calls may therefore
execute before the final result is returned.

## Scope

The current provider contract is stream-first, so direct completions aggregate
normalized provider events. They intentionally do not expose an opaque raw
provider response, a portable document abstraction, or provider-hosted tools.
