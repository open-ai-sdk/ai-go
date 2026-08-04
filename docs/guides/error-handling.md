# Error handling

ai-go keeps stable error classification separate from provider-specific
details. Use `errors.As` to inspect context and `errors.Is` for sentinel or
kind matching; avoid parsing error strings.

## Direct completions

`*llm.CompletionError` classifies failures as transport, JSON decode, invalid
request, invalid response, or provider error. Its `Unwrap` method retains the
original cause, including `context.Canceled`, transport errors, and
provider API errors:

```go
response, err := llm.NewCompletion(model, prompt).Send(ctx)
if err != nil {
  var completionErr *llm.CompletionError
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

## Agent Runner

Agent construction and invocation validation fail synchronously. `Build`
returns `*agent.BuildError`; `Runner.Run` and `Runner.Stream` return
`*agent.RunError` before provider I/O when invocation input is invalid. Both
retain their underlying causes through `Unwrap`.

```go
assistant, err := agent.New(model).
  Tools(tools).
  Build()
if err != nil {
  return err
}

result, err := assistant.Runner().
  Prompt(prompt).
  MaxTurns(4).
  Run(ctx)
if err == nil {
  return use(result)
}

var exhausted *agent.MaxTurnsError
if errors.As(err, &exhausted) {
  // The partial Result and full Transcript are independently owned.
  return savePartial(exhausted.Result)
}
return err
```

Runtime errors preserve partial committed state where meaningful. In
particular, `*agent.MaxTurnsError` means another model call was required after
the positive total-turn budget was exhausted. Tool causes remain classifiable
as `*tool.InputError`, `*tool.ExecutionError`, `*tool.DeniedError`, or
`*tool.NoSuchToolError` through `errors.As`.

## Structured output

`*llm.StructuredOutputError` distinguishes prompt failure, JSON decoding, empty
output, and schema validation; it is re-exported from `agent` and `ai` for
source compatibility. `CompleteObject`, `RunObject`, and `Extractor` all use
the same parse → validate → decode pipeline. An `Extractor` that exhausts its
configured retries returns `*agent.ExtractionError`, including attempts and
usage accumulated across failed attempts. Keep the normalized response or
partial Agent Result for diagnostics, and never log raw provider payloads
without applying the application's data-handling policy.

See [Agent Runner](/core/agent-runner) for the full Result, transcript, and
streaming terminal-error contract.
