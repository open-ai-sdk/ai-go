package aisdk

import (
	"encoding/json"
	"io"
	"iter"
	"log/slog"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream/ainode"
)

const (
	ChunkStart                = ainode.ChunkStart
	ChunkStartStep            = ainode.ChunkStartStep
	ChunkTextStart            = ainode.ChunkTextStart
	ChunkTextDelta            = ainode.ChunkTextDelta
	ChunkTextEnd              = ainode.ChunkTextEnd
	ChunkReasoningStart       = ainode.ChunkReasoningStart
	ChunkReasoningDelta       = ainode.ChunkReasoningDelta
	ChunkReasoningEnd         = ainode.ChunkReasoningEnd
	ChunkToolInputStart       = ainode.ChunkToolInputStart
	ChunkToolInputDelta       = ainode.ChunkToolInputDelta
	ChunkToolInputAvailable   = ainode.ChunkToolInputAvailable
	ChunkToolOutputAvailable  = ainode.ChunkToolOutputAvailable
	ChunkFinishStep           = ainode.ChunkFinishStep
	ChunkFinish               = ainode.ChunkFinish
	ChunkError                = ainode.ChunkError
	ChunkMessageMetadata      = ainode.ChunkMessageMetadata
	ChunkAbort                = ainode.ChunkAbort
	ChunkToolInputError       = ainode.ChunkToolInputError
	ChunkToolOutputError      = ainode.ChunkToolOutputError
	ChunkToolOutputDenied     = ainode.ChunkToolOutputDenied
	ChunkToolApprovalRequest  = ainode.ChunkToolApprovalRequest
	ChunkToolApprovalResponse = ainode.ChunkToolApprovalResponse
	ChunkCustom               = ainode.ChunkCustom
	ChunkReasoningFile        = ainode.ChunkReasoningFile
	ChunkSourceURL            = ainode.ChunkSourceURL
	ChunkSourceDocument       = ainode.ChunkSourceDocument
	ChunkFile                 = ainode.ChunkFile

	EnvelopePartTypeText           = ainode.EnvelopePartTypeText
	EnvelopePartTypeImage          = ainode.EnvelopePartTypeImage
	EnvelopePartTypeFile           = ainode.EnvelopePartTypeFile
	EnvelopePartTypeToolInvocation = ainode.EnvelopePartTypeToolInvocation
	EnvelopePartTypeDynamicTool    = ainode.EnvelopePartTypeDynamicTool

	InvariantUnknownChunk      = ainode.InvariantUnknownChunk
	InvariantBlockWithoutStart = ainode.InvariantBlockWithoutStart
	InvariantBlockAlreadyOpen  = ainode.InvariantBlockAlreadyOpen
	InvariantBlockStillOpen    = ainode.InvariantBlockStillOpen
	InvariantDuplicateToolCall = ainode.InvariantDuplicateToolCall
	InvariantEmptyToolCallID   = ainode.InvariantEmptyToolCallID
	InvariantEmptyToolName     = ainode.InvariantEmptyToolName
	InvariantMissingToolInput  = ainode.InvariantMissingToolInput
	InvariantUnknownToolCall   = ainode.InvariantUnknownToolCall
)

var ErrInvalidToolApprovalSignature = ainode.ErrInvalidToolApprovalSignature

type (
	Adapter                    = ainode.Adapter
	ApprovalResponseOpts       = ainode.ApprovalResponseOpts
	ChatRequestEnvelope        = ainode.ChatRequestEnvelope
	Chunk                      = ainode.Chunk
	ChunkProducer              = ainode.ChunkProducer
	ChunkProducerOption        = ainode.ChunkProducerOption
	ChunkStream                = ainode.ChunkStream
	CreateUIStreamOptions      = ainode.CreateUIStreamOptions
	EnvelopeMessage            = ainode.EnvelopeMessage
	EnvelopePartType           = ainode.EnvelopePartType
	EnvelopePartUnion          = ainode.EnvelopePartUnion
	ExecuteFunc                = ainode.ExecuteFunc
	FileChunkOpts              = ainode.FileChunkOpts
	InputTokenDetails          = ainode.InputTokenDetails
	InvariantChecker           = ainode.InvariantChecker
	InvariantCode              = ainode.InvariantCode
	InvariantViolation         = ainode.InvariantViolation
	MergeOption                = ainode.MergeOption
	MessageMetadataInfo        = ainode.MessageMetadataInfo
	OutputTokenDetails         = ainode.OutputTokenDetails
	PersistedMessageBuilder    = ainode.PersistedMessageBuilder
	SourceDocumentOpts         = ainode.SourceDocumentOpts
	SourceHook                 = ainode.SourceHook
	StreamOptions              = ainode.StreamOptions
	StreamingUIMessage         = ainode.StreamingUIMessage
	StreamingUIMessageState    = ainode.StreamingUIMessageState
	ToUIStreamOptions          = ainode.ToUIStreamOptions
	ToolApproval               = ainode.ToolApproval
	ToolApprovalResponse       = ainode.ToolApprovalResponse
	ToolApprovalSignatureInput = ainode.ToolApprovalSignatureInput
	ToolChunkOpts              = ainode.ToolChunkOpts
	ToolResult                 = ainode.ToolResult
	ToolResultHook             = ainode.ToolResultHook
	UIMessagePart              = ainode.UIMessagePart
	UIStreamEndResult          = ainode.UIStreamEndResult
	UIStreamOption             = ainode.UIStreamOption
	UIStreamWriter             = ainode.UIStreamWriter
	UsageInfo                  = ainode.UsageInfo
	Writer                     = ainode.Writer
)

func CanonicalizeToolApprovalInput(input json.RawMessage) ([]byte, error) {
	return ainode.CanonicalizeToolApprovalInput(input)
}

func CreateUIMessageStream(w io.Writer, opts CreateUIStreamOptions, execute func(*UIStreamWriter) error) {
	ainode.CreateUIMessageStream(w, opts, execute)
}
func Execute(w io.Writer, opts StreamOptions, fn ExecuteFunc) { ainode.Execute(w, opts, fn) }
func HashCanonical(input json.RawMessage) (string, error)     { return ainode.HashCanonical(input) }
func InvariantViolationCount() uint64                         { return ainode.InvariantViolationCount() }
func MergeChunks(sources ...<-chan Chunk) <-chan Chunk        { return ainode.MergeChunks(sources...) }
func ProcessUIMessageStream(chunks <-chan Chunk, state *StreamingUIMessageState) <-chan Chunk {
	return ainode.ProcessUIMessageStream(chunks, state)
}

func ResolveMessageID(messages []EnvelopeMessage, fallback string) string {
	return ainode.ResolveMessageID(messages, fallback)
}

func ResolveMessageIDFromEnvelope(env ChatRequestEnvelope, fallback string) string {
	return ainode.ResolveMessageIDFromEnvelope(env, fallback)
}

func SignToolApproval(secret []byte, input ToolApprovalSignatureInput) (string, error) {
	return ainode.SignToolApproval(secret, input)
}

func StreamToWriter(
	events iter.Seq2[aikit.StepEvent, error],
	w io.Writer,
	msgID string,
	opts ...UIStreamOption,
) string {
	return ainode.StreamToWriter(events, w, msgID, opts...)
}

func ToAIContentParts(parts []EnvelopePartUnion) []aikit.ContentPart {
	return ainode.ToAIContentParts(parts)
}
func ToAIMessages(messages []EnvelopeMessage) []aikit.Message { return ainode.ToAIMessages(messages) }
func ToUIMessageStream(events iter.Seq2[aikit.StepEvent, error], msgID string, opts ToUIStreamOptions) <-chan Chunk {
	return ainode.ToUIMessageStream(events, msgID, opts)
}
func ValidChunkType(typ string) bool { return ainode.ValidChunkType(typ) }
func VerifyToolApproval(secret []byte, signature string, input ToolApprovalSignatureInput) error {
	return ainode.VerifyToolApproval(secret, signature, input)
}
func WriteSSE(w io.Writer, chunk Chunk) error               { return ainode.WriteSSE(w, chunk) }
func WriteSSEStream(w io.Writer, chunks <-chan Chunk) error { return ainode.WriteSSEStream(w, chunks) }
func NewAdapter(msgID string) *Adapter                      { return ainode.NewAdapter(msgID) }
func NewChunkProducer(msgID string, options ...ChunkProducerOption) *ChunkProducer {
	return ainode.NewChunkProducer(msgID, options...)
}

func WithInvariantLogger(logger *slog.Logger) ChunkProducerOption {
	return ainode.WithInvariantLogger(logger)
}

func WithInvariantReporter(reporter func(InvariantViolation)) ChunkProducerOption {
	return ainode.WithInvariantReporter(reporter)
}
func NewInvariantChecker() *InvariantChecker     { return ainode.NewInvariantChecker() }
func MergeWithOnEnd(fn func(string)) MergeOption { return ainode.MergeWithOnEnd(fn) }
func MergeWithPersistence(builder *PersistedMessageBuilder) MergeOption {
	return ainode.MergeWithPersistence(builder)
}
func MergeWithSourceHook(hook SourceHook) MergeOption { return ainode.MergeWithSourceHook(hook) }
func MergeWithToolResultHook(hook ToolResultHook) MergeOption {
	return ainode.MergeWithToolResultHook(hook)
}

func NewPersistedMessageBuilder() *PersistedMessageBuilder {
	return ainode.NewPersistedMessageBuilder()
}

func ApprovalResponses(messages []EnvelopeMessage) []ToolApprovalResponse {
	return ainode.ApprovalResponses(messages)
}
func WithUIOnEnd(fn func(string)) UIStreamOption { return ainode.WithUIOnEnd(fn) }
func WithUIPersistence(builder *PersistedMessageBuilder) UIStreamOption {
	return ainode.WithUIPersistence(builder)
}
func WithUISourceHook(hook SourceHook) UIStreamOption { return ainode.WithUISourceHook(hook) }
func WithUIToolResultHook(hook ToolResultHook) UIStreamOption {
	return ainode.WithUIToolResultHook(hook)
}

func NewStreamingUIMessageState(messageID string, lastMessage *StreamingUIMessage) *StreamingUIMessageState {
	return ainode.NewStreamingUIMessageState(messageID, lastMessage)
}
func NewWriter(w io.Writer) *Writer { return ainode.NewWriter(w) }
