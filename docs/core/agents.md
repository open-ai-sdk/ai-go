# Agents

Package `agent` owns the reusable multi-step API. Its lifecycle has three
parts:

```text
agent.Builder -> immutable *agent.Agent -> per-run agent.Runner
```

The builder holds long-lived defaults, `Build` validates and snapshots them,
and each Runner owns one invocation's input and overrides. There is no Agent
API in package `ai` and no compatibility layer for the previous functional
options or Agent implementation.

## Build an Agent

Create tools first, place them in an ordered immutable registry, then build the
Agent:

```go
weather, err := tool.New(
	"weather",
	"Get the current weather for a city",
	func(ctx context.Context, input struct {
		City string `json:"city" description:"City name"`
	}) (struct {
		Summary string `json:"summary"`
	}, error) {
		return struct {
			Summary string `json:"summary"`
		}{Summary: lookupWeather(input.City)}, nil
	},
)
if err != nil {
	return err
}

tools, err := tool.NewSet(weather)
if err != nil {
	return err
}

assistant, err := agent.New(model).
	ID("travel-assistant").
	Instructions("Answer carefully and state uncertainty.").
	Tools(tools).
	MaxTurns(4).
	Build()
if err != nil {
	return err
}
```

Builder methods use value semantics. A fluent call returns a new builder, and
`Build` defensively copies mutable configuration. The resulting Agent has no
mutable exported state and can create concurrent Runners; registered tool
implementations must still be safe for however the application invokes them.

`Build` rejects invalid static configuration before provider I/O, including a
nil model, duplicate tools, invalid turn or concurrency limits, impossible
active-tool/tool-choice combinations, and incomplete approval configuration.
`MaxTurns` defaults to `1` and must be positive.

## Run with overrides

`Agent.Runner()` snapshots the Agent defaults. Runner methods use the same
value-builder style and never mutate the Agent:

```go
result, err := assistant.Runner().
	Prompt("Find the weather in Hanoi").
	Temperature(0.2).
	ActiveTools("weather").
	ToolConcurrency(2).
	Run(ctx)
if err != nil {
	return err
}

fmt.Println(result.Text)
fmt.Println(result.Steps)
```

`ToolConcurrency(1)` is serial. Higher values allow tool bodies to finish in
parallel, while results and transcript messages are committed in model-call
order.

`Result` is the single aggregate for a run. It includes the full independently
owned `Transcript`, individual `Steps`, final text and reasoning, usage, tool
results, pending approvals, warnings, sources, generated files, finish reason,
provider metadata, and structured output. `GeneratedMessages()` derives the
continuation produced by the run without storing a second message history.

## Ordered messages and multimodal input

Every Runner starts with no input. Supply at least one valid message:

- `Messages(messages...)` replaces the complete ordered input sequence.
- `Message(message)` appends one full message.
- `Prompt(text)` appends one text user message.

Use `Messages` for full chat history, multimodal content, prior tool calls and
results, or approval-response history:

```go
result, err := assistant.Runner().
	Messages(
		aikit.SystemMessage("Use the supplied trip context."),
		aikit.Message{
			Role: aikit.RoleUser,
			Content: []aikit.ContentPart{
				aikit.TextPart("Describe this destination"),
				aikit.ImageURLPart("https://example.com/destination.jpg"),
			},
		},
	).
	Message(aikit.UserMessage("Then suggest what to pack.")).
	Run(ctx)
```

Messages and nested mutable values are cloned before execution. Invalid roles,
empty input, an empty `Prompt`, a tool result without its preceding tool call,
or an approval response without its preceding request fail synchronously with
`*agent.RunError`.

## Turn budget and early stopping

`MaxTurns` is one hard total budget for model calls. The initial request,
tool-result continuation, invalid-call retry, and structured-output retry all
consume it. `StopWhen` may end a run earlier, but cannot expand that budget.
There is no zero or negative sentinel for an unbounded run.

If another model call is required after the budget is spent, `Run` returns a
typed error and the partial result:

```go
result, err := assistant.Runner().
	Prompt("Research this and use tools when needed.").
	MaxTurns(2).
	Run(ctx)

var exhausted *agent.MaxTurnsError
if errors.As(err, &exhausted) {
	partial := result
	if exhausted.Result != nil {
		partial = exhausted.Result
	}
	// The last committed step keeps its real finish reason.
	return savePartial(partial)
}
if err != nil {
	return err
}
```

In streaming mode, committed events are yielded before the iterator returns
the same `*agent.MaxTurnsError`. A budget failure does not produce a successful
done event.

## Streaming

`Runner.Stream` returns a single-use, single-owner
`iter.Seq2[aikit.StepEvent, error]`:

```go
events, err := assistant.Runner().
	Prompt("Explain Go channels").
	Stream(ctx)
if err != nil {
	return err // validation failed before streaming began
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

Range the sequence exactly once. A second range returns
`agent.ErrStreamUsed`. Breaking the loop cancels the child context and releases
the provider and tool work owned by that run. Use `Run` when an aggregate is
needed; do not attempt to create multiple views over the event sequence.

Hooks registered with `Builder.Hook` or `Runner.Hook` execute synchronously in
registration order. A Runner hook is appended only for that invocation. Hook
actions can patch or stop a lifecycle stage; they do not create a second
execution or aggregation path.

See [Streaming](/core/streaming) for the difference between a direct model
stream and an Agent event stream, and [Tools](/core/tools) for tool construction
and approval behavior.
