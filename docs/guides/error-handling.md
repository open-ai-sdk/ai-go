# Error handling

ai-go keeps stable error classification separate from provider-specific
details. Use `errors.As` to inspect context and `errors.Is` for sentinel or
kind matching; avoid parsing error strings.

## Direct completions

`*ai.CompletionError` classifies failures as transport, JSON decode, invalid
request, invalid response, or provider error. Its `Unwrap` method retains the
original cause, including `context.Canceled`, transport errors, and
`*ai.APIError`:

```go
response, err := ai.NewCompletion(model, prompt).Send(ctx)
if err != nil {
  var completionErr *ai.CompletionError
  if errors.As(err, &completionErr) && completionErr.Retryable() {
    // Retry according to the application's backoff and idempotency policy.
  }
  if response != nil {
    log.Printf("partial text: %q", response.Text)
  }
  return err
}
```

Cancellation, invalid requests, and malformed responses are not retryable.
Provider HTTP retryability remains available through the wrapped API error.

## Agent prompts

`ai.GenerateText` wraps failed runs in `*ai.PromptError`. Its kind separates
completion, cancellation, unknown/disallowed tool, tool execution, maximum
turns, and memory/history failures. `Partial` and `History` are deep snapshots,
so they remain safe to inspect or persist after the request returns.

```go
result, err := ai.GenerateText(ctx, request)
if err != nil {
  var promptErr *ai.PromptError
  if errors.As(err, &promptErr) && promptErr.Partial != nil {
    saveDraft(promptErr.Partial.Text, promptErr.History)
  }
  return err
}
```

Tool and completion causes remain discoverable through the prompt wrapper's
unwrap chain.

## Structured output

`*ai.StructuredOutputError` distinguishes prompt failure, JSON decoding, empty
output, and schema validation. Typed-output helpers may additionally report a
deserialization failure after a valid model response. Keep the normalized
response or partial agent result for diagnostics, and never log raw provider
payloads without applying the application's data-handling policy.
