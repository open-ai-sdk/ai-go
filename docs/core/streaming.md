# Streaming

ai-go exposes two deliberately different stream levels. A direct completion
streams one provider call as `aikit.StreamEvent` values. An Agent Runner
streams the complete multi-turn execution as `aikit.StepEvent` values,
including tool calls, tool results, approvals, usage, and step boundaries.

## Direct model stream

Use `llm.NewCompletion(...).Stream` when the application owns tool execution
and continuation:

```go
events, err := llm.NewCompletion(model, "Explain Go channels").
	Instructions("Answer concisely.").
	Stream(ctx)
if err != nil {
	return err
}

for event := range events {
	if event.Type == aikit.StreamEventError {
		return event.Error
	}
	if event.Type == aikit.StreamEventTextDelta {
		fmt.Print(event.TextDelta)
	}
}
```

The returned channel belongs to that model call. Cancel the supplied context
to stop it. Direct completions expose tool calls but do not execute them.

## Agent stream

Build a reusable Agent and stream one invocation from its Runner:

```go
assistant, err := agent.New(model).
	Instructions("Answer concisely.").
	Tools(tools).
	MaxTurns(4).
	Build()
if err != nil {
	return err
}

events, err := assistant.Runner().
	Prompt("Explain Go channels").
	Stream(ctx)
if err != nil {
	return err // synchronous Runner validation
}

for event, err := range events {
	if err != nil {
		return err
	}
	switch event.Type {
	case aikit.StepEventTextDelta:
		fmt.Print(event.TextDelta)
	case aikit.StepEventToolResult:
		log.Printf("tool %s completed", event.ToolCallName)
	}
}
```

`Runner.Stream` returns an `iter.Seq2[aikit.StepEvent, error]` with one owner.
Range it once. It is not a fan-out result and has no secondary text stream,
late event view, or consume/drain operation. Call `Runner.Run` instead when you
need an aggregated `*agent.Result`.

Breaking iteration cancels the child context and drains the underlying runtime
so provider and tool work is released. Explicitly cancelling the parent
context has the same terminal effect. A second attempt to range the same
sequence returns `agent.ErrStreamUsed`.

Configuration errors are returned by `Stream` before it returns an iterator.
Runtime failures are yielded from the iterator after all previously committed
events. In particular, `*agent.MaxTurnsError` carries the partial Result and
full Transcript when another model call is required after `MaxTurns` is
exhausted; that terminal path does not yield `aikit.StepEventDone`.

`Run` and `Stream` use the same driver and state reducer. For the same model
events they agree on transcript order, usage, tool results, finish reason,
warnings, sources, files, provider metadata, and terminal errors.
