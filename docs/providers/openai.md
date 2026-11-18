# OpenAI

Create one OpenAI client, then derive the model handles needed by the
application:

```go
client, err := openai.NewClient(openai.Config{
  APIKey: os.Getenv("OPENAI_API_KEY"),
})
if err != nil {
  return err
}

model := client.CompletionModel("gpt-5") // Responses API
chatModel := client.ChatModel("gpt-4o")  // Chat Completions API
```

`NewClient` validates required configuration and transport setup eagerly. Its
`Config` supports `APIKey`, an optional `BaseURL`, request `Timeout`, streaming
`ChunkTimeout`, and an injectable `HTTPClient`. ai-go does not read environment
variables or `.env` files implicitly; load configuration in the application and
pass it explicitly.

The client owns credentials and shared HTTP resources. `CompletionModel` and
`ChatModel` return lightweight handles, so reuse one client instead of rebuilding
provider configuration for each model. A model ID can be used with more than one
factory when OpenAI supports that model on both operations.

Pass typed OpenAI options through the direct completion builder or an Agent
Builder/Runner when needed:

```go
result, err := llm.NewCompletion(model, "Explain this code").
  With(openai.ProviderOptions{ReasoningEffort: "low"}).
  Send(ctx)
```

Keep provider configuration at client creation and request-specific options on
the request.

## Responses payload

`client.CompletionModel` implements the optional native completion capability.
A direct `Send` therefore returns normalized assistant content and retains the
successful OpenAI Responses payload for provider-specific diagnostics:

```go
response, err := llm.NewCompletion(model, "Explain Go interfaces").Send(ctx)
if err != nil {
  return err
}

native, ok := llm.RawResponseAs[*openai.ResponsesResponse](response)
if ok {
  fmt.Println(native.ID, native.Status)
}
```

`ResponsesResponse.Raw` contains the exact response JSON, including fields the
typed DTO does not yet represent. It can contain sensitive data; ai-go never
logs it automatically, so redact it before application logging.

Reasoning tokens count toward OpenAI's output-token budget. A response can
therefore be successful but `incomplete`, with no visible text or assistant
content, when `max_output_tokens` is exhausted during reasoning. In that case
inspect `response.FinishReason`, usage, and `native.IncompleteDetails`; increase
the output budget or reduce reasoning effort when visible output is required.

## Tools, tool choice, and web search

`llm.Request.ToolChoice` reaches the Responses request body: `auto`, `none`,
and `required` are sent as strings and a named choice as
`{"type":"function","name":"..."}`. A named choice must refer to a declared
tool, and an unknown choice type is an error rather than a silent drop. Nil
omits the field. `tool_choice` and `parallel_tool_calls` are sent only when the
request declares tools, unless `ToolPolicyWithoutTools` is enabled.

```go
runner := assistant.Runner().With(openai.ProviderOptions{
  ParallelToolCalls: ptr(false), // nil keeps the API default
})
```

`aikit.ToolDefinition.Strict` sends `strict: true` on the function tool. The
schema must already satisfy strict-mode rules; `tool.Strictify` rewrites a raw
schema to comply. `ChatProviderOptions.ParallelToolCalls` does the same for
Chat Completions.

`ProviderOptions.EnableWebSearch` adds OpenAI's built-in web search tool. The
tool type defaults to `web_search_preview`; set `WebSearchToolType` (for
example `"web_search"`) for endpoints that expect the other spelling. Settings
the Responses API has no field for (`TopK`, `Seed`, `StopSequences`) produce
`unsupported-setting` warnings instead of being dropped silently.

The finish event's `ProviderMetadata["openai"]` carries `responseId`, `model`,
`requestID` (when the server supplies `x-request-id`), and `rawResponse`
(`json.RawMessage`, the terminal Responses payload). This includes unknown
provider fields without intercepting the HTTP body. Non-streaming `Complete`
results carry the identifiers in metadata and expose exact JSON plus `RequestID`
on `ResponsesResponse`. Tool-call delta events expose the native history item
identifier at `ProviderMetadata["openai"]["itemID"]`; completed tool-call content
retains it as `aikit.ContentPart.ItemID`.

## Optional Responses wire controls

All presentation controls below are opt-in `openai.ProviderOptions`; zero
values preserve existing request encoding:

| Option | Behavior |
| --- | --- |
| `InstructionsAsTopLevel` | Send `Request.Instructions` as top-level `instructions`, not a system message |
| `TrimInstructions` | Trim surrounding Unicode whitespace from `Request.Instructions` and omit blank results; history messages remain unchanged |
| `ExplicitMessageType` | Include `type: "message"` on role-based inputs |
| `SplitUserContentParts` | Emit one user message per content part, preserving order |
| `ImageDetail` | Set user and nested tool-result image detail (`auto`, `low`, or `high`); empty omits it |
| `IncludeEncryptedReasoning` | Include `reasoning.encrypted_content`, independently of `Store` |
| `ToolPolicyWithoutTools` | Send explicitly supplied tool choice/parallel policy without tool definitions |
| `HistoryItemStatus` | Set status on replayed function calls and outputs, e.g. `completed` |
| `OutputSchemaName` | Override a JSON schema's name |

Schema names otherwise use `llm.OutputSchema.Name`, falling back to
`structured_output`. Function-call history uses `ContentPart.ItemID` for `id`
and `ToolCallID` for `call_id`; they are separate identifiers.
Top-level instructions cannot carry the per-part prompt cache breakpoint and
cannot be combined with `PromptCacheInstructions`.

`EmitReasoningEvents` is a separate, default-off stream presentation option. It
preserves nullable source item IDs on reasoning deltas and emits one terminal
`StreamEventReasoningSummary` per completed item. Only nonblank `summary_text`
blocks are retained, trimmed and joined with two newlines; encrypted/redacted
blocks are never exposed as summaries. Part completion alone is not an item
terminal. This option does not alter the provider request body.

Responses streams require a terminal response event. EOF or `[DONE]` without
one produces `*aikit.MissingTerminalEventError`, also detectable with
`errors.Is(err, io.ErrUnexpectedEOF)`. Existing retry middleware handles it only
before commitment. Set `AllowMissingTerminalEvent: true` explicitly for a
delta-only legacy endpoint. `response.failed` is an API error, never a successful
finish. Neither local decoder option is sent in the request body.

`Config.Headers` supplies extra request headers. `Config.ResponsesPath`
overrides the route relative to `BaseURL` for both `Stream` and `Complete`;
empty uses `responses`. For example, a base of `https://api.example.org` and
path `custom/v1/responses` targets that exact route.

Both top-level SSE errors (`{"type":"error","code":...,"message":...}`) and
nested `error` objects produce `*aikit.APIError` with `StatusCode == 0`.
`server_error` is retryable; `invalid_request_error` is not. Parsed error code,
message, and request ID remain available. Use
`transport.WithErrorBodyCapture(ctx)` to retain the exact SSE error JSON in
`APIError.RawBody`; raw errors and successful responses may contain sensitive
data and must be redacted before logging.

## Prompt caching

For GPT-5.6 and later Responses models, use `PromptCacheKey` with a stable,
cacheable `Instructions` prefix. `PromptCacheInstructions` marks the end of
that prefix; with explicit mode, changing user prompts after it do not replace
the cache entry:

```go
options := openai.ProviderOptions{
  PromptCacheKey:          "support-policy-v1",
  PromptCacheMode:         openai.PromptCacheModeExplicit,
  PromptCacheInstructions: true,
}

response, err := llm.NewCompletion(model, "What should this customer do next?").
  Instructions(stablePolicyOver1024Tokens).
  With(options).
  Send(ctx)
if err != nil {
  return err
}
fmt.Println(response.Usage.InputTokenDetails.CacheReadTokens)
fmt.Println(response.Usage.InputTokenDetails.CacheWriteTokens)
```

The marked prefix must contain at least 1,024 tokens and be identical across
calls. Reuse the same cache key for matching prefixes. The first call normally
writes cache data and later calls can read it; caching remains provider-managed,
so a cache read is not guaranteed. See OpenAI's [prompt-caching guide](https://developers.openai.com/api/docs/guides/prompt-caching)
for retention and billing details.

## Files and mixed output

Upload files through the client because uploading is a provider operation, not
a model capability:

```go
uploaded, err := client.UploadFile(ctx, openai.UploadFileRequest{
  Filename:  "notes.txt",
  Purpose:   openai.FilePurposeUserData,
  Data:      []byte("Important context"),
  MediaType: "text/plain",
})
if err != nil {
  return err
}
```

A Responses completion is not necessarily text-only. If the provider emits
file/image events, direct completion aggregation exposes them through
`CompletionResponse.Files` and also keeps file parts in
`CompletionResponse.Message.Content` in their original order with text and
other assistant content. OpenAI also exposes a dedicated `ImageModel` through
`NewImageModel`, `Client.ImageModel`, and the `openai` registry provider.

## Native image generation and edits

`gpt-image-2` uses OpenAI's synchronous Images API. A request without source
images calls `POST /v1/images/generations`; supplying inline source images calls
the multipart `POST /v1/images/edits` endpoint.

```go
model := openai.NewImageModel("gpt-image-2", openai.Config{
  APIKey: os.Getenv("OPENAI_API_KEY"),
})

result, err := ai.GenerateImage(ctx, ai.GenerateImageRequest{
  Model:  model,
  Prompt: "A ceramic teapot photographed on a cobalt backdrop",
  Size:   "1536x1024",
  ProviderOptions: map[string]any{
    "openai": openai.ImageOptions{
      Quality:      "high",
      OutputFormat: "png",
      Background:   "opaque",
    },
  },
})
```

For edits, add one or more `llm.ImageInput` values with inline `Data` and
`MediaType`. URL-only edit inputs are rejected rather than fetched by the SDK.
`Mask` and model-specific edit controls are available through
`openai.ImageOptions`. For `gpt-image-2`, omit `InputFidelity` because the model
always uses high fidelity, and do not request a transparent background.
Successful responses retain their exact provider JSON in `result.Raw`.

## PDF inputs

Responses models accept PDF input through inline data, an external URL, or an
uploaded file ID. Inline bytes use `file_data` on the wire:

```go
pdf, err := os.ReadFile("report.pdf")
if err != nil {
  return err
}

message := aikit.Message{
  Role: aikit.RoleUser,
  Content: []aikit.ContentPart{
    aikit.TextPart("Summarize the report and explain its charts."),
    aikit.DocumentDataPart(pdf, "application/pdf", "report.pdf"),
  },
}

response, err := llm.NewCompletion(model, "").
  Messages(message).
  With(openai.ProviderOptions{PDFDetail: openai.PDFDetailHigh}).
  Send(ctx)
```

`PDFDetail` accepts `auto`, `low`, or `high` and applies to PDF page-image
processing. Leave it empty to use the API default. Extracted PDF text is always
included; higher detail primarily benefits dense charts, diagrams, and small
print while consuming more input tokens. Prefer `openai.PDFDetailAuto`,
`openai.PDFDetailLow`, or `openai.PDFDetailHigh` over string literals.

For files reused across requests, upload once with `Client.UploadFile` using
`FilePurposeUserData`, then pass the returned ID with
`aikit.DocumentFileIDPart`. `aikit.DocumentURLPart` sends an external URL directly.
Each file or image content part must have exactly one source: inline data, an
external URL, or an uploaded file ID. The provider rejects manually constructed
parts with missing or conflicting sources before sending the request. A
filename is sent only with inline file data; URL and uploaded-ID inputs use
their source field without a filename.
OpenAI currently limits each file and the combined files in one request to 50
MB; see the [file-input guide](https://developers.openai.com/api/docs/guides/file-inputs)
for current limits and processing details.

## Compatibility

`openai.NewLanguageModel` and `openai.NewChatLanguageModel` remain available for
existing applications. They retain their previous signatures and deferred
configuration-error behavior. New code should use `NewClient`, which validates
eagerly and shares provider resources across model handles. The legacy
`LanguageModel.UploadFile` method also remains as a forwarding compatibility
method; prefer `Client.UploadFile` in new code.

See [providers and clients](/core/providers-and-clients) for the capability
model and generic provider infrastructure.
