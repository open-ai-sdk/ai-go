# AI SDK v7 UI streams

The `uistream` package owns the protocol-neutral event drain and framing
driver. `uistream/ainode` provides the AI SDK v7 encoder and decoder;
`uistream/agui` provides a minimal AG-UI adapter. `aisdk` remains the compatible
AI Node type-alias, forwarder, and imperative-writer surface. `aisdkhttp` is the
small `net/http` boundary that decodes a protocol request, passes its ordered
message history to an Agent Runner, and flushes the resulting event stream.

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

The included `examples/chat-server` is the runnable reference implementation
with `/chat` and `/healthz`, and is exercised by the browser conformance suite.
Gin users can use the separately versioned `aisdkgin` module, which wraps the
same HTTP handler without adding Gin to the core dependency graph.

See [Agent Runner](/core/agent-runner) for Runner ownership and validation before
connecting the iterator to an HTTP adapter.
