# Hooks

Hooks add ordered, synchronous policy and observation to one Agent run. They
are deliberately scoped to `agent`: direct `llm.NewCompletion` calls do not
enter this lifecycle. Implement only the small capability interfaces a hook
needs, or use `agent.HookFuncs` for an inline hook.

Register reusable policy on the Agent, and request-specific policy on a
Runner. Agent hooks always run first; Runner hooks are appended after them.

```go
audit := agent.HookFuncs{
	Name: "tool-audit",
	BeforeToolFunc: func(
		ctx context.Context,
		hc agent.HookContext,
		call aikit.ToolCallInfo,
	) (agent.ToolCallAction, error) {
		// Tool arguments can contain user data. Log an identifier, not Args.
		log.Printf("run=%s turn=%d tool=%s", hc.RunID, hc.Turn, call.Name)
		if call.Name == "delete_account" {
			return agent.ToolCallAction{
				Kind: agent.ToolCallSkip, Reason: "requires application approval",
			}, nil
		}
		return agent.ToolCallAction{Kind: agent.ToolCallRun}, nil
	},
}

assistant, err := agent.New(model).Hook(audit).Build()
if err != nil {
	return err
}
result, err := assistant.Runner().Hook(requestAudit).Prompt("...").Run(ctx)
```

Hooks are guardrails and observability, not an authorization boundary. A tool
must still authorize the caller inside its own handler.

## Lifecycle

The blocking and streaming drivers share completion, model-turn, tool-call,
and tool-result phases. Streaming adds delta observations. Events are
normalized Agent values, not provider-native response objects.

| Capability | Event | Action | Purpose |
| --- | --- | --- | --- |
| `BeforeCompletionHook` | prepared `llm.Request` | continue, patch, stop | Change only the current model request. |
| `CompletionResponseHook` | `CompletionResponseEvent` | continue, stop | Inspect the normalized response before tool dispatch/finalization. |
| `ModelTurnHook` | `ModelTurnEvent` | continue, retry, stop | Accept or reject a completed model turn. |
| `InvalidToolCallHook` | invalid call recovery input | delegate, fail, retry, repair, skip, stop | Recover malformed/unknown model calls. |
| `BeforeToolHook` | validated `aikit.ToolCallInfo` | run, rewrite, skip, stop | Apply call policy or replace JSON arguments. |
| `ToolResultHook` | `ToolResultEvent` | keep, rewrite, stop | Inspect raw facts and adjust only model presentation. |
| `AfterToolHook` | presentation result | keep, rewrite, stop | Compatibility post-tool presentation hook. |
| `TextDeltaHook` | `TextDeltaEvent` | continue, stop | Observe a streamed text fragment and aggregate. |
| `ToolCallDeltaHook` | `ToolCallDeltaEvent` | continue, stop | Observe a streamed tool-call argument fragment. |
| `StreamFinishHook` | `StreamFinishEvent` | continue, stop | Observe provider stream completion before turn acceptance. |
| `RunFinishedHook` | cloned `*agent.Result`, error | observe only | Observe every terminal path. |

`StreamEventHook` remains available for the normalized `aikit.StepEvent`
stream. Prefer the typed delta capabilities when you only need text, tool-call
fragments, or stream finish.

Every steering action stops later hooks for that event. Register audit hooks
before a hook that can stop, skip, or retry if the audit must see all attempts.
An error or panic in a steering hook fails the run as `*agent.HookError`, which
retains the original cause. `RunFinishedHook` is an observer: its panic is
recovered and logged rather than replacing the completed result.

## Completion request patches

`BeforeCompletion` sees a clone of the current prepared request. Returning
`CompletionPatch` changes this turn only; it does not mutate the Agent or the
next turn. Multiple patches compose in registration order:

- scalar settings, instructions, messages, and tool choice use the last value;
- provider-option maps shallow-merge, with later keys winning;
- `ActiveTools` intersects the currently available definitions, preserving
  definition order.

The next retry or continuation starts from the normal Runner configuration and
runs request hooks again. Use a hook scratchpad when a policy needs state
across turns.

## Retry a model turn

`ModelTurnHook` runs after a normalized response exists but before the runner
commits tool work or finalizes the turn. It may reject only a tool-free turn.
`agent.Repeat()` repeats with the same preceding history; `agent.RetryWithFeedback`
also appends the rejected assistant text and a user feedback message before the
next request. Both consume the existing `MaxTurns` budget.

This example limits feedback retries using the run-local scratchpad:

```go
retryIncomplete := agent.HookFuncs{
	Name: "retry-incomplete",
	ModelTurnFunc: func(
		ctx context.Context,
		hc agent.HookContext,
		event agent.ModelTurnEvent,
	) (agent.ModelTurnAction, error) {
		if event.HasToolCalls || !strings.Contains(event.Text, "RETRY") {
			return agent.ModelTurnAction{Kind: agent.ModelTurnContinue}, nil
		}

		attempt, _ := agent.ScratchpadGet[int](hc, "retry-attempt")
		attempt++
		hc.Store("retry-attempt", attempt)
		if attempt > 2 {
			return agent.ModelTurnAction{
				Kind: agent.ModelTurnStop, Reason: "retry limit reached",
			}, nil
		}
		return agent.ModelTurnAction{
			Kind: agent.ModelTurnRetry,
			Retry: agent.RetryWithFeedback("Return a complete answer."),
		}, nil
	},
}
```

The Runner charges each retry as a model call, so it can end with the normal
typed `*agent.MaxTurnsError`. A tool-call turn cannot be retried at this
boundary because its calls must be dispatched or rejected deliberately. Use a
`BeforeToolHook`, `InvalidToolCallHook`, approval policy, or `ToolResultHook`
instead.

## Streaming, deltas, and buffering

`TextDeltaHook`, `ToolCallDeltaHook`, and `StreamFinishHook` run only while a
stream is consumed. `TextDeltaEvent` contains the new fragment and text
accumulated for the current turn; `ToolCallDeltaEvent` contains the tool ID,
index, name when available, and JSON argument fragment.

High-frequency hooks advertise their needs with `InterestedHook.HookInterests`.
`HookFuncs` derives this mask from non-nil `TextDeltaFunc`,
`ToolCallDeltaFunc`, and `StreamFinishFunc`, so no extra configuration is
needed for the adapter. A reusable direct implementation should return only
the `HookInterest` bits it handles.

If a run has a `ModelTurnHook`, the runner buffers the current turn's deltas
until the hook accepts it. This applies to both `Run`'s result reduction and
`Runner.Stream`'s public iterator. On retry it discards that turn; no rejected
text or tool-call fragments enter a Result, the public iterator, callbacks, or
AI SDK v7 wire stream. This preserves the frozen UI protocol at the cost of
one turn of latency and memory. Accepted events retain their original order.

## Raw execution and model presentation

Tool execution first creates a canonical `aikit.ToolResult`: output content,
one disposition, a safe model-facing error when needed, and host-only
metadata. `ToolResultHook.OnToolResult` receives both views:

- `event.Raw` is a cloned immutable execution fact for audit/policy code.
- `event.Presentation` is the running model-facing result. A rewrite becomes
  the next hook's presentation and the content recorded for the continuation.

Rewriting the presentation cannot change the raw facts or reveal host-only
metadata. The older `AfterToolHook` sees only the current presentation and is
kept for pre-release compatibility. Use the richer hook for new policy code.

```go
redact := agent.HookFuncs{
	Name: "redact-tool-presentation",
	ToolResultFunc: func(
		ctx context.Context,
		hc agent.HookContext,
		event agent.ToolResultEvent,
	) (agent.ToolResultAction, error) {
		if event.Raw.Name != "customer_lookup" {
			return agent.ToolResultAction{Kind: agent.ToolResultKeep}, nil
		}
		shown := event.Presentation
		shown.Output = "Customer record retrieved."
		shown.Content = []aikit.ToolResultContent{
			aikit.TextToolResultContent(shown.Output),
		}
		return agent.ToolResultAction{Kind: agent.ToolResultRewrite, Result: shown}, nil
	},
}
```

Never turn arbitrary Go error messages into a presentation. Let the runtime
derive safe output with `tool.Details`, or return a deliberately safe
`tool.DetailedError`; see [Tools](/core/tools#errors-and-dispositions).

## Scratchpad and concurrency

`HookContext` is stable for a run. It carries `RunID`, `AgentID`, whether the
surface is streaming, and the one-based current `Turn`. Its scratchpad belongs
only to this invocation—never to the Agent—and is safe for parallel tool
callbacks.

Use `Store`, `Load`, `Delete`, and `Update` for untyped state. The generic
helpers make typed state concise:

```go
agent.ScratchpadUpdate[int](hc, "tool-count", func(value int, present bool) (int, bool) {
	return value + 1, true
})
```

`Update` is atomic, but its callback runs while the scratchpad is locked. Keep
that callback short; do not block or re-enter the same scratchpad from it.
Avoid storing credentials, cancellation objects, or raw tool arguments there.
Use the handler context for deadlines/cancellation and a tool's own private
state for authorization.

## Choose the smallest hook

Use a capability interface directly for reusable components with one concern;
use `HookFuncs` for a local composition. Hooks run synchronously, so expensive
network I/O or slow logging directly adds to model or tool latency. Keep them
deterministic, return explicit actions, and keep provider-native response
inspection in provider-specific code rather than the normalized hook
lifecycle.

Continue with [Tools](/core/tools) for typed authoring, rich results, safe
errors, invocation context, and MCP snapshots.
