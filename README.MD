# ai-go

An **AI SDK v7 protocol adapter for Eino agents**.

```
Gin  +  Eino  +  ai-go   ──SSE──▶   @ai-sdk/react  useChat
```

`ai-go` serves a conforming [AI SDK v7](https://ai-sdk.dev) UI message stream driven by
a [CloudWeGo Eino](https://github.com/cloudwego/eino) agent, so a Go backend can talk to
an unmodified `useChat` frontend.

## This is not an agent SDK

It used to be. It no longer is, and that is the point.

Providers, the tool-loop engine, MCP, and the model abstraction were removed — roughly
27k lines — because Eino already provides all of them, and better. What nothing else
provides is the AI SDK v7 wire protocol: a search across every Go file and README in
`eino-ext` for `text-delta`, `tool-input-available`, or `ui-message-stream` returns
nothing. That gap is the whole scope of this library.

If you want an agent framework, use Eino. If you want Eino to speak to `useChat`, use
this.

## Packages

| Package | Depends on Eino | Purpose |
|---|---|---|
| `aisdk` | **no** | The protocol: chunk types, SSE framing, UIMessage parsing, approval crypto, the normalized `StepEvent` vocabulary |
| `einoadapter` | yes | `TypedAgentEvent[*schema.AgenticMessage]` → chunks; the approval gate |
| `aisdkhttp` | no | `http.Handler` + SSE flusher (net/http only) |
| `aisdkgin` | no | `gin.HandlerFunc` |
| `conformance` | — | Node harness driving the real `ai` package against the Go server |

Two boundaries are enforced in CI, not just documented:

- **`aisdk` imports neither Eino nor Gin** — asserted with `go list -deps ./aisdk/...`.
  It currently has *zero* external dependencies. Module-graph pruning means a consumer
  importing only `aisdk` never compiles Eino or `sonic`.
- **Eino imports inside `einoadapter` live on an allowlist** that CI diffs
  (`einoadapter/.eino-imports-allowlist`), so widening the Eino surface is a review
  event rather than an accident.

## Tool approval signing — and what it does not prove

Human-in-the-loop approvals are stateless: the request goes out to the browser, and the
answer comes back inside the message history on the next POST. Nothing is stored
server-side. To stop a client from forging an approval request, each one is HMAC-signed.

Two payload formats:

```
v1  ["ai-sdk-tool-approval-v1", approvalId, toolCallId, toolName, hashCanonical(input)]
v2  ["ai-go-tool-approval-v2",  approvalId, toolCallId, toolName, hashCanonical(input),
     principalId, chatId, iat]
```

`v1` is the AI SDK v7 format, reproduced byte-for-byte and **verified** for interop with a
TypeScript server. `v2` is what ai-go **emits**, adding the three terms v1 lacks.

**Read this before relying on it.** `approved` is not in either payload. The client sets
`approved` itself and passes the server's signature through unchanged, so a signature
verifies whatever the answer says. Concretely: a user clicks **Deny**, and anything with
client control — XSS, a hostile extension, devtools, curl — re-POSTs the same history with
`approved: true`. The signature still verifies and the tool runs.

So the signature proves *the server gated this tool with this input, for this principal, in
this chat, at this time.* It does **not** make the browser's answer trustworthy. No signing
scheme on this side of the wire can; only a server-side record of the decision would, which
is exactly what the stateless design trades away. What `v2` buys over `v1` is that a
signature stops being a permanent bearer capability replayable into any chat by anyone who
obtains it.

Deliberate divergences from the reference:

- **No legacy newline-joined payload.** The reference accepts one as a verify-time
  fallback for approvals signed by pre-JSON builds; a greenfield Go server never issued
  one, and upstream schedules its removal. Accepting it would only widen what this server
  trusts.
- **Invalid UTF-8 is refused, not replaced.** Go's JSON encoder would substitute U+FFFD,
  and that is a hash collision: `"\uD800"` and `"\uD801"` would produce one digest.
  `HashCanonical` returns a typed error instead.
- **A depth budget of 64** on `HashCanonical`. It is the first code to touch
  attacker-supplied JSON on the approval path, and the reference's recursion is unguarded.

Cross-language agreement is enforced by shared vectors in `aisdk/testdata/`, asserted from
**both** Go and TypeScript — see `conformance/`. A one-directional check cannot catch the
failure that matters, which is Go accepting a signature the real client would reject.

## Status

Under active reconstruction. `aisdk` carries the protocol layer and its test suite;
`einoadapter`, `aisdkhttp`, and `aisdkgin` are scaffolded and land phase by phase.

Eino is pinned to **v0.9.13** and CI asserts the exact version. This is load-bearing:
the agentic message API does not exist below v0.9.0-beta.1, so a drifted pin would fail
in confusing ways rather than obvious ones.

## Requirements

- Go 1.25+
- Eino v0.9.13
- A frontend on `ai` / `@ai-sdk/react` 7.x

## License

See [LICENSE](LICENSE).
