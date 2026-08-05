# UI streams

`uistream` is ai-go's shared boundary for streaming normalized
`aikit.StepEvent` values to a browser-facing UI protocol. It owns the common
mechanics — request decoding, SSE framing, cancellation, flushing, terminal
errors, and panic recovery — while each adapter owns its protocol vocabulary
and client-specific ordering.

Choose the adapter matching the client:

- [AI SDK v7 UI streams](/integrations/ui-streams) for AI SDK `useChat`.
- [AG-UI and TanStack AI](/integrations/ag-ui) for TanStack AI clients.

## Shared adapter seam

[UI stream protocol extensions](/integrations/protocol-extensions) explains
`uistream.Protocol`, `Pipe`, the lifecycle invariants, and the capability
boundaries between the bundled adapters. Read it before changing stream
semantics shared by more than one protocol.

To add a third wire protocol, follow [Write a UI stream adapter](/integrations/writing-ui-stream-adapter).
It contains the minimal encoder/decoder/framer skeleton and the contract test
that every adapter should have.

The imperative `aisdk.Writer` APIs remain AI SDK-specific compatibility
surfaces; they do not belong to the event-driven `uistream` adapter seam.
