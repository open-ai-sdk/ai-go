# Run the chat server

The repository includes a runnable server that exercises the production `aisdkhttp` boundary for text, tool, error, approval, and denial flows.

```sh
go run ./examples/chat-server
curl -i http://127.0.0.1:8787/healthz
```

Use it as a reference for an AI SDK v7-compatible chat endpoint or to run the browser conformance suite locally. The server is deliberately small: your application supplies a model and run function; `aisdkhttp.Handler` handles request decoding, response headers, SSE chunks, flushing, and disconnect cancellation.

For protocol parameterization, `examples/multi-protocol` serves one Agent at
`/ai-sdk` and `/ag-ui`. `aisdkhttp.HandlerFor` selects a protocol while the
run function and `uistream.Pipe` driver stay shared.
