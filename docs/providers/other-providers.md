# Other providers

`ai-go` ships focused provider packages plus an additive capability registry.

- **Anthropic:** create a language model with `anthropic.NewLanguageModel`, or create an `anthropic.Provider` and select its model.
- **Gemini:** `gemini.NewLanguageModel` provides the compatible language-model path. The package also includes native language models plus embedding and image model constructors for Gemini-specific capabilities.
- **Kie:** `kie.NewProvider(apiKey).Image(modelID)` preserves the typed compatibility API. `ImageModel(string)` supports registry use, and `NewFromEnv()` reads `KIE_API_KEY`. `CreateTask`/`TaskStatus` expose the task API, and `Video(modelID)` adds video models.
- **OpenRouter:** `openrouter.NewLanguageModel` (see [OpenRouter](/providers/openrouter)).
- **OpenAI-compatible APIs:** use `openaicompat.NewModel` with its explicit `Config`, including a compatibility provider, model ID, and endpoint/auth configuration. `openaicompat.NewEmbeddingModel` covers `/embeddings`.

All models are used through the same `ai` facade. Select a model type that matches the operation: language models for text and objects, embedding models for `Embed`, and image models for image generation.

```go
registry := ai.NewRegistry()
_ = registry.Register(openai.NewFromEnv())
_ = registry.Register(anthropic.NewFromEnv())
_ = registry.Register(gemini.NewFromEnv())
_ = registry.Register(kie.NewFromEnv())
```

KIE Seedream 4.0 uses the exact upstream IDs
`bytedance/seedream-v4-text-to-image` and `bytedance/seedream-v4-edit`.

## Kie jobs wire controls

`Config.BaseURL` retains its proxy-mirror routing by default. Set
`DirectBaseURL: true` to append upstream paths such as
`/api/v1/jobs/createTask` directly to that base URL instead.

For durable workflows that classify acceptance themselves, use
`Provider.CreateTaskReply` / `TaskStatusReply`, or
`VideoModel.SubmitVideoReply` / `VideoStatusReply`. These submit or query
exactly once, without retries or envelope normalization. `JobsReply` exposes
the received `StatusCode`, untranslated `Body`, and separate `ReadError`.
This preserves the distinction between a transport failure, a received HTTP
rejection, and an unreadable reply after a potentially accepted submission.
Input fields retain their JSON types, including numeric durations.

Typed receipt decoders are available when untranslated bytes are not needed:
`DecodeImageSubmission` and `DecodeImageStatus` preserve optional numeric image
failure codes and object/string `resultJson`; `DecodeVideoSubmission` and
`DecodeVideoStatus` classify jobs acceptance and verified video task results.
Video receipts retain HTTP status, numeric-or-string failure codes and credits.
`JobsSubmitError` distinguishes rejected from indeterminate submissions;
`JobsQueryError` identifies provider contract violations. Applications can map
these SDK receipts and errors into their own business DTOs without parsing
provider envelopes.

Raw reply controls are opt-in: `MaxReplyBytes` bounds the body,
`RequireUTF8Reply` checks its encoding, and `DisableRedirects` prevents
redirect following on the SDK-created client. Existing typed jobs and model
methods retain their defaults.

`llm.NewResultDownloader` accepts explicit `ResultDownloadOptions`: result
host allowlist, byte cap, timeout, and optional headers. It checks every DNS
answer, pins the connection to those answers, bypasses proxies, refuses
redirects and credentials, and distinguishes refused results from transient
failures with `ResultDownloadError`. There are no application headers or
host defaults. The explicit loopback-HTTP option is intended for local
endpoints; otherwise only HTTPS with public addresses is accepted.
