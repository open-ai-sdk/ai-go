# AI SDK v7 UI streams

The `aisdk` package owns the frozen AI SDK v7 chunk union, SSE framing, UI-message conversion, and approval signatures. `aisdkhttp` is the small `net/http` boundary: it decodes a v7 chat request, calls your event runner, and promptly flushes the response stream.

```go
func chatRun(model llm.Model) aisdkhttp.RunFunc {
  return func(ctx context.Context, messages []aikit.Message) (<-chan aikit.StepEvent, error) {
    return agent.Stream(ctx, agent.RunParams{
      Model: model,
      Request: llm.Request{Messages: messages},
    }), nil
  }
}

http.Handle("/chat", aisdkhttp.Handler(chatRun(model)))
```

The included `examples/chat-server` is a runnable reference implementation with `/chat` and `/healthz`. It is also used by the browser conformance suite. Gin users can use the separately versioned `aisdkgin` module, which wraps this same HTTP handler without adding Gin to the core dependency graph.
