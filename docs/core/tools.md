# Tools

Tools give an Agent named, schema-backed operations. Construct tools once,
place them in an immutable `tool.Set`, and attach that set to an Agent. The
Runner snapshots both definitions and invokers before each run, so later
source mutations cannot change an in-flight call.

```go
weather, err := tool.New("weather", "Look up the weather for a city", func(
	ctx context.Context,
	input struct { City string `json:"city"` },
) (string, error) {
	return "Sunny in " + input.City, nil
})
if err != nil {
	return err
}
tools, err := tool.NewSet(weather)
if err != nil {
	return err // duplicate names and invalid schemas fail here
}
```

Use a typed Go input struct for ordinary tools. `tool.New` derives and checks
the JSON Schema when the tool is created; a malformed model call becomes a
typed input error before the handler runs. Use `tool.NewDynamic` when an
integration supplies a runtime schema instead.

## Implement a tool as a type

For a stateful tool or a hand-authored schema, implement `tool.Typed` and pass
the value to `tool.Adapt`. `Describe` provides the provider definition and
`Call` receives decoded arguments. `context.Context` carries cancellation,
deadlines, and request-scoped values.

```go
type OperationArgs struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Adder struct{}

func (Adder) Describe() aikit.ToolDefinition {
	return aikit.ToolDefinition{
		Name:        "add",
		Description: "Add x and y together",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"x": map[string]any{"type": "integer"},
				"y": map[string]any{"type": "integer"},
			},
			"required": []string{"x", "y"},
		},
	}
}

func (Adder) Call(_ context.Context, args OperationArgs) (int, error) {
	return args.X + args.Y, nil
}

add, err := tool.Adapt(Adder{})
if err != nil {
	return err
}
```

`tool.Adapt` captures the definition, validates model JSON before `Call`, and
uses the same output and error handling as `tool.New`. Go infers the generic
argument and output types from `Adder{}`. Use `tool.New` for the concise
function form and `Typed` when the receiver needs dependencies, configuration,
or a custom schema.

The Agent owns the loop: it validates a model call, invokes the tool, commits a
canonical result, and makes a later model turn only when one is needed. For
application-owned continuation, use `Set.InvokeResult` directly. See
[Agent Runner](/core/agent-runner) for the turn budget and [Hooks](/core/hooks)
for call policy and result presentation.

## Make output explicit

Ordinary typed handler returns preserve the established behavior: `string` is
literal text and other Go values are JSON. Use `tool.ExecutionResult` when the
tool needs ordered mixed content or private metadata:

```go
report, err := tool.New("report", "Build a small report", func(
	ctx context.Context,
	input struct{},
) (tool.ExecutionResult, error) {
	output, err := tool.Content(
		aikit.TextToolResultContent("Report ready"),
		aikit.JSONToolResultContent(json.RawMessage(`{"rows":2}`)),
	)
	if err != nil {
		return tool.ExecutionResult{}, err
	}
	return tool.ExecutionResult{
		Output: output,
		Metadata: map[string]any{"cache": "hit"},
	}, nil
})
```

`tool.Text`, `tool.JSON`, `tool.Image`, and `tool.Content` make the output
type explicit. `tool.Content` retains the exact order of text, JSON, and image
parts. JSON-looking text is still text: ai-go never guesses an output type.

`Metadata` is host-only. It is excluded from provider history, default logs,
traces, and AI SDK UI streams. Do not rely on that boundary as an authorization
mechanism; a tool must enforce its own access rules.

The released `Invokable.Invoke` method remains available for custom and legacy
tools. New code that needs rich output should implement or call
`ResultInvokable.InvokeResult`. The registry adapts legacy bytes at that
boundary: valid bytes become explicit JSON; other bytes are literal text.

## Errors and dispositions

Tool handlers return ordinary Go errors. `errors.Is` and `errors.As` continue
to expose input, execution, denial, and unknown-tool errors to the host.
`tool.Details(err)` derives a stable safe classification and model-visible
message without copying arbitrary operator error text into conversation
history.

Every committed `aikit.ToolResult` has one disposition:

| Disposition | Meaning |
| --- | --- |
| `success` | The handler completed successfully. |
| `error` | Input or execution failed. |
| `denied` | Policy denied execution. |
| `refused` | A required approval was refused. |
| `skipped` | The runtime or a Hook skipped the call. |

For application-specific model feedback, return an error implementing
`tool.DetailedError`. Keep diagnostic causes in its normal Go error chain and
put only deliberately safe content in `ErrorDetails.ModelOutput`.

## Invocation context and concurrency

A handler receives the normal `context.Context`, including cancellation and
deadlines. The runtime also attaches the configured per-tool value, a
run-wide value, and the model's tool-call ID:

```go
func execute(ctx context.Context, input struct{ Query string `json:"query"` }) (string, error) {
	requestID, _ := tool.TypedContext[string](ctx)
	runtime := tool.RuntimeContextFrom(ctx)
	callID := tool.ToolCallIDFromContext(ctx)
	_ = requestID
	_ = runtime
	_ = callID
	return input.Query, nil
}
```

Use `tool.ToolContextFrom` when the per-tool value is not statically typed.
Maps passed through runtime contexts are cloned for each invocation, including
parallel tool calls. Treat values you place in a context as request-scoped and
avoid secrets in model-visible output or metadata that may be exported by your
own instrumentation.

`Runner.ToolConcurrency(1)` serializes handlers. Higher values permit handler
bodies to finish in parallel; their results are still committed in model-call
order. Hook callback chains are ordered per lifecycle event, so do not use
hooks as a global mutable-work queue.

## MCP discovery

MCP tools join the same immutable registry:

```go
tools, err := mcp.ToolSetFromClientsContext(ctx, map[string]*mcp.Client{
	"inventory": inventoryClient,
})
```

Discovery is cancellable, reads every page, and creates a one-shot snapshot;
it does not subscribe to live tool-list changes. Remote text, JSON, images,
and structured content retain their typed order where the MCP response carries
it. Rebuild a set when the remote server's tool list changes. See
[Model Context Protocol](/integrations/mcp) for client setup.

## Next: streaming and hooks

Tool result content appears in `Result`, the transcript, and the next model
request according to provider capability. Use [Hooks](/core/hooks) to audit or
steer tool calls. In particular, `ToolResultHook` can inspect immutable raw
execution facts while separately changing the model presentation.
