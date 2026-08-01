# Stream responses

`ai.StreamText` returns a stream result with iterator views. Handle text deltas as they arrive:

```go
stream := ai.StreamText(ctx, ai.GenerateTextRequest{
  Model:    model,
  Messages: []ai.Message{ai.UserMessage("Explain Go channels")},
})

for event, err := range stream.Events() {
  if err != nil { return err }
  if event.Type == ai.StepEventTextDelta {
    fmt.Print(event.TextDelta)
  }
}
```

Cancel the context supplied to `StreamText` to stop the underlying run. Leaving an iterator alone does not cancel it: another view or `Consume` call may still be reading the same result. Use the same request fields as `GenerateText` for tools, callbacks, retry policy, tracing, and per-provider settings.
