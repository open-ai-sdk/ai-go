# Agent runtime goroutine and channel audit

Audited 2026-07-31 against the public `agent` runtime. Production searches:

```sh
rg -n '\bgo\s+(func|[A-Za-z_])|\.Go\(' agent -g '*.go' -g '!*_test.go'
rg -n '(<-|close\(|make\(chan)' agent -g '*.go' -g '!*_test.go'
```

## Package-owned goroutines

| Site | Ownership and shutdown | Panic boundary | Channel ownership |
|---|---|---|---|
| `agent/run.go: Stream` | Starts one run goroutine. It returns on normal completion, provider/control failure, or `ctx.Done()`; model consumption selects on cancellation. | The outermost defer uses `safego.Recover`, covering initialization, the loop, and deferred span cleanup. | Creates and exclusively closes the returned event channel. Its panic-error send selects on `ctx.Done()`. |
| `agent/step.go: executeToolCallsParallel` (`errgroup.Go`) | One bounded worker per valid parallel tool call. `errgroup.Wait` joins every started worker before the step returns. Queued workers check the group context before invoking user code. | Every worker has `safego.Recover`. A recovered control panic becomes a terminal run error after already-completed tool results are emitted. | Workers do not send to or close channels; each writes to one exclusive result slot. |

The former fatal-event `go drainStreamEvents(...)` site was removed. It could
range forever when a broken provider emitted an error and left its channel
open. Cancellation-aware receive loops now let the agent goroutine return
without creating an unjoinable drain goroutine.

## Event channel send and close sites

| Site | Audit result |
|---|---|
| `agent/run.go: Stream` panic handler | Sends the recovered error with `select { case ch <- event; case <-ctx.Done(): }`. |
| `agent/runtime.go: run.emit` | Rejects an already-cancelled context, then selects between the output send and `ctx.Done()`. Every normal runtime event is routed through this method. |
| `agent/run.go: Stream` close | A single outer defer closes the event channel for normal return, cancellation, or panic. No other code closes it. |

Provider channels are received in `agent/stream.go` and
`agent/structured_output.go` with a `ctx.Done()` select. The agent never closes
a provider-owned channel.

## User callback boundaries

Observer callbacks (`OnChunk`, `OnStepEnd`, `OnEnd`, `OnError`) run through
`run.safeObserver`; panics are recovered, logged when a logger exists, and
swallowed. Control callbacks (`PrepareStep`, `StopWhen`, repair, approval
policy/responder, tool execution, `ToModelOutput`) fail the run through the
outer or parallel-worker recovery boundary.

The compatibility façade in `ai/agent-options.go` still owns two callback-merge
goroutines while `ai.ToolLoopAgent` remains an `ai` façade. Both have individual
`safego.Recover` defers and are joined by a `sync.WaitGroup`; they own no
channels. A barrier regression test proves the callbacks start concurrently.

## Regression evidence

- `TestRun_CancelHungProvider_ClosesOutputAndReleasesProvider`
- `TestRun_ProviderErrorThenOpenChannel_DoesNotLeakDrain`
- `TestRun_PanicDuringInitialization_EmitsErrorAndClosesChannel`
- `TestObserverCallbackPanicsAreSwallowed`
- `TestControlCallbackPanicsFailRun`
- `TestToModelOutputPanicPreservesToolResultBeforeFailingRun`
- `TestMergeCallback_RunsBothCallbacksConcurrently`

