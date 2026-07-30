# conformance

A Node harness that drives the **real** `ai` package (7.0.35) against the Go server,
so conformance is asserted by the actual client rather than by our reading of it.

Filled in by Phase 03. Planned shape:

- `uiMessageChunkSchema` validates every chunk ai-go emits. This is the client's own
  Zod gate (`ui/default-chat-transport.ts:28-30` throws on any failure), so passing it
  is not optional.
- `readUIMessageStream` as the test oracle. A server does not consume its own SSE, so
  the deleted `process-ui-message-stream.go` is replaced by the real client here
  instead of being reimplemented in Go.
- Negative fixtures for the seven `UIMessageStreamError` throw sites
  (`ui/process-ui-message-stream.ts:145,163,439,457,500,518,622`) — a `*-delta`
  without its `*-start`, and an unknown `approvalId`.

Invariants the client does **not** enforce are deliberately absent from here and live
as producer-side Go assertions instead: a reused or empty `toolCallId`, a missing
`input`, and unclosed blocks all pass the client silently (tool lookup falls back to a
whole-message reverse scan; unknown chunk types hit an ignoring `default:`). Testing
them here would assert nothing.

Run: `pnpm install && pnpm test`
