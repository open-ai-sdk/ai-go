package ainode

import "strings"

var validChunkTypes = map[string]struct{}{
	ChunkAbort: {}, ChunkCustom: {}, ChunkError: {}, ChunkFile: {}, ChunkFinish: {},
	ChunkFinishStep: {}, ChunkMessageMetadata: {}, ChunkReasoningDelta: {},
	ChunkReasoningEnd: {}, ChunkReasoningFile: {}, ChunkReasoningStart: {},
	ChunkSourceDocument: {}, ChunkSourceURL: {}, ChunkStart: {}, ChunkStartStep: {},
	ChunkTextDelta: {}, ChunkTextEnd: {}, ChunkTextStart: {}, ChunkToolApprovalRequest: {},
	ChunkToolApprovalResponse: {}, ChunkToolInputAvailable: {}, ChunkToolInputDelta: {},
	ChunkToolInputError: {}, ChunkToolInputStart: {}, ChunkToolOutputAvailable: {},
	ChunkToolOutputDenied: {}, ChunkToolOutputError: {},
}

// ValidChunkType accepts the 27 v7 literals and the data-* prefix member.
func ValidChunkType(typ string) bool {
	_, literal := validChunkTypes[typ]
	return literal || strings.HasPrefix(typ, "data-")
}
