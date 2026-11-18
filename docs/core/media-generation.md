# Media generation

Media generation uses a capability-specific model instead of overloading the
language-model interface. An `llm.ImageModel` generates complete image blobs:

```go
type ImageModel interface {
  ModelID() string
  Generate(context.Context, GenerateImageRequest) (*GenerateImageResult, error)
}
```

Generate an image with the provider-neutral request:

```go
result, err := ai.GenerateImage(ctx, ai.GenerateImageRequest{
  Model:       model,
  Prompt:      "A quiet library in watercolor",
  N:           1,
  AspectRatio: "16:9",
})
if err != nil {
  return err
}

image := result.Images[0]
fmt.Println(image.MediaType, len(image.Bytes()))
```

`GenerateImageResult.Raw` retains an exact copy of the successful provider
response for diagnostics. `ResponseHeaders`, when supplied by a provider,
retains successful response headers such as request identifiers. Both may contain
sensitive data, so applications should not log them automatically.

`Images` supplies source images for editing through inline bytes or a URL.
`Size`, `AspectRatio`, `Seed`, and `ProviderOptions` express common and
provider-specific controls. Support for each field depends on the selected
provider and model; inspect `Warnings` when a provider reports a normalized
compatibility issue.

An inline `llm.ImageInput` may specify `FileName` to preserve its multipart
filename, including on masks. OpenAI synthesizes its existing filename when the
field is empty; inline data and content type are otherwise unchanged.

Image output can also appear as file events from a multimodal language model.
That remains a language-model completion and is exposed through
`CompletionResponse.Files`; a dedicated `ImageModel` is used when image
generation itself is the requested operation.

Provider clients expose only the model capabilities they implement. See
[Providers and clients](/core/providers-and-clients) for capability discovery
and [provider integrations](/providers/) for concrete model constructors.

## Provider registry

Applications that select providers and model IDs from configuration or a
database can use the additive registry instead of a provider switch:

```go
registry := ai.NewRegistry()
if err := registry.Register(openai.NewFromEnv()); err != nil {
  return err
}
if err := registry.Register(kie.NewFromEnv()); err != nil {
  return err
}

model, err := registry.ImageModel("openai", "gpt-image-2")
if err != nil {
  return err
}
```

Registration uses each provider's stable `Name`. Looking up a missing provider
or a capability it does not implement returns a registry error. Existing
provider-specific constructors remain available.

## Provider support

| Provider registry name | Language | Image generation | Environment constructor |
| --- | --- | --- | --- |
| `openai` | Yes | Yes, synchronous Images API | `openai.NewFromEnv()` (`OPENAI_API_KEY`) |
| `anthropic` | Yes | No | `anthropic.NewFromEnv()` (`ANTHROPIC_API_KEY`) |
| `gemini` | Yes | Yes, synchronous native API | `gemini.NewFromEnv()` (`GEMINI_API_KEY`) |
| `openrouter` | Yes | Yes, synchronous JSON Images API | `openrouter.NewFromEnv()` (`OPENROUTER_API_KEY`) |
| `kie` | No | Yes, asynchronous task API with polling | `kie.NewFromEnv()` (`KIE_API_KEY`) |

Kie also provides video models (`llm.VideoModel`, `llm.VideoProvider`); see
[Kie tasks and video](#kie-tasks-and-video).

KIE includes Seedream 3.0, 4.0, 4.5, and 5.0 Lite/Pro model IDs. Their option
sets differ by upstream family: Seedream 3 uses `image_size` and
`guidance_scale`; Seedream 4 uses `image_size`, `image_resolution`, and
`max_images`; Seedream 4.5/5 use `aspect_ratio` and `quality`. Edits send
source URLs as `image_urls`. `kie.ModelSeedreamV5ProLayerDecomposition`
requires exactly one source image and supports layer-specific `size` and
`output_format`. See KIE's official
[text-to-image](https://docs.kie.ai/market/seedream/seedream-v4-text-to-image)
and [edit](https://docs.kie.ai/market/seedream/seedream-v4-edit) contracts.

## OpenAI and OpenRouter images

OpenAI uses `images/generations` for JSON generation and `images/edits` for
multipart edits. `openai.ImageOptions.ResponseFormat` optionally overrides the
automatic `response_format` field; a pointer to `""` omits it for custom models.
`FirstImageOnly` selects the first row containing `b64_json`, skipping rows
without that field. Its default remains false: every image is validated and
returned.

OpenRouter exposes `openrouter.NewImageModel(modelID, Config)` and the provider
registry's `ImageModel` capability. Both generation and editing call
`POST {BaseURL}/images`. Include `/v1` in a custom API root when the upstream
requires it. Inline source images become `input_references` entries with
`type: "image_url"` and base64 data URLs; URL inputs are forwarded directly.

```go
model := openrouter.NewImageModel("vendor/image-model", openrouter.Config{
  APIKey: apiKey,
})
result, err := model.Generate(ctx, llm.GenerateImageRequest{
  Prompt: "A lighthouse in watercolor",
  Size: "1024x1024",
  N: 1,
  ProviderOptions: map[string]any{
    "openrouter": openrouter.ImageOptions{Quality: "high", OutputFormat: "png"},
  },
})
```

OpenRouter accepts `Quality`, `Background`, and `OutputFormat` image options.
`FirstImageOnly` selects the first available image; otherwise all rows are
decoded. Responses may contain base64 image bytes or downloadable `url` results.
Downloads reuse the configured HTTP doer without adding API credentials or
attribution headers. Only use a doer that is safe for the returned image hosts.
`ResponseMaxBytes` bounds the JSON response (64 MiB by default), not downloaded
image bytes. `Usage.Raw` preserves the provider's usage object, including cost and
token details; the complete `Raw` response also retains non-object usage values.

## Downloading results

`llm.DownloadImage` downloads a trusted image URL and returns its bytes and
Content-Type. `ImageDownloadOptions` supports request headers, timeout, and an
optional HTTP doer; it does not validate destination hosts or bound result size.
For untrusted result URLs use `llm.NewResultDownloader` and its explicit host,
address, byte-limit, and timeout policies instead. See the
[other provider integration](/providers/other-providers) for Kie job controls.


## Kie tasks and video

Kie jobs are asynchronous. Besides the polling `Generate` calls, the provider
exposes the two underlying calls so a caller can persist the task id and poll
from a different process:

```go
p := kie.NewProvider(apiKey)
taskID, err := p.CreateTask(ctx, "vendor/model-id", map[string]any{"prompt": "a lighthouse"})
// later, possibly elsewhere:
rec, err := p.TaskStatus(ctx, taskID)
if rec.State.Terminal() { /* rec.ResultURLs, rec.FailCode, rec.FailMsg, rec.Raw */ }
```

`TaskRecord.State` is normalized (`waiting`, `queuing`, `generating`,
`success`, `fail`; anything else is `unknown`) and `RawState` keeps the
provider's own string.

Model ids without a typed builder are not rejected. Set
`ImageOptions.RawInput` (or `VideoOptions.RawInput`) and the request is sent as
`{model, input}` with `prompt` and `image_urls` added when `RawInput` does not
already set them. Typed validation still applies to the listed Seedream models;
for those, use the existing typed options.

Video generation implements `llm.VideoModel`:

```go
model := p.Video("vendor/video-model-id")
result, err := model.GenerateVideo(ctx, llm.GenerateVideoRequest{
  Prompt: "waves at dusk", Duration: 5 * time.Second, AspectRatio: "16:9",
})
// result.URLs are provider-hosted; ai-go does not download the video.
```

`SubmitVideo` and `VideoStatus` give the same create/status split for video.
