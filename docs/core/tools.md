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
