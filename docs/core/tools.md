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

Attach the immutable registry to an Agent Builder. `MaxTurns` is the positive
total model-call budget and must allow any follow-up model call after a tool
result:

```go
assistant, err := agent.New(model).
  Tools(tools).
  MaxTurns(4).
  Build()
if err != nil { return err }

result, err := assistant.Runner().
  Prompt("What is the weather in Hanoi?").
  ActiveTools("get_weather").
  Run(ctx)
```

`tool.NewSet` rejects duplicate names and snapshots definitions and invokers
together in registration order. Agent construction and per-run snapshots do
not expose mutable registry fields. Dynamic tools and MCP-discovered tools use
the same `tool.Set` runtime, so tool execution failures remain typed across
sources.

Tool output is literal text by default. When a provider can consume richer
results, construct ordered typed content explicitly:

```go
part := aikit.RichToolResultPart(
  call.ID,
  call.Name,
  aikit.TextToolResultContent("chart generated"),
  aikit.JSONToolResultContent(json.RawMessage(`{"rows": 12}`)),
  aikit.ImageToolResultContent(png, "image/png"),
)
```

JSON-looking strings remain text. Use `aikit.JSONToolResultContent` or
`aikit.ParseToolResultJSON` when JSON semantics are intended. Rich content is cloned
when messages and result snapshots are built, so caller mutation cannot alter
stored history.
