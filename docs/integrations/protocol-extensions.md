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

| Adapter | Protocol | Terminator |
| --- | --- | --- |
| `uistream/ainode` | AI SDK v7 UI message stream | `finish` chunk then `data: [DONE]` |
| `uistream/agui` | AG-UI, as consumed by TanStack AI | `RUN_FINISHED` |

Both cover the full `aikit.StepEvent` vocabulary: text, reasoning, tool
lifecycle, tool approval, sources, files, structured output, and usage. They
differ in how each is spelled, because each protocol's client enforces its own
schema. See [AG-UI and TanStack AI](/integrations/ag-ui) and
[AI SDK v7 UI streams](/integrations/ui-streams) for the per-protocol contract.

`StepEventDone` is swallowed by both adapters: the driver's `Finish` call owns
the single terminal event, so an adapter never emits one from `Encode`.

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
