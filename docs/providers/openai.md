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

## PDF inputs

Responses models accept PDF input through inline data, an external URL, or an
uploaded file ID. Inline bytes use `file_data` on the wire:

```go
pdf, err := os.ReadFile("report.pdf")
if err != nil {
  return err
}

message := ai.Message{
  Role: ai.RoleUser,
  Content: []ai.ContentPart{
    ai.TextPart("Summarize the report and explain its charts."),
    ai.DocumentDataPart(pdf, "application/pdf", "report.pdf"),
  },
}

response, err := ai.NewCompletion(model, "").
  Messages(message).
  With(openai.ProviderOptions{PDFDetail: openai.PDFDetailHigh}).
  Send(ctx)
```

`PDFDetail` accepts `auto`, `low`, or `high` and applies to PDF page-image
processing. Leave it empty to use the API default. Extracted PDF text is always
included; higher detail primarily benefits dense charts, diagrams, and small
print while consuming more input tokens. Prefer `openai.PDFDetailAuto`,
`openai.PDFDetailLow`, or `openai.PDFDetailHigh` over string literals.

For files reused across requests, upload once with `Client.UploadFile` using
`FilePurposeUserData`, then pass the returned ID with
`ai.DocumentFileIDPart`. `ai.DocumentURLPart` sends an external URL directly.
Each file or image content part must have exactly one source: inline data, an
external URL, or an uploaded file ID. The provider rejects manually constructed
parts with missing or conflicting sources before sending the request. A
filename is sent only with inline file data; URL and uploaded-ID inputs use
their source field without a filename.
OpenAI currently limits each file and the combined files in one request to 50
MB; see the [file-input guide](https://developers.openai.com/api/docs/guides/file-inputs)
for current limits and processing details.

## Compatibility

`openai.NewLanguageModel` and `openai.NewChatLanguageModel` remain available for
existing applications. They retain their previous signatures and deferred
configuration-error behavior. New code should use `NewClient`, which validates
eagerly and shares provider resources across model handles. The legacy
`LanguageModel.UploadFile` method also remains as a forwarding compatibility
method; prefer `Client.UploadFile` in new code.

See [providers and clients](/core/providers-and-clients) for the capability
model and generic provider infrastructure.
