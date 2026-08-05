# UI stream protocol extensions

`uistream` separates the event-driven transport mechanics from a UI protocol.
It owns request/response framing, draining an `aikit.StepEvent` iterator,
terminal-error normalization, flushing, and panic recovery. A protocol adapter
owns its event vocabulary, ordering, block identity, persistence model, usage
shape, and public error redaction.

Use `aisdkhttp.HandlerFor` to serve an adapter. The existing `Handler` remains
the stable AI Node v7 compatibility default.

```go
http.Handle("/chat", aisdkhttp.HandlerFor(agui.Protocol(), run))
```

The seam covers only event-driven streams. The `aisdk.Writer` and related
imperative APIs intentionally remain AI Node-specific, since they can emit
arbitrary protocol chunks that have no corresponding engine event.

## Bundled adapters

| Adapter | Protocol | Terminator | State events |
| --- | --- | --- | --- |
| `uistream/ainode` | AI SDK v7 UI message stream | `finish` chunk then `data: [DONE]` | Not representable in this protocol; dropped |
| `uistream/agui` | AG-UI, as consumed by TanStack AI | `RUN_FINISHED` | `STATE_SNAPSHOT`, `STATE_DELTA` |

Both adapters map text, reasoning, tool lifecycle, approvals, sources, files,
structured output, and usage. State is the intentional exception: AI SDK v7
has no run-state wire channel, while AG-UI carries it natively. They differ in
how each feature is spelled because each client enforces its own schema. See
[AG-UI and TanStack AI](/integrations/ag-ui) and [AI SDK v7 UI streams](/integrations/ui-streams)
for the per-protocol contract.

`StepEventDone` supplies final metadata to the AI SDK v7 adapter when present.
`Pipe` still guarantees termination: if an event iterator ends without it,
AINode emits a default `finish` chunk and `[DONE]`; AG-UI always emits its
terminal event from `Finish`.

## Writing an adapter

An adapter supplies three pieces to `uistream.Protocol`:

- `NewEncoder(Options) Encoder` — a per-run encoder. `Start` opens the stream,
  `Encode` maps one engine event to zero or more frames, and `Finish` is called
  exactly once with the terminal error, or `nil` on success.
- `Decoder` — parses the request body into `uistream.Request`.
- `Framer` — sets response headers and writes one frame.

Two invariants are the driver's, not the adapter's. `Pipe` normalizes
`StepEventError` into the terminal error before it reaches `Encode`, so an
adapter that receives one should treat it as a bug. And `Finish` runs even when
`Start` or `Encode` failed, so terminal cleanup belongs there rather than at the
end of the event loop.

Errors reaching the wire must pass through `uistream.RedactStreamError`, which
preserves an API error's HTTP status while dropping provider detail.

Behavioral conformance against a real client is a separate gate: the browser
suite in `conformance/` drives both adapters with the actual client libraries,
while the Go tests pin JSON shape, ordering, pairing, and terminal behavior.

For a copyable minimal implementation and its test shape, see
[Write a UI stream adapter](/integrations/writing-ui-stream-adapter).
