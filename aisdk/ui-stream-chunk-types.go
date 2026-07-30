// Package aisdk implements the AI SDK v7 UI message stream protocol: chunk types,
// SSE framing, UIMessage parsing, and the normalized StepEvent vocabulary a producer
// converts into chunks.
//
// It has no Eino dependency and no router dependency — CI asserts that with
// `go list -deps ./aisdk/...`, so a consumer that only speaks the protocol never
// compiles an agent framework. The Eino side lives in einoadapter.
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

	// Source chunks for web search results and citations.
	ChunkSource  = "source"
	ChunkSources = "sources"

	// ChunkMessageMetadata is attached to the assistant message being built.
	ChunkMessageMetadata = "message-metadata"

	// ChunkAbort signals stream cancellation.
	ChunkAbort = "abort"

	// Tool error and approval chunk types (AI SDK Node v6 parity).
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
