# Gemini path consolidation

## Decision

Keep both language-model constructors, with explicit capability boundaries:

- `NewLanguageModel` uses Gemini's OpenAI-compatible Chat Completions endpoint.
  It handles basic chat, function tools, structured output, multimodal input,
  stream usage, and thinking configuration.
- `NewNativeLanguageModel` uses `streamGenerateContent`. It remains the only
  path for Google Search grounding, citations, source metadata, response
  modalities, image/file output deltas, and native thought signatures.

The SDK does not auto-route between them. The endpoints have different URL and
wire contracts, and silently changing paths would make proxy configuration and
request capture unpredictable.

## What was removed

The former compatible grounding stack was unreachable from
`NewLanguageModel`: production used the shared compatible decoder while
`gemini_sse_stream_decoder.go` was called only by tests. Its fixtures also
encoded Google Search and thinking fields in shapes not accepted by the
compatible chat endpoint.

The following duplicate compatibility helpers were removed:

- the Gemini wrapper around the generic request encoder;
- the unreachable Gemini compatibility SSE decoder;
- compatibility-only Google Search mutation and warning relay;
- tests that asserted those unreachable wire shapes.

The compatible model now rejects grounding and response-output options with an
error directing callers to `NewNativeLanguageModel`. Thinking options are
nested under `extra_body.google.thinking_config`.

## Why the native path remains

The native path carries behavior the Chat Completions codec cannot express:

- `googleSearch` and incrementally streamed grounding chunks;
- normalized web, retrieved-context, image, and Maps source events;
- citation, URL-context, safety, and search-entry metadata;
- `responseModalities` and `imageConfig`;
- inline image/file output deltas;
- native function calls and thought signatures.

Native stream decoding now accumulates grounding arrays across chunks, keeps
raw citation and URL-context metadata, recognizes `sourceUri`/`imageUri`, and
remembers tool calls until the later finish chunk. These are guarded by
multi-chunk regression tests.

## Transport and options

Compatible chat, native chat, embedding, and image calls all use
`transport.Client`. `Config.BaseURL` now reaches embedding calls, and
`Config.ChunkTimeout` reaches both language-model paths. Callers can inject a
`transport.Doer` through `Config.HTTPClient`.

Gemini options use the typed `ProviderOptions` value as the primary path and
the strict JSON-object decoder retained by Phase 03. The invalid configurable
Google Search sub-shapes were removed; basic native `googleSearch: {}` remains.
