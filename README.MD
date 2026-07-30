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
