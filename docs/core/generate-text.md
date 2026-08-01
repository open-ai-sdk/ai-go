# Generate text

`ai.GenerateText` produces an aggregated result. Pass an explicit `GenerateTextRequest` when you need full control over the request, tool loop, callbacks, or provider settings.

```go
result, err := ai.GenerateText(ctx, ai.GenerateTextRequest{
  Model:        model,
  Instructions: "Be concise and cite uncertainty.",
  Messages:     []ai.Message{ai.UserMessage("Summarize this report")},
})
if err != nil { return err }

fmt.Println(result.Text)
fmt.Println(result.Usage, result.FinishReason)
```

The result also exposes reasoning, completed steps, tool results, warnings, sources, generated files, provider metadata, and continuation messages. `GenerateText` defaults to one model step. A tool call within that step can run, but another model call requires an explicit stop condition such as `ai.IsStepCount(3)`.

Use `ai.NewRequest(model, prompt)` when a fluent request builder better fits your application. Typed provider option structs are preferred; map-based provider options are intended for values decoded from JSON and validated strictly.
