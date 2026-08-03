# Typed tools

Create a tool from a typed Go function, then place it in a `tool.Set`. The SDK derives and validates the input JSON Schema when the tool is constructed.

```go
weather, err := tool.New("get_weather", "Get weather for a city",
  func(ctx context.Context, input struct {
    City string `json:"city" description:"City name"`
  }) (struct {
    Summary string `json:"summary"`
  }, error) {
    return struct { Summary string `json:"summary"` }{
      Summary: "Weather lookup for " + input.City,
    }, nil
  },
)
if err != nil { return err }

tools, err := tool.NewSet(weather)
if err != nil { return err }
```

Attach `tools` to `GenerateTextRequest.Tools` and set `StopWhen: ai.IsStepCount(n)` when the model needs a follow-up turn after a tool result. `tool.NewSet` rejects duplicate names. Dynamic tools and MCP-discovered tools use the same shared `tool.Set` runtime, so tool execution failures remain typed across sources.

Tool output is literal text by default. When a provider can consume richer
results, construct ordered typed content explicitly:

```go
part := ai.RichToolResultPart(
  call.ID,
  call.Name,
  ai.TextToolResultContent("chart generated"),
  ai.JSONToolResultContent(json.RawMessage(`{"rows": 12}`)),
  ai.ImageToolResultContent(png, "image/png"),
)
```

JSON-looking strings remain text. Use `JSONToolResultContent` or
`ParseToolResultJSON` when JSON semantics are intended. Rich content is cloned
when messages and result snapshots are built, so caller mutation cannot alter
stored history.
