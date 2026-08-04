# Streaming

ai-go has two stream vocabularies, and keeping them separate is deliberate.

| Level | Event type | Covers |
|---|---|---|
| One direct model call | `aikit.StreamEvent` | assistant content: text, reasoning, tool-call deltas, sources, files, usage, finish |
| One multi-turn agent run | `aikit.StepEvent` | everything above plus tool execution, tool results, approvals, step boundaries, structured output, and the run's terminator |

An agent run contains many model calls, so collapsing the two would either lose
step structure or force every direct-completion consumer to handle events that
can never occur. Every streaming entrypoint returns an
`iter.Seq2[E, error]` over one of them.

## Migrating from `CompletionRequestBuilder.Stream`

`CompletionRequestBuilder.Stream` has been **removed**. `StreamSend` replaces
it and is a strict superset: the same events, plus the aggregated
`*CompletionResponse` after the stream ends.

Before:

```go
events, err := llm.NewCompletion(model, "Explain Go channels").Stream(ctx)
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

After:

```go
stream, err := llm.NewCompletion(model, "Explain Go channels").StreamSend(ctx)
if err != nil {
	return err
}
for event, err := range stream.Events() {
	if err != nil {
		return err
	}
	if event.Type == aikit.StreamEventTextDelta {
		fmt.Print(event.TextDelta)
	}
}
return nil
```

Three differences to note while migrating:

- A provider error arrives through the **error half** of the sequence, not as an
  `aikit.StreamEventError` value. `Encode`-style type switches on
  `StreamEventError` become an `if err != nil`.
- The provider call starts on the first pull, not at `StreamSend`. Request
  validation still fails immediately.
- After the range, `stream.Response()` gives you what `Send` would have
  returned.

If you genuinely want the raw channel, `llm.Model.Stream` is public and
unchanged — see [the provider boundary](#why-model-stream-is-still-a-channel).

## Direct model stream

Use it when your application owns tool execution and continuation:

```go
stream, err := llm.NewCompletion(model, "Explain Go channels").
	Instructions("Answer concisely.").
	StreamSend(ctx)
if err != nil {
	return err // synchronous request validation
}

for event, err := range stream.Events() {
	if err != nil {
		return err
	}
	if event.Type == aikit.StreamEventTextDelta {
		fmt.Print(event.TextDelta)
	}
}

response, err := stream.Response()
if err != nil {
	return err
}
fmt.Println(response.Usage.TotalTokens)
return nil
```

`Response()` returns everything `Send` returns — text, message content parts,
sources, files, usage, message ID, finish reason, warnings, provider metadata —
without a second model call. Direct completions surface tool calls but never
execute them.

### Prompt and chat

`StreamPrompt` and `StreamChat` are the streaming twins of `Prompt` and `Chat`:

```go
stream, err := llm.StreamChat(ctx, model, "And in one sentence?", history...)
if err != nil {
	return err
}
defer stream.Close()

for event, err := range stream.Events() {
	if err != nil {
		return err
	}
	if event.Type == aikit.StreamEventTextDelta {
		fmt.Print(event.TextDelta)
	}
}
return nil
```

### Shape the request first

`StreamCompletion` returns a builder rather than a stream, so settings, tools,
and provider options can be applied before anything is sent:

```go
builder, err := llm.Streaming(model).StreamCompletion(ctx, "Summarize", history...)
if err != nil {
	return err
}
stream, err := builder.Temperature(0.2).MaxTokens(256).StreamSend(ctx)
if err != nil {
	return err
}
for _, err := range stream.Events() {
	if err != nil {
		return err
	}
}
response, err := stream.Response()
if err != nil {
	return err
}
fmt.Println(response.Text)
return nil
```

## Agent stream

Build a reusable Agent and stream one invocation from its Runner:

```go
events, err := assistant.Runner().
	Prompt("Explain Go channels").
	MaxTurns(4).
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
return nil
```

`Runner.Stream` returns an `iter.Seq2[aikit.StepEvent, error]` with one owner.
Range it once. It is not a fan-out result and has no secondary text stream, late
event view, or consume/drain operation.

### Streaming and the aggregate together

`Runner.StreamRun` gives you both. The aggregate costs nothing extra — the same
reducer already ran behind `Stream` and was thrown away:

```go
stream, err := assistant.Runner().
	Prompt("Explain Go channels").
	MaxTurns(4).
	StreamRun(ctx)
if err != nil {
	return err
}

for event, err := range stream.Events() {
	if err != nil {
		return err
	}
	if event.Type == aikit.StepEventTextDelta {
		fmt.Print(event.TextDelta)
	}
}

result, err := stream.Result()
if err != nil {
	return err
}
fmt.Println(len(result.Steps), result.Usage.TotalTokens)
return nil
```

`Agent.StreamPrompt` and `Agent.StreamChat` are the shorthand forms:

```go
stream, err := assistant.StreamChat(ctx, "And in one sentence?", history...)
if err != nil {
	return err
}
for event, err := range stream.Events() {
	if err != nil {
		return err
	}
	if event.Type == aikit.StepEventTextDelta {
		fmt.Print(event.TextDelta)
	}
}
_, err = stream.Result()
return err
```

`Run`, `Stream`, and `StreamRun` use the same driver and state reducer. For the
same model events they agree on transcript order, usage, tool results, finish
reason, warnings, sources, files, provider metadata, and terminal errors.

One documented exception: `HookContext.Streaming` is `false` under `Run` and
`true` under `Stream` and `StreamRun`. A hook that branches on it will make the
two paths diverge, by design.

## Terminal state

`llm.StreamingResponse` and `agent.StepStream` follow the same rules.

**Single-goroutine ownership.** `iter.Seq2` bodies run in the consumer's
goroutine, and that is where the aggregate is written. Reading `Response()`,
`Result()`, or `State()` from another goroutine is a data race, not a supported
call. `State()` exists so a same-goroutine caller can ask whether draining
finished without pretending cross-goroutine reads are safe.

**Drain before reading.** `Response()` and `Result()` return
`ErrStreamNotDrained` while the sequence has not ended.

**Single use.** A second range yields `ErrStreamUsed` — `llm.ErrStreamUsed` or
`agent.ErrStreamUsed` — and leaves the first range's aggregate intact.

**Three states.**

| `StreamState` | Meaning |
|---|---|
| `StreamNotDrained` | never ranged, or still ranging |
| `StreamCompleted` | ranged to the end, or stopped on the terminal event |
| `StreamAborted` | stopped before the terminal event, cancelled, or failed |

**Break position matters.** Stopping on the terminal event — `StreamEventFinish`
for a model call, `StepEventDone` for a run — is a normal early exit: the state
is `StreamCompleted` and the error is nil. Stopping before it is an abort: you
get the partial aggregate *and* a cancellation error, never a nil aggregate with
a nil error. Nothing synthesizes `context.Canceled` for a stream that actually
finished.

`StreamCompleted` reports that nothing failed. It does not promise every event
was seen, and the difference is sharper than it looks at the model layer:

- `agent.StepEventDone` really is the last event — the run driver emits nothing
  after it — so breaking there gives a whole `*agent.Result`.
- `aikit.StreamEventFinish` is **not** always last. OpenAI-compatible endpoints
  report usage on a trailing chunk that carries no choices, which arrives after
  the chunk carrying the finish reason; the Gemini native decoder does the same.
  Breaking on finish therefore drops those token counts, silently and with a nil
  error.

Range to the end when the aggregate matters. Breaking early is for consumers
that want the text and are done.

**Release.** Breaking cancels the underlying call. For a response you obtain and
then decide not to range, `llm.StreamingResponse.Close` releases it; because the
provider call is lazy, an unranged response has opened nothing to begin with.
`Close` on a drained response is a no-op.

## Writing code over either layer

Three interfaces in `aikit` are implemented at both layers, so a helper can take
either:

| Interface | Model layer | Agent layer |
|---|---|---|
| `StreamingPrompt[E, S]` | `llm.ModelStream` | `*agent.Agent` |
| `StreamingChat[E, S]` | `llm.ModelStream` | `*agent.Agent` |
| `StreamingCompletion[B]` | `llm.ModelStream` → `llm.CompletionRequestBuilder` | `*agent.Agent` → `agent.Runner` |

```go
stream, err := source.StreamPrompt(ctx, prompt)
if err != nil {
	return stream, err
}
for _, err := range stream.Events() {
	if err != nil {
		return stream, err
	}
}
return stream, nil
```

`S` is the concrete stream type rather than the `Stream[E]` interface because Go
has no covariant return types: an interface method declared to return `Stream[E]`
can only be satisfied by a method whose return type is literally `Stream[E]`,
which erases the concrete type and forces every caller to type-assert before it
can reach `Response()` or `Result()`.

The cost is real and worth knowing before you write such a helper: **inference
is partial**. The implementer is inferred; `E` and `S` are not, so callers name
them:

```go
_, err := docAcceptAnythingStreamable[aikit.StreamEvent, *llm.StreamingResponse](
	ctx, llm.Streaming(model), "Explain Go channels",
)
return err
```

Direct method calls pay nothing. Use the free functions —
`llm.StreamPrompt`, `llm.StreamChat` — for a single call, and the interfaces
only for code that must accept anything streamable.

## Why `Model.Stream` is still a channel

`llm.Model.Stream` returns `<-chan aikit.StreamEvent` and will keep doing so. It
is the **provider-facing** contract: every third-party provider implements it,
and widening it would break all of them. The **consumer-facing** surface —
`StreamSend`, `StreamPrompt`, `StreamChat`, `Runner.Stream`, `Runner.StreamRun`
— is uniformly `iter.Seq2`. The boundary is between who implements and who
consumes, not an inconsistency.

Cancelling the context is how a stream is released. Implementations must close
the channel when the context is done; consumers abandon whatever is left rather
than draining, so a provider that ignores cancellation leaks only its own
goroutine and never blocks the caller.

## Event reference

`aikit.StreamEventType` — one direct model call:

| Constant | Carries |
|---|---|
| `StreamEventTextDelta` | `TextDelta`, `ThoughtSignature` |
| `StreamEventReasoningDelta` | `TextDelta`, `ThoughtSignature` |
| `StreamEventToolCallDelta` | `ToolCallIndex`, `ToolCallID`, `ToolCallName`, `ToolCallArgsDelta` |
| `StreamEventUsage` | `Usage` — may be reported across several events, and may arrive after finish |
| `StreamEventSource` | `Source` |
| `StreamEventFileDelta` | `FileData`, `FileMediaType` |
| `StreamEventFinish` | `MessageID`, `FinishReason`, `RawFinishReason`, `ProviderMetadata`, `Warnings` |
| `StreamEventError` | `Error` — surfaced through the sequence's error half, not as an event |

`aikit.StepEventType` — one multi-turn agent run:

| Constant | Carries |
|---|---|
| `StepEventStepStart` | `StepNumber` |
| `StepEventStepEnd` | `StepNumber`, `MessageID`, `FinishReason`, `RawFinishReason`, `Warnings` |
| `StepEventTextDelta` | `TextDelta` |
| `StepEventReasoningDelta` | `ReasoningDelta`, `ThoughtSignature` |
| `StepEventToolCallStart` | first delta for a new tool call |
| `StepEventToolCallDelta` | a later argument fragment |
| `StepEventToolCallReady` | arguments complete, about to execute |
| `StepEventToolCallInvalid` | arguments failed to parse; execution skipped |
| `StepEventToolResult` | `ToolResult` |
| `StepEventToolApprovalRequest` | `ApprovalID`, `ApprovalSignature` |
| `StepEventToolOutputDenied` | an approval was refused |
| `StepEventUsage` | `Usage`, merged within the step |
| `StepEventSource` | `Source` |
| `StepEventFileDelta` | `FileData`, `FileMediaType` |
| `StepEventStructuredOutput` | `StructuredOutput` |
| `StepEventDone` | the run's terminator |
| `StepEventError` | `Error` — surfaced through the sequence's error half |

Both tables are checked against the source enums by a test, so a new event type
cannot ship undocumented.

## Errors

Configuration and request-validation errors are returned before you get an
iterator. Runtime failures are yielded from the iterator, after every previously
committed event.

`*agent.MaxTurnsError` carries the partial `Result` and full `Transcript` when
another model call is required after `MaxTurns` is exhausted; that terminal path
does not yield `aikit.StepEventDone`. `StepStream.Result()` returns the same
partial result alongside the error.

Direct completions preserve `Send`'s failure shape: aggregation failures are
`*llm.CompletionError` with `Operation == "collect"`, and the partial response is
returned with the error rather than discarded.

## Deliberate non-parity with rig

ai-go's streaming design is modelled on rig, and three differences are choices
rather than gaps:

- **No `PauseControl`.** An `iter.Seq2` consumer already controls the pull rate;
  not consuming *is* pausing.
- **No `Unknown` passthrough event.** Unrecognized provider content becomes an
  `aikit.Warning` on the finish event instead of an event type consumers must
  handle.
- **The aggregate is promised, provider extras are not.** Rig's
  `StreamingCompletionResponse` guarantees only the aggregated `choice`
  post-drain; its `response` may be absent. `Response()` and `Result()` make the
  same promise: the aggregate is always there, provider-supplied extras are
  never invented.

One correction to rig's own documentation, since it is a natural next thing to
read: the streaming page there presents `Prompt`/`Chat`/`Completion` and
`StreamingPrompt`/`StreamingChat`/`StreamingCompletion` as six traits. In rig's
source there are four, all at the **agent** level —
`Prompt` and `Chat` in `rig-agent/src/completion.rs`, `StreamingPrompt` and
`StreamingChat` in `rig-agent/src/streaming.rs`. No `Completion` or
`StreamingCompletion` trait exists; the model layer is one trait,
`CompletionModel`, with `completion()` and `stream()` methods. ai-go exposes the
streaming interfaces at **both** layers, which is a deliberate superset.

See [Agent Runner](/core/agent-runner) for the complete per-invocation lifecycle
and [Completions](/core/completions) for the non-streaming twins.
