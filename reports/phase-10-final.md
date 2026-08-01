# Phase 10 final report

Measured on 2026-08-01 from the phase baseline commit `424c0f9` and the final
working tree. Generated files under dependency directories are excluded.

## Delivered architecture

- `ai` is an alias-only façade. Concrete high-level generation, options,
  aggregation, and stream-view ownership now lives in `agent/generate`.
- `GenerateText`, `StreamText`, `GenerateObject[T]`, `Embed`, and `EmbedMany`
  retain their public façade behavior. Bare text generation remains one step;
  `ToolLoopAgent` remains 20 steps.
- The shared approval signature/canonical-JSON implementation moved downward
  to the standard-library-only `aikit` leaf. `agent` no longer imports `aisdk`;
  `aisdk` re-exports the frozen v7 approval contract.
- Logging and tracing are silent by default. `agent.Tracer`/`ai.WithTracer`
  provide explicit injection, and the regular `otelagent` package contains the
  OpenTelemetry adapter. Core `go.mod` declares the adapter dependencies, while
  `agent` and provider-only package closures contain no OTel, go-logr, or
  xxhash packages.
- Provider streaming response ownership is explicit: `transport.Client.DoStream`
  owns and closes asynchronous response bodies, while `transport.HandleResponse`
  owns synchronous response lifetimes. No `nolint` suppression is used.
- All 88 remaining kebab-case Go files were renamed with `git mv`; all Go-source
  directories have `doc.go`; the compatibility module compiles and executes all
  four required façade operations.

## Measured result

| Metric | Baseline | Final | Result |
|---|---:|---:|---:|
| Non-test Go LOC | 17,963 | 21,481 | +3,518 |
| Test Go LOC | 18,748 | 22,565 | +3,817 |
| Go files | 245 | 354 | +109 |
| `ai` non-test LOC | 4,114 | 583 | -3,531 (-85.8%) |
| `ai` + new `agent/generate` LOC | 4,114 | 4,103 | -11 |
| Provider non-test LOC | 6,239 | 5,734 | -505 (-8.1%) |
| Hand-written converters between façade and engine | 646 | 0 | -646 |
| Non-test files over 200 LOC | 30 | 34 | +4 |
| Non-test files over 250 LOC | 18 | 16, all justified | gate satisfied |
| Kebab-case Go filenames | 193 plan baseline / 88 entering phase 10 | 0 | gate satisfied |
| Benchmarks | 3 | 3 | preserved |

The overall LOC target was not met. New public packages, wire invariants,
stateless approval security, HTTP edges, compatibility/conformance coverage,
and structural checks more than offset the removed converter/provider code.
The provider consolidation also missed the original 1,400–1,700 LOC estimate:
the measured reduction is 505 LOC. These are explicit deviations, not release
claims.

### Package/location comparison

| Baseline location | Baseline LOC | Final owner/location | Final LOC |
|---|---:|---|---:|
| `ai` | 4,114 | `ai` | 583 |
| — | — | `agent/generate` | 3,520 |
| `aitypes` | 360 | `aikit` | 761 |
| `internal/engine` | 1,793 | `agent` | 2,948 |
| `uistream` | 3,332 | `aisdk` | 3,214 |
| `httputil` | 275 | `transport` + `aisdkhttp` | 1,040 |
| `mcp` | 1,505 | `mcp` | 1,482 |
| all `provider/*` | 6,239 | all `provider/*` | 5,734 |
| `schema` | 71 | removed; owned by `tool` | 0 |
| `internal/ctxlog` | 34 | removed; context logging in `transport` | 0 |
| `internal/safego` | 60 | `internal/safego` | 63 |
| `internal/tracing` | 120 | `internal/tracing` | 55 |
| — | — | optional `otelagent` package | 72 |
| — | — | `llm` | 449 |
| — | — | `tool` | 978 |

### Final non-test files over 200 LOC

All files over 250 LOC contain a specific inline
`ai-go: file-length-justification` marker enforced by CI.

```text
489 provider/gemini/native_request_encoder.go
487 agent/generate/stream_result.go
464 provider/gemini/native_sse_decoder.go
460 agent/step.go
432 agent/run.go
428 agent/approval_resume.go
423 mcp/transport_http.go
349 provider/openai/openai_responses_request_encoder.go
339 provider/anthropic/anthropic_language_model.go
322 provider/openaicompat/request_encoder.go
312 provider/openai/openai_responses_sse_stream_decoder.go
302 agent/tool_approval.go
300 provider/anthropic/anthropic_sse_decoder.go
291 provider/kie/kie_image_model.go
261 provider/openaicompat/sse_decoder.go
256 mcp/client.go
247 tool/schema.go
246 agent/generate/smooth_stream.go
234 agent/generate/request_builder.go
232 agent/generate/runtime_facade.go
225 agent/validation.go
221 mcp/jsonrpc.go
221 aisdk/invariants.go
220 aisdk/persisted_message.go
209 provider/gemini/gemini_image_model.go
208 agent/generate/generate_text_request_and_result.go
207 examples/chat-server/main.go
207 aisdk/producer.go
206 provider/openaicompat/model.go
206 aisdk/writer.go
205 aisdk/to_ui_message_stream.go
204 aikit/approval_canonical.go
204 agent/stream.go
203 tool/set.go
```

## Final gate

- `go build ./...`
- `go vet ./...`
- `go test -race ./... -count=1`
- build, vet, and race tests for the core module (including `otelagent`) and
  the separate `aisdkgin` and `compat-test` modules
- `golangci-lint run --timeout=5m` — 0 issues
- `bun run test` — 15/15
- `bun run typecheck`
- `bun run check`
- `bun run test:browser` — 4/4 Chromium flows (text, tool,
  approve, deny); the local run used `AI_GO_CONFORMANCE_API_PORT=18787`
  because an unrelated process owned the default port
- all structural boundary checks and the alias-only façade AST check
- all three benchmarks executed with `-benchtime=1x`

The module remains on `v0.x`. No `v1.0.0` tag was created.
