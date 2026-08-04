# Tools

Create typed tools with `tool.New` and register them in an immutable
duplicate-rejecting `tool.Set`.

```go
weather, err := tool.New("weather", "Look up weather", func(ctx context.Context, input struct {
  City string `json:"city"`
}) (tool.ExecutionResult, error) {
  output, err := tool.Content(
    aikit.TextToolResultContent("Sunny in " + input.City),
    aikit.JSONToolResultContent(json.RawMessage(`{"celsius":31}`)),
  )
  return tool.ExecutionResult{Output: output, Metadata: map[string]any{"cache": "hit"}}, err
})
if err != nil { return err }
tools, err := tool.NewSet(weather)
```

`string` results from `tool.New` are literal text. Other ordinary Go values
are JSON. Use `tool.JSON` for explicit JSON and `tool.Image` for images;
`tool.Content` preserves the exact order of text, JSON, and image parts.
JSON-looking text is never guessed to be JSON. `Metadata` is host-only and is
not sent to providers, logs, traces, or SSE by default.

`Invokable.Invoke` remains the released raw-JSON compatibility API.
`ResultInvokable.InvokeResult` is additive and is preferred by `tool.Set`.
Legacy valid bytes become JSON output; non-JSON bytes become literal text.
`Set.Snapshot` keeps definitions and exact invokers together, so later source
mutation cannot change a run.

Errors remain normal Go errors: `errors.Is` and `errors.As` work for input,
execution, denial, and unknown-tool classifications. `tool.Details` provides
a safe model-facing kind and output; arbitrary operator cause text stays in
the error chain and is never used as model feedback.

Tool invocation contexts retain cancellation/deadlines and include the named
`ToolsContext`, cloned `RuntimeContext`, and tool-call ID. Read them through
`tool.ToolContextFrom`, `tool.RuntimeContextFrom`, and
`tool.ToolCallIDFromContext`. Parallel calls receive isolated map snapshots.

MCP discovery uses the same immutable registry. Use
`mcp.ToolSetFromClientsContext` for cancellable, paginated one-shot discovery;
there is no live-refresh registry. See [MCP](/integrations/mcp) and
[Hooks](/core/hooks) for remote content and execution steering.
