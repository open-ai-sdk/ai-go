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
response for diagnostics. It may contain provider-specific or sensitive data,
so applications should not log it automatically.

`Images` supplies source images for editing through inline bytes or a URL.
`Size`, `AspectRatio`, `Seed`, and `ProviderOptions` express common and
provider-specific controls. Support for each field depends on the selected
provider and model; inspect `Warnings` when a provider reports a normalized
compatibility issue.

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
| `kie` | No | Yes, asynchronous task API with polling | `kie.NewFromEnv()` (`KIE_API_KEY`) |

KIE includes the exact Seedream 4.0 Market IDs
`bytedance/seedream-v4-text-to-image` and `bytedance/seedream-v4-edit`. The
typed constants are `kie.ModelSeedreamV4TextToImage` and
`kie.ModelSeedreamV4Edit`. Seedream options map to `image_size`,
`image_resolution`, `max_images`, `seed`, and `nsfw_checker`; edits additionally
send source URLs as `image_urls`. See KIE's official
[text-to-image](https://docs.kie.ai/market/seedream/seedream-v4-text-to-image)
and [edit](https://docs.kie.ai/market/seedream/seedream-v4-edit) contracts.
