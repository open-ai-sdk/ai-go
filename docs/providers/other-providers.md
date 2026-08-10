# Other providers

`ai-go` ships focused provider packages plus an additive capability registry.

- **Anthropic:** create a language model with `anthropic.NewLanguageModel`, or create an `anthropic.Provider` and select its model.
- **Gemini:** `gemini.NewLanguageModel` provides the compatible language-model path. The package also includes native language models plus embedding and image model constructors for Gemini-specific capabilities.
- **Kie:** `kie.NewProvider(apiKey).Image(modelID)` preserves the typed compatibility API. `ImageModel(string)` supports registry use, and `NewFromEnv()` reads `KIE_API_KEY`.
- **OpenAI-compatible APIs:** use `openaicompat.NewModel` with its explicit `Config`, including a compatibility provider, model ID, and endpoint/auth configuration.

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
