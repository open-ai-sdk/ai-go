# Provider transport inventory

Measured on `rig-ai` immediately before Phase 04 at `649a934`. The approved
plan expected 11 request constructors; the current tree had 12. The table below
records the actual work list and the correctness properties that the shared
transport must preserve.

## HTTP request sites

| # | Request site | Shape | Timeout and cancellation | Error/body handling before Phase 04 | Phase 04 disposition |
|---:|---|---|---|---|---|
| 1 | `provider/anthropic.(*LanguageModel).Stream` | Streaming POST, Anthropic Messages SSE | Response-header timeout; request context owns stream lifetime | Shared typed API error; decoder and caller both closed body | Shared `transport.Client`, error mapper, SSE reader, and stream owner |
| 2 | `provider/openai.(*LanguageModel).doRequest` | Streaming or one-shot POST, Responses API | Response-header timeout; optional per-chunk timeout | Shared typed API error; success body returned to caller | Shared `transport.Client`; streaming caller uses shared stream owner |
| 3 | `provider/openai.(*fileClient).upload` | One-shot multipart POST | Safe whole-response 120 s client timeout | Unbounded body read; raw error body entered error text | Request construction stays provider-local; shared typed/redacted error mapper |
| 4 | `provider/internal/openaichat.(*LanguageModel).Stream` | Streaming POST, Chat Completions SSE | Response-header timeout; optional injected client and per-chunk timeout | Shared typed API error; decoder and caller both closed body | Shared `transport.Client`, error mapper, SSE reader, and stream owner |
| 5 | `provider/gemini.(*NativeLanguageModel).Stream` | Streaming POST, native Gemini SSE | Response-header timeout; request context owns stream lifetime | Shared typed API error; decoder and caller both closed body | Shared `transport.Client`, error mapper, SSE reader, and stream owner |
| 6 | `provider/gemini.(*ImageModel).Generate` | One-shot POST | Safe whole-response client timeout | Bounded body read, but raw body entered error text | Request construction stays provider-local; shared typed/redacted error mapper |
| 7 | `provider/gemini.(*EmbeddingModel).EmbedBatch` | One-shot POST | Safe whole-response client timeout | Bounded body read, but raw body entered error text | Request construction stays provider-local; shared typed/redacted error mapper |
| 8 | `provider/kie.(*ImageModel).submitTask` | One-shot POST | Request context plus configured whole-response timeout | Shared Kie envelope parser; application-level `code` can fail on HTTP 200 | Kept provider-local; Kie application semantics are not HTTP transport semantics |
| 9 | `provider/kie.(*ImageModel).fetchStatus` | One-shot polling GET | Request context plus configured whole-response timeout | Shared Kie envelope parser | Kept provider-local |
| 10 | `provider/kie.(*ImageModel).downloadOne` | One-shot binary GET | Request context plus configured whole-response timeout | Status-only error; successful body is image bytes | Kept provider-local |
| 11 | `provider/kie.(*Provider).UploadBase64` | One-shot POST | Request context plus configured whole-response timeout | Shared Kie envelope parser | Kept provider-local |
| 12 | `provider/kie.(*Provider).UploadFromReader` | Streaming multipart request body; one-shot response | Request context plus configured whole-response timeout | Shared Kie envelope parser; pipe writer has its own lifecycle | Kept provider-local |

The eight provider-local constructors left after Phase 04 are the non-streaming
paths assigned to the broader provider retarget in Phase 05. Four streaming
provider constructors now use the shared client.

## SSE decoders

| Decoder | Before Phase 04 | Cancellation/body ownership | Provider-specific behavior retained |
|---|---|---|---|
| `provider/anthropic.decodeSSEStream` | Uncapped `bufio.Reader`; `event:` + `data:` frames | Decoder and caller double-closed; decoder ran a close-on-cancel watcher | Anthropic event names, content-block state, thinking/tool deltas, usage |
| `provider/openai.decodeResponsesSSEStream` | Uncapped `bufio.Reader`; one JSON value per `data:` line | Decoder closed body/channel; `GuardStream` relayed and drained | Responses event dispatch, response ID metadata, tool-call assembly |
| `provider/internal/openaichat.DecodeSSEStream` | Uncapped `bufio.Reader`; one JSON value per `data:` line | Decoder and caller double-closed; `GuardStream` relayed and drained | Chat chunk normalization, usage, finish fallback, provider hooks |
| `provider/gemini.decodeNativeSSEStream` | Uncapped `bufio.Reader`; one JSON value per `data:` line | Decoder and caller double-closed; `GuardStream` relayed and drained | Native grounding, files, thinking, tools, usage, finish metadata |

`provider/gemini/gemini-sse-stream-decoder.go` was not a fifth line reader; it
already delegated framing to `openaichat` and supplied only Gemini metadata and
source hooks.

All four decoders now consume the single uncapped `transport.SSEReader`.
`transport.Stream` alone owns the HTTP response body and returned channel,
closes a hung body on context cancellation, recovers decoder panics, and drains
decoder output when a consumer cancels.

## Other shared transport behavior

- Retry classification, exponential backoff, jitter, cancellation-aware waits,
  and typed `Retry-After` extraction moved from `ai/retry.go` to
  `transport/retry.go`. The public `ai.RetryConfig` remains an alias so the API
  contract is unchanged.
- HTTP error mapping moved from `httputil` to `transport`. It retains status,
  parsed provider code/message, request ID, and header-derived `Retry-After`.
  Raw bodies remain absent from errors and go only to an explicitly injected
  debug logger.
- The two planned Gemini redaction bypasses and the additionally discovered
  OpenAI file-upload bypass now use the shared mapper.
- MCP's Streamable HTTP SSE parser used `bufio.Scanner` without checking
  `Scanner.Err`, so a line over 64 KiB ended silently. It now uses the shared
  frame decoder and retains multiline `data`, `event`, `id`, comments, and
  trailing-frame behavior.
- MCP reconnection backoff and Kie task polling remain separate domain
  policies. Neither retries a provider model request, so folding them into the
  provider retry policy would conflate different state machines.

## Baseline verification

Before refactoring:

- `grep` found no `bufio.Scanner` in `provider/`.
- All four provider SSE readers were uncapped and all four closed the body on
  cancellation.
- Streaming clients had no `http.Client.Timeout`; Gemini image/embedding,
  OpenAI upload, and Kie kept whole-response deadlines because they are
  non-SSE paths.
- Provider `Retry-After` was already header-based and typed.
- No non-test control flow inspected `err.Error()` with `strings.Contains`.
