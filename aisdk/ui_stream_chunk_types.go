// Package aisdk provides an SSE adapter that translates aikit.StepEvents into
// the AI SDK UI message stream wire format.
package aisdk

// Chunk type names as used in the x-vercel-ai-ui-message-stream protocol.
const (
	ChunkStart               = "start"
	ChunkStartStep           = "start-step"
	ChunkTextStart           = "text-start"
	ChunkTextDelta           = "text-delta"
	ChunkTextEnd             = "text-end"
	ChunkReasoningStart      = "reasoning-start"
	ChunkReasoningDelta      = "reasoning-delta"
	ChunkReasoningEnd        = "reasoning-end"
	ChunkToolInputStart      = "tool-input-start"
	ChunkToolInputDelta      = "tool-input-delta"
	ChunkToolInputAvailable  = "tool-input-available"
	ChunkToolOutputAvailable = "tool-output-available"
	ChunkFinishStep          = "finish-step"
	ChunkFinish              = "finish"
	ChunkError               = "error"

	// ChunkMessageMetadata is attached to the assistant message being built.
	ChunkMessageMetadata = "message-metadata"

	// ChunkAbort signals stream cancellation.
	ChunkAbort = "abort"

	// Tool error and approval chunk types.
	ChunkToolInputError       = "tool-input-error"
	ChunkToolOutputError      = "tool-output-error"
	ChunkToolOutputDenied     = "tool-output-denied"
	ChunkToolApprovalRequest  = "tool-approval-request"
	ChunkToolApprovalResponse = "tool-approval-response"
	ChunkCustom               = "custom"
	ChunkReasoningFile        = "reasoning-file"

	// Structured source types.
	ChunkSourceURL      = "source-url"
	ChunkSourceDocument = "source-document"

	// ChunkFile is an assistant-emitted file reference.
	ChunkFile = "file"
)
