# conformance

Runs the **real** `ai@7.0.35` client over SSE that `ai-go` produced, so conformance is
asserted by the actual client rather than by our reading of it.

```bash
go test ./aisdk/ -run TestConformanceFixtures -update-fixtures   # regenerate fixtures
cd conformance && pnpm install && pnpm test                      # validate them
```

## Why three layers

The v7 client is strict in two places and quietly lenient everywhere else. A single
layer would miss most of what can go wrong.

| Layer | Where | Catches |
|---|---|---|
| 1 — schema | `schema-conformance.test.ts` | Structural divergence and unknown chunk types, via `parseJsonEventStream` + the real `uiMessageChunkSchema` — the same call `DefaultChatTransport` makes |
| 2 — behaviour | `processor-conformance.test.ts` | Ordering and state-machine divergence, via `readUIMessageStream`, asserting rendered `message.parts` |
| 3 — producer | `aisdk/chunk-invariants.go` | Everything the client accepts and then renders **wrong** |

Layer 3 is in Go on purpose. A reused or empty `toolCallId`, a missing `input`, a dotless
`custom.kind`, and an unclosed text block all pass the client without error — its tool
lookup falls back to a whole-message reverse scan instead of throwing, and unknown chunk
types hit an ignoring `default:`. A TypeScript negative fixture for any of them would
pass, so they are asserted producer-side instead.

A fourth layer — a real browser `useChat` under Playwright — is Phase 10.

## Two things this suite protects against in itself

**`terminateOnError: true` is not optional.** The default is `false`
(`read-ui-message-stream.ts:29`), which routes every error to `onError` and then closes
the stream *normally*. Under that default every negative fixture yields a plausible
truncated message and no exception, so the suite goes green while testing nothing.
`negative.test.ts` has a meta-test that demonstrates this directly, and every negative
case asserts that `onError` actually fired rather than merely that the run finished.

**The `.expected.json` files are hand-written.** They are authored from reading
`ai/src/ui/process-ui-message-stream.ts`, not generated from Go output. If both sides
came from Go, the suite would assert only that Go agrees with itself.

## Adding a fixture

1. Add a `conformanceFixture` entry to `conformanceFixtures()` in
   `ai-go/aisdk/conformance-fixtures-generate_test.go`. Put it in `invalid/` if the
   client's stream processor should throw, or `invalid-schema/` if the chunk schema
   itself should reject it.
2. Regenerate: `go test ./aisdk/ -run TestConformanceFixtures -update-fixtures`.
3. For a positive fixture, hand-write `fixtures/<name>.expected.json` by reading the TS
   processor. Layer 2 fails if it is missing. For a multi-step fixture the expectation
   **must** pin the index of every `step-start` part — the client does not throw for a
   mis-derived step boundary, so position is the only way to detect one.

Then `pnpm test`.

## Deliberate omissions

Four fixture ideas were dropped because they cannot fail:

| Not tested here | Why |
|---|---|
| `abort` | The processor has no `case 'abort'`; it is a rendering no-op. v7's abort is client-driven. |
| Reused / unknown `toolCallId` | The whole-message fallback lookup exists precisely to tolerate it. Layer 3 instead. |
| `tool-input-available` with no `input` | `input` is `z.unknown()`, so an absent key validates. Layer 3 instead. |
| `custom` and `data-*` streams | No Eino content block produces them, so there is no contract to keep yet. |

## Version pinning

`ai` is pinned to `7.0.35` with no caret, matching `ref/ai-v7-node/packages/ai`. That
means this suite will **not** warn about a v7.1 protocol change — on any upgrade, diff
`ui-message-chunks.ts` and `process-ui-message-stream.ts` first.

`@ai-sdk/provider-utils` is pinned to `5.0.12`, the version `ai@7.0.35` itself depends
on, because layer 1 calls `parseJsonEventStream` directly.
