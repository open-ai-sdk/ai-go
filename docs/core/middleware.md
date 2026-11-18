# Middleware

Middleware wraps an `llm.Model` and returns another `llm.Model`, so it works
anywhere a model does: direct completions, agents, and runners. The core does
no retrying or stall detection on its own; both are opt-in and have no default
durations or counts.

```go
model := llm.WithRetry(
  llm.WithStallTimeout(base, 2*time.Minute),
  llm.RetryOptions{MaxRetries: 1, InitialDelay: 500 * time.Millisecond},
)
```

Middleware changes `Stream` only. A wrapped model does not expose the inner
model's `Complete`, so direct completions aggregate the wrapped stream and go
through the middleware. `NativeSchemaSupport` is forwarded when the inner model
has it.

## `WithStallTimeout(model, d)`

Cancels the stream and ends it with `*llm.StallTimeoutError` when no decoded
event arrives for `d`. The timer resets only on events the provider decoder
emits. SSE keepalive comments and blank lines never produce events, so they
cannot hold a hung generation open (unlike a byte-level `ChunkTimeout`). It also
covers the wait for the first event, including the time `Stream` takes to
return. `d <= 0` disables it.

## `WithRetry(model, RetryOptions)`

By default, retries are limited to streams that have produced no content:

- an error returned by `Stream`, or an error event before the first content
  event (text, reasoning, tool-call delta, source, file), is retried;
- once a content event has been forwarded the attempt is committed and later
  errors pass through unretried, so callers never see duplicated output;
- provider rejections (nonretryable HTTP 4xx, or a status-less
  `invalid_request_error`) are never retried, even when a custom `Retryable`
  returns true;
- waiting and every attempt honour the caller's context.

`RetryOptions` fields: `MaxRetries` (0 disables), `InitialDelay`, `MaxDelay`
(also caps a provider `Retry-After`), `BackoffFactor`, `Retryable`, `OnRetry`,
`CommitOn`, and `EmitRetryEvents`.
The default classifier retries typed transient failures: retryable HTTP
statuses, status-less `server_error` events, network timeouts, connection
resets, truncated responses, and `StallTimeoutError`. Provider error text is
never inspected.

`CommitOn` optionally replaces the default content commitment rule. For example,
an application may commit only on nonempty text while still showing provisional
reasoning. Finish always commits, regardless of the callback.

`EmitRetryEvents: true` exposes pre-commit events immediately rather than
buffering and discarding a failed attempt. It returns a channel immediately:
startup failures and exhausted retries arrive as `StreamEventError` events.
Before retrying, it cancels and joins the old producer, sends `StreamEventRetry`
with the informational triggering `Error`, then calls `OnRetry` and waits.
Use the ordered boundary to reset provisional consumer state; do not treat its
cause as a terminal failure or mutate consumer state concurrently from `OnRetry`.
Neither option relaxes the provider-rejection guard. Zero values preserve the
original buffered-channel behavior.

## Explicit transport diagnostics

`transport.WithResponseObserver(ctx, func(transport.ResponseMetadata))`
observes a snapshot of status and response headers synchronously before body
decoding. Observers compose; modifying one observer's header copy does not
modify the response or another observer's copy.

`transport.WithErrorBodyCapture(ctx)` opts into retaining bounded provider
error bodies in `aikit.APIError.RawBody`. This diagnostic data is untrusted
and can contain secrets. It is absent by default, never included in
`APIError.Error()`, and must be redacted by the application before logging.
Responses SSE error capture follows the same opt-in.
