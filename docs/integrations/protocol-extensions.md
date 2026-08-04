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

The bundled `agui` adapter is a minimal subset: text and tool lifecycle events
are supported; state synchronization, approvals, reasoning, source/file
events, durability, and resume are deliberately not supported.
