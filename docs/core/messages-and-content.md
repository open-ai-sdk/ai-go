# Messages and content

`ai.Message` is the provider-neutral conversation unit. It has a role, ordered
content parts, and an optional provider-issued `ID` for assistant messages.
Preserve the complete assistant message when continuing a conversation: tool
call IDs, reasoning signatures, content order, and the message ID may all be
required by the provider.

## Roles and validation

Use `ai.UserMessage`, `ai.AssistantMessage`, and `ai.SystemMessage` for the
common text-only case. For mixed content, construct a message explicitly and
call `Validate` at application boundaries:

```go
message := ai.Message{
  Role: ai.RoleUser,
  Content: []ai.ContentPart{
    ai.TextPart("Summarize this report"),
    ai.DocumentDataPart(pdf, "application/pdf", "report.pdf"),
  },
}
if err := message.Validate(); err != nil {
  return err
}
```

Validation rejects empty messages, assistant-only content on user/tool roles,
malformed media parts, and message IDs on non-assistant roles. Role-specific
accessors return `ai.ErrWrongMessageRole` instead of silently interpreting the
wrong message shape.

## Explicit media kinds

Images, audio, documents, and video each have URL, inline-data, and provider
file-ID constructors. `FilePart`, `FileDataPart`, and `FileIDPart` remain as
legacy generic file constructors. Prefer the explicit kind so provider
adapters can translate or reject content deterministically.

Provider and model capabilities differ. A normalized content kind means the
SDK can represent it; it does not guarantee that every backend accepts it.

## Assistant content

Assistant content may contain text, reasoning, and tool calls in one ordered
slice. A direct completion returns this as `CompletionResponse.Message`; agent
steps place it in continuation messages. The provider message ID appears both
as `CompletionResponse.MessageID` and `Message.ID`.

## Tool results

`ToolResultPart` carries literal string output. Use `RichToolResultPart` for
ordered explicit text, JSON, or image content. JSON-looking text stays text;
call `JSONToolResultContent` or `ParseToolResultJSON` to opt into structured
JSON semantics.

## Ownership

`Message.Clone`, `ContentPart.Clone`, and `ToolResult.Clone` copy mutable byte
slices and raw JSON. Request builders, continuation history, callbacks, and
partial error snapshots use these ownership rules so later caller mutation
does not rewrite stored conversation state.
