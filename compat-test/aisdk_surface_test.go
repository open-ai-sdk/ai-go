package compattest

import (
	"github.com/open-ai-sdk/ai-go/aisdk"
	"github.com/open-ai-sdk/ai-go/uistream/ainode"
)

var (
	compatChunk aisdk.Chunk
	directChunk ainode.Chunk = compatChunk
	_           aisdk.Chunk  = directChunk
)

// These compile-time references are the migration gate for aisdk's public
// surface. Keeping them in a separate module proves the compatibility package
// remains usable by external consumers after its implementation moves.
var (
	_ = aisdk.ChunkStart
	_ = aisdk.ChunkStartStep
	_ = aisdk.ChunkTextStart
	_ = aisdk.ChunkTextDelta
	_ = aisdk.ChunkTextEnd
	_ = aisdk.ChunkReasoningStart
	_ = aisdk.ChunkReasoningDelta
	_ = aisdk.ChunkReasoningEnd
	_ = aisdk.ChunkToolInputStart
	_ = aisdk.ChunkToolInputDelta
	_ = aisdk.ChunkToolInputAvailable
	_ = aisdk.ChunkToolOutputAvailable
	_ = aisdk.ChunkFinishStep
	_ = aisdk.ChunkFinish
	_ = aisdk.ChunkError
	_ = aisdk.ChunkMessageMetadata
	_ = aisdk.ChunkAbort
	_ = aisdk.ChunkToolInputError
	_ = aisdk.ChunkToolOutputError
	_ = aisdk.ChunkToolOutputDenied
	_ = aisdk.ChunkToolApprovalRequest
	_ = aisdk.ChunkToolApprovalResponse
	_ = aisdk.ChunkCustom
	_ = aisdk.ChunkReasoningFile
	_ = aisdk.ChunkSourceURL
	_ = aisdk.ChunkSourceDocument
	_ = aisdk.ChunkFile

	_ = aisdk.EnvelopePartTypeText
	_ = aisdk.EnvelopePartTypeImage
	_ = aisdk.EnvelopePartTypeFile
	_ = aisdk.EnvelopePartTypeToolInvocation
	_ = aisdk.EnvelopePartTypeDynamicTool

	_ = aisdk.InvariantUnknownChunk
	_ = aisdk.InvariantBlockWithoutStart
	_ = aisdk.InvariantBlockAlreadyOpen
	_ = aisdk.InvariantBlockStillOpen
	_ = aisdk.InvariantDuplicateToolCall
	_ = aisdk.InvariantEmptyToolCallID
	_ = aisdk.InvariantEmptyToolName
	_ = aisdk.InvariantMissingToolInput
	_ = aisdk.InvariantUnknownToolCall

	_ = aisdk.ErrInvalidToolApprovalSignature

	_ = aisdk.CanonicalizeToolApprovalInput
	_ = aisdk.CreateUIMessageStream
	_ = aisdk.Execute
	_ = aisdk.HashCanonical
	_ = aisdk.InvariantViolationCount
	_ = aisdk.MergeChunks
	_ = aisdk.ProcessUIMessageStream
	_ = aisdk.ResolveMessageID
	_ = aisdk.ResolveMessageIDFromEnvelope
	_ = aisdk.SignToolApproval
	_ = aisdk.StreamToWriter
	_ = aisdk.ToAIContentParts
	_ = aisdk.ToAIMessages
	_ = aisdk.ToUIMessageStream
	_ = aisdk.ValidChunkType
	_ = aisdk.VerifyToolApproval
	_ = aisdk.WriteSSE
	_ = aisdk.WriteSSEStream
	_ = aisdk.NewAdapter
	_ = aisdk.NewChunkProducer
	_ = aisdk.WithInvariantLogger
	_ = aisdk.WithInvariantReporter
	_ = aisdk.NewInvariantChecker
	_ = aisdk.MergeWithOnEnd
	_ = aisdk.MergeWithPersistence
	_ = aisdk.MergeWithSourceHook
	_ = aisdk.MergeWithToolResultHook
	_ = aisdk.NewPersistedMessageBuilder
	_ = aisdk.ApprovalResponses
	_ = aisdk.WithUIOnEnd
	_ = aisdk.WithUIPersistence
	_ = aisdk.WithUISourceHook
	_ = aisdk.WithUIToolResultHook
	_ = aisdk.NewStreamingUIMessageState
	_ = aisdk.NewWriter
)

var (
	_ aisdk.Adapter
	_ aisdk.ApprovalResponseOpts
	_ aisdk.ChatRequestEnvelope
	_ aisdk.Chunk
	_ aisdk.ChunkProducer
	_ aisdk.ChunkProducerOption
	_ aisdk.ChunkStream
	_ aisdk.CreateUIStreamOptions
	_ aisdk.EnvelopeMessage
	_ aisdk.EnvelopePartType
	_ aisdk.EnvelopePartUnion
	_ aisdk.ExecuteFunc
	_ aisdk.FileChunkOpts
	_ aisdk.InputTokenDetails
	_ aisdk.InvariantChecker
	_ aisdk.InvariantCode
	_ aisdk.InvariantViolation
	_ aisdk.MergeOption
	_ aisdk.MessageMetadataInfo
	_ aisdk.OutputTokenDetails
	_ aisdk.PersistedMessageBuilder
	_ aisdk.SourceDocumentOpts
	_ aisdk.SourceHook
	_ aisdk.StreamOptions
	_ aisdk.StreamingUIMessage
	_ aisdk.StreamingUIMessageState
	_ aisdk.ToUIStreamOptions
	_ aisdk.ToolApproval
	_ aisdk.ToolApprovalResponse
	_ aisdk.ToolApprovalSignatureInput
	_ aisdk.ToolChunkOpts
	_ aisdk.ToolResult
	_ aisdk.ToolResultHook
	_ aisdk.UIMessagePart
	_ aisdk.UIStreamEndResult
	_ aisdk.UIStreamOption
	_ aisdk.UIStreamWriter
	_ aisdk.UsageInfo
	_ aisdk.Writer
)
