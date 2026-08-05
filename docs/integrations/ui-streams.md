# AI SDK v7 UI streams

The `uistream` package owns the protocol-neutral event drain and framing
driver. `uistream/ainode` provides the AI SDK v7 encoder and decoder;
`uistream/agui` provides the [AG-UI adapter](/integrations/ag-ui) that TanStack
AI clients consume. `aisdk` remains the compatible AI Node type-alias,
forwarder, and imperative-writer surface. `aisdkhttp` is the small `net/http`
boundary that decodes a protocol request, passes its ordered message history to
an Agent Runner, and flushes the resulting event stream.

Build the Agent once and create a fresh Runner for every request:

```go
func chatRun(assistant *agent.Agent) aisdkhttp.RunFunc {
	return func(
		ctx context.Context,
		messages []aikit.Message,
	) (iter.Seq2[aikit.StepEvent, error], error) {
		return assistant.Runner().
			Messages(messages...).
			Stream(ctx)
	}
}

http.Handle("/chat", aisdkhttp.Handler(chatRun(assistant)))
```

`Messages` replaces the Runner's complete ordered input sequence. That
preserves multipart user content, previous assistant/tool messages, and
approval-response history posted by `useChat`; reducing the request to a text
prompt would lose that state.

The Runner sequence remains single-owner. `aisdkhttp` ranges it once and maps
its leaf `aikit.StepEvent` values into v7 chunks. Client disconnects and an
early consumer stop cancel the child run. Synchronous Runner validation errors
remain pre-stream HTTP failures; terminal iterator errors become redacted v7
error chunks followed by `[DONE]`.

The Go Agent rewrite does not change the v7 wire contract: chunk fields,
finish-reason translation, response headers, approval signatures, and SSE
termination stay compatible. The protocol adapters depend only on the leaf
`aikit` vocabulary and the `uistream` driver; they do not import `agent`.

## Token usage

v7 removed every usage field from the chunk schema, so token counts travel in
`messageMetadata` — the only channel that reaches `useChat`'s `onFinish`. The
handler folds per-step usage snapshots into a run total and attaches it to the
`finish` chunk:

```json
{
  "type": "finish",
  "finishReason": "stop",
  "messageMetadata": {
    "usage": { "inputTokens": 11, "outputTokens": 22, "totalTokens": 33 }
  }
}
```

`inputTokenDetails` and `outputTokenDetails` are included only when the provider
reports them, and the whole `messageMetadata` field is omitted when a run reports
no usage at all.

Read it client-side from the message metadata:

```tsx
const { messages } = useChat<UIMessage<{ usage?: { totalTokens: number } }>>({
  transport: new DefaultChatTransport({ api: '/chat' }),
})

const total = messages.at(-1)?.metadata?.usage?.totalTokens
```

## Tool outcomes

A tool result's `Disposition` selects the terminal chunk for the call:

| `aikit.ToolResultDisposition` | v7 chunk | Client part state |
| --- | --- | --- |
| `ToolResultSuccess` (and unset) | `tool-output-available` | `output-available` |
| `ToolResultError` | `tool-output-error` | `output-error` |
| `ToolResultDenied` | `tool-output-denied` **and then** `tool-output-available` — see below | `output-available` |
| `ToolResultRefused`, `ToolResultSkipped` | `tool-output-available` — v7 has no equivalent state | `output-available` |

A refused approval currently emits **two** terminal chunks for one call. The
agent reports the refusal directly (`StepEventToolOutputDenied`) and *also*
returns a `ToolResult` with the denied disposition, which the producer maps to
`tool-output-available`. The client applies them in order, so the part settles on
`output-available` carrying `{"error":"tool approval denied"}` as its output —
a denial that renders as a success. Treat `output-denied` as unreliable on the
engine path until this is resolved; the imperative `Writer.WriteToolOutputDenied`
is unaffected.

`errorText` on `tool-output-error` reaches the browser **unredacted**, unlike the
terminal `error` chunk. It comes from `ToolResult.Output` — the scrubbed
model-visible text `tool.Details` built — and falls back to `ToolResult.Error`
only when `Output` is empty. The two are deliberately different: `Error` retains
the full Go chain, including wrapped causes such as internal hostnames or
credentials in a URL, and is host-side only. A tool that wants a specific failure
message on screen should implement `tool.DetailedError`; its text lands in
`Output` and reaches the client unchanged.

## Files and structured output

A model-emitted file becomes a `file` chunk carrying a data URL. v7's `file`
chunk has only `url` and `mediaType` — it has no `filename`, so a filename
cannot be delivered over the stream:

```json
{"type":"file","mediaType":"image/png","url":"data:image/png;base64,..."}
```

Structured output has no dedicated v7 chunk. It is published on the sanctioned
`data-` extension prefix as `data-structured-output`, marked `transient` so it
reaches the UI without being persisted into the message parts.

The included `examples/chat-server` is the runnable reference implementation
with `/chat` and `/healthz`, and is exercised by the browser conformance suite.
Gin users can use the separately versioned `aisdkgin` module, which wraps the
same HTTP handler without adding Gin to the core dependency graph.

See [Agent Runner](/core/agent-runner) for Runner ownership and validation before
connecting the iterator to an HTTP adapter.
