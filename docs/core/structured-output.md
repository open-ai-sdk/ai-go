# Structured output

For one direct model call, `ai.CompleteObject[T]` derives a JSON Schema from
the exported fields of `T` and unmarshals the normalized response. `T` must be
a struct; nested structs and slices are supported.

```go
type Answer struct {
  Summary string   `json:"summary"`
  Sources []string `json:"sources"`
}

request := llm.NewRequest("What is a Go interface?").
  Instructions("Return a short factual summary.").
  Build()

result, err := ai.CompleteObject[Answer](ctx, model, request)
if err != nil { return err }

fmt.Println(result.Object.Summary)
```

The direct result includes the typed Object and normalized completion response,
including usage, finish reason, warnings, and provider metadata.

For structured output during multi-turn Agent Runner execution, attach an explicit
`llm.OutputSchema` to the Agent Builder or one Runner:

```go
schema, err := tool.Schema[Answer]()
if err != nil { return err }

result, err := assistant.Runner().
  Prompt("Research Go interfaces, then return the requested object.").
  Output(llm.OutputSchema{Type: "object", Schema: schema}).
  MaxTurns(4).
  Run(ctx)
if err != nil { return err }

var answer Answer
if err := json.Unmarshal(result.StructuredOutput, &answer); err != nil {
  return err
}
```

The final structured-output model call consumes the same positive `MaxTurns`
budget as every other model call. If that call is required after the budget is
spent, `*agent.MaxTurnsError` carries the partial Result and full Transcript.
The runtime does not retry decode or schema failures; decode, empty-output, and
schema validation errors are classifiable as `*agent.StructuredOutputError`.

See [Agent Runner](/core/agent-runner) for how this final model call participates
in the run's shared turn budget and Result.
