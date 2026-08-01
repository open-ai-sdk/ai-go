# OpenAI

Use `openai.NewLanguageModel` for the OpenAI Responses API:

```go
model := openai.NewLanguageModel("gpt-5", openai.Config{
  APIKey: os.Getenv("OPENAI_API_KEY"),
})
```

Pass typed OpenAI options alongside a generation request when needed:

```go
result, err := ai.GenerateText(ctx, ai.GenerateTextRequest{
  Model:    model,
  Messages: []ai.Message{ai.UserMessage("Explain this code")},
  ProviderOptions: map[string]any{
    "openai": openai.ProviderOptions{ReasoningEffort: "low"},
  },
})
```

For an OpenAI Chat Completions model, use `openai.NewChatLanguageModel`. Keep provider configuration at model creation and request-specific options on the request.
