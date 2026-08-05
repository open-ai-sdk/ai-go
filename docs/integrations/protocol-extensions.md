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

The adapter accepts the core `RunAgentInput` fields (`threadId`, `runId`,
`messages`, `state`, `tools`, `context`, and `forwardedProps`) and emits
`RUN_*`, `STEP_*`, `TEXT_MESSAGE_*`, and `TOOL_CALL_*` events. Usage snapshots
are folded per step and returned under `RUN_FINISHED.result.usage`.

`StepEventDone` is swallowed because the driver's `Finish` call owns the one
terminal AG-UI event. Reasoning, structured-output, source, and file events are
deliberately dropped. An approval request returns
`agui.ErrToolApprovalUnsupported` and terminates with `RUN_ERROR`; AG-UI's
interrupt/resume lifecycle is outside this minimal adapter.

This subset follows the official AG-UI core event and `RunAgentInput`
documentation as reviewed on 2026-08-04. Behavioral conformance against a real
AG-UI client remains a separate integration gate; the local golden tests prove
JSON shape, ordering, pairing, and terminal behavior only.
