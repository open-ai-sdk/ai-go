# Media generation

Media generation uses a capability-specific model instead of overloading the
language-model interface. An `ai.ImageModel` generates complete image blobs:

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
