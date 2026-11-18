# OpenRouter

`provider/openrouter` is a thin layer over `provider/openaicompat` for
[OpenRouter](https://openrouter.ai)'s Chat Completions API.

```go
model := openrouter.NewLanguageModel("openai/gpt-4o-mini", openrouter.Config{
  APIKey:  os.Getenv("OPENROUTER_API_KEY"),
  Referer: "https://app.example", // optional attribution headers
  Title:   "Example app",
})
```

`openrouter.NewProvider(cfg)` and `openrouter.NewFromEnv()` (reads
`OPENROUTER_API_KEY`) register under the name `openrouter`.

## Options

Pass `openrouter.ProviderOptions` through the request:

```go
result, err := llm.NewCompletion(model, "Summarize this").
  With(openrouter.ProviderOptions{
    Reasoning: &openrouter.Reasoning{Effort: "high"},
    Provider: &openrouter.Routing{
      Order:             []string{"ExampleHost"},
      RequireParameters: ptr(true),
    },
    Models: []string{"example/fallback-model"},
  }).
  Send(ctx)
```

| Option | Wire field |
| --- | --- |
| `Reasoning` | `reasoning` |
| `Provider` | `provider` (routing preferences) |
| `Models`, `Transforms` | `models`, `transforms` |
| `IncludeUsage` | `usage: {include: true}`; on unless set to `false` |
| `ParallelToolCalls` | `parallel_tool_calls`, only with tools |
| `User` | `user` |

Structured output uses `response_format` with a strict `json_schema`.

## Usage cost

With usage accounting on, the final stream chunk carries a full usage object.
ai-go keeps it unmodified in `aikit.Usage.Raw`; `openrouter.Cost(usage)` reads
the provider-reported `cost`. It is the figure OpenRouter bills, not a computed
price. `Raw` also keeps `cost_details` and any other fields the provider adds.

## Stream behavior

OpenRouter sends `: OPENROUTER PROCESSING` keepalive comments. They are bytes,
so `Config.ChunkTimeout` (a byte-level read timer) is never triggered by them.
Use `llm.WithStallTimeout` to bound the time between decoded events, and
`llm.WithRetry` to retry failures that happen before any content arrives (see
[Middleware](/core/middleware)). Errors reported inside the event stream
(`data: {"error":{...}}`) end the stream with an error event. The finish event
carries `ProviderMetadata["openrouter"]` with `responseId` and `model`.

## Responses API

Responses is an explicit alternative to the default Chat Completions model:

```go
model := openrouter.NewResponsesModel("openai/gpt-5", openrouter.Config{
  APIKey: os.Getenv("OPENROUTER_API_KEY"),
  Referer: "https://app.example",
  Title: "Example app",
})

response, err := llm.NewCompletion(model, "Summarize this").
  With(openai.ProviderOptions{
    IncludeEncryptedReasoning: true,
    ExplicitMessageType: true,
  }).
  Send(ctx)
```

`NewProvider(cfg).ResponsesModel(id)` creates the same model handle.
`Config.BaseURL`, credentials, attribution/custom headers, timeouts, and injected
HTTP client apply to both APIs. Responses requests target `responses` beneath
the base URL and use the generic OpenAI encoder, decoder, and
`openai.ProviderOptions` key. OpenRouter's Chat Completions routing options do
not apply to this model. No HTTP request rewriting is needed.

Responses finish metadata uses the `openai` namespace and retains response/model
identifiers, the server request ID, and exact terminal response JSON; native
non-streaming completion returns `*openai.ResponsesResponse`. See the
[OpenAI Responses controls](/providers/openai#optional-responses-wire-controls)
for history item IDs, schema names, and optional wire presentation.
