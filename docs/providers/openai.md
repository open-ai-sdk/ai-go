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

Keep provider configuration at client creation and request-specific options on
the request.

## Responses payload

`client.CompletionModel` implements the optional native completion capability.
A direct `Send` therefore returns normalized assistant content and retains the
successful OpenAI Responses payload for provider-specific diagnostics:

```go
response, err := ai.NewCompletion(model, "Explain Go interfaces").Send(ctx)
if err != nil {
  return err
}

native, ok := ai.RawResponseAs[*openai.ResponsesResponse](response)
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
other assistant content. OpenAI does not yet expose a dedicated `ImageModel`
factory in ai-go.

## Compatibility

`openai.NewLanguageModel` and `openai.NewChatLanguageModel` remain available for
existing applications. They retain their previous signatures and deferred
configuration-error behavior. New code should use `NewClient`, which validates
eagerly and shares provider resources across model handles. The legacy
`LanguageModel.UploadFile` method also remains as a forwarding compatibility
method; prefer `Client.UploadFile` in new code.

See [providers and clients](/core/providers-and-clients) for the capability
model and generic provider infrastructure.
