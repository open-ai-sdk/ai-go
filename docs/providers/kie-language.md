# Kie language models

`provider/kie` supports explicit Responses and Chat Completions endpoints through
`llm.Model` and the optional native `llm.CompletionModel` interface. Model IDs,
endpoint paths, prompts, schemas, reasoning effort, and native tools are supplied
by the caller. No model-name routing is performed.

## Construct a model

```go
provider := kie.NewProvider(os.Getenv("KIE_API_KEY"), kie.WithConfig(kie.Config{
    BaseURL: "https://api.kie.ai",
    Timeout: 90 * time.Second,
}))

responses := provider.Responses("configured-model",
    kie.WithLanguagePath("configured/v1/responses"))
chat := provider.Chat("configured-model", "configured-model/v1/chat/completions")
```

`Responses` defaults to the generic `responses` path, not a model-specific Kie
route. `Chat` requires an explicit path. `WithLanguagePath` can override either
handle's endpoint. Paths are resolved beneath `Config.BaseURL`, preserving any
base path. Language endpoints do not use the media API's proxy-path rewriting.
The provider's authentication, headers, injected HTTP client, and timeout apply.
Existing image/video defaults are unchanged.

## Complete a request

```go
request := llm.Request{
    Instructions: "Answer using the supplied context.",
    Messages: []aikit.Message{{
        Role: aikit.RoleUser,
        Content: []aikit.ContentPart{
            aikit.TextPart("Describe this image."),
            aikit.ImageURLPart("https://example.test/image.png"),
        },
    }},
    Output: &llm.OutputSchema{
        Name: "description",
        Type: "object",
        Schema: map[string]any{
            "type": "object",
            "properties": map[string]any{"text": map[string]any{"type": "string"}},
            "required": []string{"text"},
            "additionalProperties": false,
        },
    },
    ProviderOptions: map[string]any{"kie": kie.LanguageOptions{
        ReasoningEffort: "medium",
        NativeTools: []map[string]any{{"type": "web_search"}},
    }},
}
response, err := llm.Complete(ctx, responses, request)
```

`kie.LanguageOptions` also works with `llm.NewCompletion(...).With(...)`.
The JSON-map equivalent uses `reasoningEffort`, `schemaName`, and `nativeTools`.
Unknown JSON-map option keys and incorrect option types are rejected by the
shared provider-option decoder.

| Control | Responses wire | Chat wire |
| --- | --- | --- |
| `ReasoningEffort` | `reasoning.effort` | `reasoning_effort` |
| `Output.Name` | `text.format.name` | `response_format.json_schema.name` |
| `SchemaName` | Overrides `Output.Name` | Overrides `Output.Name` |
| `NativeTools` | Native objects in `tools` | Native objects in `tools` |
| `Request.Tools` | Flat function definitions | Nested function definitions |

Structured output sends `strict: true`, but the SDK does not rewrite the supplied
schema or add required properties. An unnamed schema uses the compatible
encoder's generic `structured_output` name. `json_object` and plain text outputs
are also supported.

Responses sends `model` plus role/content-array `input` entries without a
message `type` field. It does not send `stream`, `store`, or `include` fields.
Chat sends typed `messages` content arrays, `stream: false`, and
`include_thoughts: false`; it does not send `model` in the body. Images use
`input_image` with a URL string for Responses and `image_url` with a nested URL
object for Chat. Function calls and tool results can be passed in normalized
message history.

Native tools are preserved as supplied, independently of callable function
tools. To request a gateway's Chat search capability without an unwanted
`parameters` member:

```go
request.ProviderOptions["kie"] = kie.LanguageOptions{
    NativeTools: []map[string]any{{
        "type": "function",
        "function": map[string]any{"name": "googleSearch"},
    }},
}
response, err = llm.Complete(ctx, chat, request)
```

Only pass tools supported by the selected upstream route. The SDK does not
choose search tools or convert one provider-native tool into another.

## Raw response, usage, and errors

`CompletionResponse` provides normalized text, reasoning, assistant message,
function calls, finish reason, and token usage. `Usage.Raw` retains every field
of the provider's usage object, including unknown fields.

```go
raw, ok := llm.RawResponseAs[map[string]any](response)
```

For JSON replies, `raw` is the entire provider JSON object. Some Responses
routes return SSE even though no streaming flag was sent. These are accepted;
`raw` is `map[string]any{"events": []any{...}}`, retaining all JSON event payloads
in order, not only the final response. The terminal `response.completed` payload
supplies normalized text and usage. SSE comments and `[DONE]` markers are not
JSON events and are excluded.

Failed/incomplete responses, refusals, missing assistant content, and missing
terminal SSE payloads return a classified `*llm.CompletionError`. A decoded
failure also returns a partial completion containing its full raw response.
HTTP failures wrap `*aikit.APIError`, retaining status, provider code/message,
and request ID. Enable raw error-body diagnostics explicitly:

```go
ctx = transport.WithErrorBodyCapture(ctx)
response, err = llm.Complete(ctx, responses, request)
var apiError *aikit.APIError
if errors.As(err, &apiError) {
    // apiError.StatusCode and apiError.RawBody are available for diagnostics.
}
```

Raw request/response values and raw error bodies can contain sensitive data.
They are never logged automatically. `APIError.Error()` does not print the raw
body. Use `transport.WithResponseObserver` if HTTP status and response headers
are needed before decoding.

`model.RequestBody(request)` returns the exact pre-serialization request object
used by `Complete`, without network I/O, for application-owned diagnostics. Its
nested schema/native-tool values may refer to the input request; do not mutate
them, and redact private prompts and URLs before recording them.

## Stream contract

Both handles implement `llm.Model.Stream` by making the same nonstreaming
completion request and emitting its normalized content, usage, and finish events.
This is not incremental upstream streaming: events begin after completion.
Context cancellation stops event delivery and closes the channel. Use native
`Complete` (including `llm.Complete` or `NewCompletion(...).Send`) when the full
raw response map is required.
