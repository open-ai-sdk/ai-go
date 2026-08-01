# Structured output

`ai.GenerateObject[T]` derives a JSON Schema from the exported fields of `T`, asks the model for an object matching that schema, and unmarshals the result for you. `T` must be a struct; nested structs and slices are supported.

```go
type Answer struct {
  Summary string   `json:"summary"`
  Sources []string `json:"sources"`
}

result, err := ai.GenerateObject[Answer](ctx, ai.GenerateObjectRequest{
  Model:        model,
  Instructions: "Return a short factual summary.",
  Messages:     []ai.Message{ai.UserMessage("What is a Go interface?")},
})
if err != nil { return err }

fmt.Println(result.Object.Summary)
```

The result includes typed `Object`, usage, finish reason, warnings, and provider metadata. For a runtime-defined schema or raw structured JSON, use `GenerateText` with an output schema instead.
