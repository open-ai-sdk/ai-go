package aisdk

import "strings"

// Constructors for every member of the client's uiMessageChunkSchema.
//
// Why constructors rather than composing Chunk literals: a required field that is
// missing is usually invisible. The client's Zod gate catches some of it, but its
// looseObject members silently accept extra keys, its tool lookup falls back to a
// whole-message reverse scan instead of throwing, and unknown chunk types hit an
// ignoring default branch. So a malformed chunk frequently renders wrong rather than
// failing — which is far harder to diagnose than a compile error. Taking the required
// fields as positional parameters makes the failure impossible to express.
//
// Optional fields go through ChunkOption. Present-and-false is distinguishable from
// absent, because the option sets a map key and no option leaves it out entirely.

// ChunkOption sets an optional field on a chunk under construction.
type ChunkOption func(map[string]any)

func applyOptions(f map[string]any, opts []ChunkOption) map[string]any {
	for _, o := range opts {
		if o != nil {
			o(f)
		}
	}
	return f
}

// WithProviderMetadata attaches providerMetadata. A nil or empty map is dropped
// rather than emitted as {}, since an empty object is not the same as absent to a
// client reading it.
func WithProviderMetadata(pm map[string]any) ChunkOption {
	return func(f map[string]any) {
		if len(pm) > 0 {
			f["providerMetadata"] = pm
		}
	}
}

// WithToolMetadata attaches toolMetadata.
func WithToolMetadata(tm map[string]any) ChunkOption {
	return func(f map[string]any) {
		if len(tm) > 0 {
			f["toolMetadata"] = tm
		}
	}
}

// WithProviderExecuted marks a tool as executed by the provider rather than by this
// server. Set explicitly, including to false, because absent and false are different
// signals: absent means "not stated", false means "definitely ours".
func WithProviderExecuted(b bool) ChunkOption {
	return func(f map[string]any) { f["providerExecuted"] = b }
}

// WithDynamic marks a tool whose schema is not known at build time.
func WithDynamic(b bool) ChunkOption {
	return func(f map[string]any) { f["dynamic"] = b }
}

// WithTitle sets a human-readable tool title.
func WithTitle(s string) ChunkOption {
	return func(f map[string]any) { f["title"] = s }
}

// WithPreliminary marks a tool output as not final, so the client can render a
// partial result and expect it to be replaced.
func WithPreliminary(b bool) ChunkOption {
	return func(f map[string]any) { f["preliminary"] = b }
}

// WithMessageMetadata attaches messageMetadata to start/finish.
func WithMessageMetadata(md any) ChunkOption {
	return func(f map[string]any) {
		if md != nil {
			f["messageMetadata"] = md
		}
	}
}

// --- text ---

func TextStart(id string, opts ...ChunkOption) Chunk {
	return Chunk{ChunkTextStart, applyOptions(map[string]any{"id": id}, opts)}
}

func TextDeltaChunk(id, delta string, opts ...ChunkOption) Chunk {
	return Chunk{ChunkTextDelta, applyOptions(map[string]any{"id": id, "delta": delta}, opts)}
}

func TextEnd(id string, opts ...ChunkOption) Chunk {
	return Chunk{ChunkTextEnd, applyOptions(map[string]any{"id": id}, opts)}
}

// --- reasoning ---

func ReasoningStart(id string, opts ...ChunkOption) Chunk {
	return Chunk{ChunkReasoningStart, applyOptions(map[string]any{"id": id}, opts)}
}

func ReasoningDeltaChunk(id, delta string, opts ...ChunkOption) Chunk {
	return Chunk{ChunkReasoningDelta, applyOptions(map[string]any{"id": id, "delta": delta}, opts)}
}

func ReasoningEnd(id string, opts ...ChunkOption) Chunk {
	return Chunk{ChunkReasoningEnd, applyOptions(map[string]any{"id": id}, opts)}
}

// --- tool input ---

func ToolInputStart(toolCallID, toolName string, opts ...ChunkOption) Chunk {
	return Chunk{ChunkToolInputStart, applyOptions(map[string]any{
		"toolCallId": toolCallID, "toolName": toolName,
	}, opts)}
}

// ToolInputDelta carries a partial-JSON fragment. It takes no options: the schema
// member has exactly two fields and accepts nothing else.
func ToolInputDelta(toolCallID, inputTextDelta string) Chunk {
	return Chunk{ChunkToolInputDelta, map[string]any{
		"toolCallId": toolCallID, "inputTextDelta": inputTextDelta,
	}}
}

// ToolInputAvailable completes a tool call's input.
//
// input is written even when nil, as an explicit JSON null. The client's schema types
// it z.unknown() and so accepts the field being missing entirely, but a consumer
// reading the persisted history cannot tell "no input" from "field forgotten" if it is
// absent. Emitting null states it.
func ToolInputAvailable(toolCallID, toolName string, input any, opts ...ChunkOption) Chunk {
	return Chunk{ChunkToolInputAvailable, applyOptions(map[string]any{
		"toolCallId": toolCallID, "toolName": toolName, "input": input,
	}, opts)}
}

// ToolInputError reports malformed tool input. errorText is redacted.
func ToolInputError(toolCallID, toolName string, input any, err error, opts ...ChunkOption) Chunk {
	return Chunk{ChunkToolInputError, applyOptions(map[string]any{
		"toolCallId": toolCallID, "toolName": toolName, "input": input,
		"errorText": redactStreamError(err),
	}, opts)}
}

// --- tool approval ---

func ToolApprovalRequest(approvalID, toolCallID string, opts ...ChunkOption) Chunk {
	return Chunk{ChunkToolApprovalRequest, applyOptions(map[string]any{
		"approvalId": approvalID, "toolCallId": toolCallID,
	}, opts)}
}

// WithIsAutomatic marks an approval the server resolved without asking a human.
func WithIsAutomatic(b bool) ChunkOption {
	return func(f map[string]any) { f["isAutomatic"] = b }
}

// WithApprovalSignature attaches the HMAC that proves this server issued the request.
func WithApprovalSignature(sig string) ChunkOption {
	return func(f map[string]any) {
		if sig != "" {
			f["signature"] = sig
		}
	}
}

// ToolApprovalResponseChunk emits the server's view of an approval decision.
//
// Named with the Chunk suffix because ToolApprovalResponse is already taken by the
// inbound envelope type in chat-request-envelope.go. They are opposite directions of
// the same concept and the inbound one is rewritten in a later phase, so this avoids
// renaming a type other code still parses into.
func ToolApprovalResponseChunk(approvalID string, approved bool, opts ...ChunkOption) Chunk {
	return Chunk{ChunkToolApprovalResponse, applyOptions(map[string]any{
		"approvalId": approvalID, "approved": approved,
	}, opts)}
}

// WithApprovalReason explains a decision.
func WithApprovalReason(reason string) ChunkOption {
	return func(f map[string]any) {
		if reason != "" {
			f["reason"] = reason
		}
	}
}

// --- tool output ---

func ToolOutputAvailable(toolCallID string, output any, opts ...ChunkOption) Chunk {
	return Chunk{ChunkToolOutputAvailable, applyOptions(map[string]any{
		"toolCallId": toolCallID, "output": output,
	}, opts)}
}

// ToolOutputError reports a tool failure. errorText is redacted, so a provider error
// or a recovered panic value cannot reach the browser.
func ToolOutputError(toolCallID string, err error, opts ...ChunkOption) Chunk {
	return Chunk{ChunkToolOutputError, applyOptions(map[string]any{
		"toolCallId": toolCallID, "errorText": redactStreamError(err),
	}, opts)}
}

// ToolOutputDenied reports a tool the user refused. No options: the schema member has
// exactly one field.
func ToolOutputDenied(toolCallID string) Chunk {
	return Chunk{ChunkToolOutputDenied, map[string]any{"toolCallId": toolCallID}}
}

// --- sources and files ---
//
// No Eino content block maps to these four families, so nothing produces them today.
// The constructors exist for protocol completeness and because a caller emitting its
// own citations or data parts is a plausible use, but no phase requires emitting them.

func SourceURL(sourceID, url string, opts ...ChunkOption) Chunk {
	return Chunk{ChunkSourceURL, applyOptions(map[string]any{
		"sourceId": sourceID, "url": url,
	}, opts)}
}

// WithSourceTitle sets the optional title on source-url. source-document requires a
// title, so it takes one positionally instead.
func WithSourceTitle(title string) ChunkOption {
	return func(f map[string]any) {
		if title != "" {
			f["title"] = title
		}
	}
}

func SourceDocument(sourceID, mediaType, title string, opts ...ChunkOption) Chunk {
	return Chunk{ChunkSourceDocument, applyOptions(map[string]any{
		"sourceId": sourceID, "mediaType": mediaType, "title": title,
	}, opts)}
}

// WithFilename sets the optional filename on source-document.
func WithFilename(name string) ChunkOption {
	return func(f map[string]any) {
		if name != "" {
			f["filename"] = name
		}
	}
}

func FileChunk(url, mediaType string, opts ...ChunkOption) Chunk {
	return Chunk{ChunkFile, applyOptions(map[string]any{
		"url": url, "mediaType": mediaType,
	}, opts)}
}

func ReasoningFileChunk(url, mediaType string, opts ...ChunkOption) Chunk {
	return Chunk{ChunkReasoningFile, applyOptions(map[string]any{
		"url": url, "mediaType": mediaType,
	}, opts)}
}

// --- custom and data ---

// CustomChunk emits a namespaced custom chunk.
//
// kind must contain a dot. The client does not enforce this — ui-message-chunks.ts
// declares kind as z.string().transform, so the `${string}.${string}` shape is a
// TypeScript compile-time constraint only and 'nodot' validates fine. It is enforced
// here because a dotless kind collides with future protocol names, and a producer-side
// check is the only place it can be caught at all.
func CustomChunk(kind string, opts ...ChunkOption) (Chunk, error) {
	if !strings.Contains(kind, ".") {
		return Chunk{}, invalidChunkf("custom.kind %q must be namespaced with a dot", kind)
	}
	return Chunk{ChunkCustom, applyOptions(map[string]any{"kind": kind}, opts)}, nil
}

// DataChunk emits a data-${name} chunk. name must be non-empty and must not itself
// start with "data-", which would produce a "data-data-x" type.
func DataChunk(name string, data any, opts ...ChunkOption) (Chunk, error) {
	if name == "" {
		return Chunk{}, invalidChunkf("data chunk name must not be empty")
	}
	if strings.HasPrefix(name, "data-") {
		return Chunk{}, invalidChunkf("data chunk name %q must not include the data- prefix", name)
	}
	return Chunk{"data-" + name, applyOptions(map[string]any{"data": data}, opts)}, nil
}

// WithDataID sets the optional id on a data chunk, which lets a later chunk with the
// same id replace it rather than append.
func WithDataID(id string) ChunkOption {
	return func(f map[string]any) {
		if id != "" {
			f["id"] = id
		}
	}
}

// WithTransient marks a data chunk the client should render but not persist.
func WithTransient(b bool) ChunkOption {
	return func(f map[string]any) { f["transient"] = b }
}

// --- lifecycle ---

// StartChunk opens a message. messageId is omitted when empty rather than sent as "",
// matching the client, which omits absent keys instead of nulling them.
func StartChunk(messageID string, opts ...ChunkOption) Chunk {
	f := map[string]any{}
	if messageID != "" {
		f["messageId"] = messageID
	}
	return Chunk{ChunkStart, applyOptions(f, opts)}
}

func StartStep() Chunk  { return Chunk{ChunkStartStep, map[string]any{}} }
func FinishStep() Chunk { return Chunk{ChunkFinishStep, map[string]any{}} }

// FinishChunk closes a message. The reason is normalized onto the wire enum, so an
// internal spelling like "tool_calls" cannot reach the client and throw. An empty
// reason is omitted, since finishReason is optional.
func FinishChunk(reason WireFinishReason, opts ...ChunkOption) Chunk {
	f := map[string]any{}
	if r := NormalizeWireFinishReason(string(reason)); r != "" {
		f["finishReason"] = string(r)
	}
	return Chunk{ChunkFinish, applyOptions(f, opts)}
}

// ErrorChunk reports a stream-level failure. errorText is redacted.
//
// Note the client does not treat this as fatal: process-ui-message-stream.ts calls
// onError and breaks, so it neither throws nor unwinds already-rendered parts. Emitting
// it does not terminate the stream — Close does.
func ErrorChunk(err error) Chunk {
	return Chunk{ChunkError, map[string]any{"errorText": redactStreamError(err)}}
}

// ErrorChunkText reports a failure whose text is already safe to show — a message this
// server authored, not a provider or panic value. Prefer ErrorChunk for anything
// derived from an error.
func ErrorChunkText(text string) Chunk {
	if text == "" {
		text = "stream error"
	}
	return Chunk{ChunkError, map[string]any{"errorText": text}}
}

// AbortChunk signals cancellation.
//
// The client has no `case 'abort'` — it is a rendering no-op, and v7's abort is
// client-driven via chat.ts. Emitting it is therefore informational for other
// consumers of the stream, not something that makes the browser stop.
func AbortChunk(reason string) Chunk {
	f := map[string]any{}
	if reason != "" {
		f["reason"] = reason
	}
	return Chunk{ChunkAbort, f}
}

// MessageMetadataChunk attaches metadata to the message being built. messageMetadata
// is required here, unlike on start/finish where it is optional.
func MessageMetadataChunk(md any) Chunk {
	return Chunk{ChunkMessageMetadata, map[string]any{"messageMetadata": md}}
}
