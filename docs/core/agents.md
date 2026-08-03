# Agents

An agent binds a language model to reusable instructions, tools, policies, and
callbacks. It runs the same provider-neutral model contract as a completion,
but may execute tools and make additional model calls until a stop condition is
met.

Use `ai.GenerateText` for a request-scoped run. Use `ai.NewToolLoopAgent` when
several runs should share the same defaults.

## Agent contract

The `ai.Agent` interface combines a small prompt/chat capability with access to
the configured tools and full generation methods:

```go
type Agent interface {
  Completion
  ID() string
  Tools() *ToolSet
  Generate(context.Context, ...Option) (*GenerateTextResult, error)
  Stream(context.Context, ...Option) (*StreamResult, error)
}
```

`ToolLoopAgent` is the standard implementation. Per-call messages, steps, and
tool calls are not stored on the agent, so one configured agent can be reused
across requests when its tools are safe for concurrent use.

## Request-scoped generation

`ai.GenerateText` runs the agent runtime from an explicit request:

```go
result, err := ai.GenerateText(ctx, ai.GenerateTextRequest{
  Model:        model,
  Instructions: "Be concise and cite uncertainty.",
  Messages:     []ai.Message{ai.UserMessage("Summarize this report")},
  Tools:        tools,
  StopWhen:     ai.IsStepCount(3),
})
if err != nil {
  return err
}

fmt.Println(result.Text)
fmt.Println(result.Steps)
```

The default stop condition is `ai.IsStepCount(1)`. A tool requested in that
step may execute, but a follow-up model call requires a stop condition that
allows another step. `MaxSteps` is an independent safety cap; zero leaves the
stop condition or a natural model finish in control.

The result aggregates text, reasoning, usage, tool results, warnings, sources,
generated files, and provider metadata across the run. `Steps` retains each
individual model call, while `Response` contains messages suitable for
conversation continuation.

## Reusable agents

Create a `ToolLoopAgent` when configuration should be shared:

```go
agent := ai.NewToolLoopAgent(model,
  ai.WithAgentID("travel-assistant"),
  ai.WithAgentInstructions("Answer carefully and state uncertainty."),
  ai.WithAgentTools(tools),
  ai.WithAgentStopWhen(ai.IsStepCount(3)),
)

answer, err := agent.Prompt(ctx, "Find the weather in Hanoi")
```

`Prompt` returns only final text. `Chat` adds caller-owned history. Use the
agent-bound completion builder when one call needs overrides while retaining
the agent defaults:

```go
result, err := agent.Completion("Find the weather in Hanoi").
  Temperature(0.2).
  ActiveTools("weather").
  Send(ctx)
```

Unlike `ai.NewCompletion`, the agent-bound builder can execute tools and call
the model multiple times. See [Completions](/core/completions) for the direct,
single-call boundary and [Tools](/core/tools) for tool construction and
approval behavior.

## Streaming an agent run

`ai.StreamText` and `ToolLoopAgent.Stream` expose step-level events, including
tool execution and transitions between model calls. This is a higher-level
stream than the raw normalized model stream returned by a direct completion.

See [Streaming](/core/streaming) for the two event layers and their ownership
rules.
