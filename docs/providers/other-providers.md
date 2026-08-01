# Other providers

`ai-go` ships focused provider packages rather than a global provider registry.

- **Anthropic:** create a language model with `anthropic.NewLanguageModel`, or create an `anthropic.Provider` and select its model.
- **Gemini:** `gemini.NewLanguageModel` provides the compatible language-model path. The package also includes native language models plus embedding and image model constructors for Gemini-specific capabilities.
- **Kie:** `kie.NewProvider(apiKey).Image(modelID)` creates an image model. The provider can also read `KIE_API_KEY` when no key is supplied.
- **OpenAI-compatible APIs:** use `openaicompat.NewModel` with its explicit `Config`, including a compatibility provider, model ID, and endpoint/auth configuration.

All models are used through the same `ai` facade. Select a model type that matches the operation: language models for text and objects, embedding models for `Embed`, and image models for image generation.
