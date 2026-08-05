package ainode

import (
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
)

// InvariantCode identifies a producer contract that the v7 client does not enforce.
type InvariantCode string

const (
	InvariantUnknownChunk      InvariantCode = "unknown-chunk-type"
	InvariantBlockWithoutStart InvariantCode = "block-without-start"
	InvariantBlockAlreadyOpen  InvariantCode = "block-already-open"
	InvariantBlockStillOpen    InvariantCode = "block-still-open"
	InvariantDuplicateToolCall InvariantCode = "duplicate-tool-call-id"
	InvariantEmptyToolCallID   InvariantCode = "empty-tool-call-id"
	InvariantEmptyToolName     InvariantCode = "empty-tool-name"
	InvariantMissingToolInput  InvariantCode = "missing-tool-input"
	InvariantUnknownToolCall   InvariantCode = "unknown-tool-call-id"
)

// InvariantViolation describes malformed producer output without stopping it.
type InvariantViolation struct {
	Code      InvariantCode
	ChunkType string
	Field     string
	Value     string
}

func (v InvariantViolation) Error() string {
	return fmt.Sprintf("aisdk invariant %s: chunk=%q field=%q value=%q", v.Code, v.ChunkType, v.Field, v.Value)
}

type toolLifecycle struct {
	inputAvailable bool
	terminal       bool
}

// InvariantChecker validates one message stream. It is not safe for concurrent use.
type InvariantChecker struct {
	openText      map[string]struct{}
	openReasoning map[string]struct{}
	tools         map[string]*toolLifecycle
}

func NewInvariantChecker() *InvariantChecker {
	return &InvariantChecker{
		openText: make(map[string]struct{}), openReasoning: make(map[string]struct{}),
		tools: make(map[string]*toolLifecycle),
	}
}

func (c *InvariantChecker) Observe(chunk Chunk) []InvariantViolation {
	var violations []InvariantViolation
	if !ValidChunkType(chunk.Type) {
		violations = append(violations, violation(InvariantUnknownChunk, chunk, "type", chunk.Type))
	}
	violations = append(violations, c.observeBlocks(chunk)...)
	violations = append(violations, c.observeTool(chunk)...)
	if chunk.Type == ChunkFinish || chunk.Type == ChunkFinishStep || chunk.Type == ChunkStartStep {
		violations = append(violations, c.finalizeBlocks()...)
	}
	return violations
}

func (c *InvariantChecker) Finalize() []InvariantViolation { return c.finalizeBlocks() }

func (c *InvariantChecker) observeBlocks(chunk Chunk) []InvariantViolation {
	var family map[string]struct{}
	switch chunk.Type {
	case ChunkTextStart, ChunkTextDelta, ChunkTextEnd:
		family = c.openText
	case ChunkReasoningStart, ChunkReasoningDelta, ChunkReasoningEnd:
		family = c.openReasoning
	default:
		return nil
	}
	id := stringField(chunk.Fields, "id")
	if strings.HasSuffix(chunk.Type, "-start") {
		if id == "" {
			return []InvariantViolation{violation(InvariantBlockWithoutStart, chunk, "id", id)}
		}
		if _, open := family[id]; open {
			return []InvariantViolation{violation(InvariantBlockAlreadyOpen, chunk, "id", id)}
		}
		family[id] = struct{}{}
		return nil
	}
	if _, open := family[id]; !open || id == "" {
		return []InvariantViolation{violation(InvariantBlockWithoutStart, chunk, "id", id)}
	}
	if strings.HasSuffix(chunk.Type, "-end") {
		delete(family, id)
	}
	return nil
}

func (c *InvariantChecker) finalizeBlocks() []InvariantViolation {
	violations := make([]InvariantViolation, 0, len(c.openText)+len(c.openReasoning))
	for id := range c.openText {
		violations = append(
			violations,
			InvariantViolation{Code: InvariantBlockStillOpen, ChunkType: ChunkTextEnd, Field: "id", Value: id},
		)
		delete(c.openText, id)
	}
	for id := range c.openReasoning {
		violations = append(
			violations,
			InvariantViolation{Code: InvariantBlockStillOpen, ChunkType: ChunkReasoningEnd, Field: "id", Value: id},
		)
		delete(c.openReasoning, id)
	}
	return violations
}

func (c *InvariantChecker) observeTool(chunk Chunk) []InvariantViolation {
	if !chunkHasToolCallID(chunk.Type) {
		return nil
	}
	id := stringField(chunk.Fields, "toolCallId")
	violations := validateToolFields(chunk, id)
	if id == "" {
		return violations
	}
	return append(violations, c.observeToolLifecycle(chunk, id)...)
}

func validateToolFields(chunk Chunk, id string) []InvariantViolation {
	var violations []InvariantViolation
	if id == "" {
		violations = append(violations, violation(InvariantEmptyToolCallID, chunk, "toolCallId", id))
	}
	if chunkNeedsToolName(chunk.Type) && stringField(chunk.Fields, "toolName") == "" {
		violations = append(violations, violation(InvariantEmptyToolName, chunk, "toolName", ""))
	}
	if chunk.Type == ChunkToolInputAvailable {
		if _, present := chunk.Fields["input"]; !present {
			violations = append(violations, violation(InvariantMissingToolInput, chunk, "input", ""))
		}
	}
	return violations
}

func (c *InvariantChecker) observeToolLifecycle(chunk Chunk, id string) []InvariantViolation {
	var violations []InvariantViolation
	state, known := c.tools[id]
	switch chunk.Type {
	case ChunkToolInputStart:
		if known {
			violations = append(violations, violation(InvariantDuplicateToolCall, chunk, "toolCallId", id))
		} else {
			c.tools[id] = &toolLifecycle{}
		}
	case ChunkToolInputAvailable, ChunkToolInputError:
		if !known {
			state = &toolLifecycle{}
			c.tools[id] = state
		} else if state.inputAvailable {
			violations = append(violations, violation(InvariantDuplicateToolCall, chunk, "toolCallId", id))
		}
		state.inputAvailable = true
	case ChunkToolApprovalRequest:
		if !known {
			c.tools[id] = &toolLifecycle{inputAvailable: true}
		}
	case ChunkToolInputDelta,
		ChunkToolOutputAvailable, ChunkToolOutputError, ChunkToolOutputDenied:
		if !known {
			violations = append(violations, violation(InvariantUnknownToolCall, chunk, "toolCallId", id))
		} else if chunk.Type == ChunkToolOutputAvailable || chunk.Type == ChunkToolOutputError || chunk.Type == ChunkToolOutputDenied {
			if state.terminal {
				violations = append(violations, violation(InvariantDuplicateToolCall, chunk, "toolCallId", id))
			}
			state.terminal = true
		}
	}
	return violations
}

func violation(code InvariantCode, chunk Chunk, field, value string) InvariantViolation {
	return InvariantViolation{Code: code, ChunkType: chunk.Type, Field: field, Value: value}
}

func chunkHasToolCallID(typ string) bool {
	switch typ {
	case ChunkToolInputStart, ChunkToolInputDelta, ChunkToolInputAvailable, ChunkToolInputError,
		ChunkToolApprovalRequest, ChunkToolOutputAvailable, ChunkToolOutputError, ChunkToolOutputDenied:
		return true
	default:
		return false
	}
}

func chunkNeedsToolName(typ string) bool {
	return typ == ChunkToolInputStart || typ == ChunkToolInputAvailable ||
		typ == ChunkToolInputError || typ == ChunkToolApprovalRequest
}

var invariantViolationCount atomic.Uint64

func InvariantViolationCount() uint64 { return invariantViolationCount.Load() }

func reportInvariant(logger *slog.Logger, reporter func(InvariantViolation), violation InvariantViolation) {
	invariantViolationCount.Add(1)
	safeObserver(func() {
		if reporter != nil {
			reporter(violation)
			return
		}
		if logger == nil {
			return
		}
		logger.Error("invalid AI SDK UI message stream chunk",
			"code", violation.Code, "chunk_type", violation.ChunkType,
			"field", violation.Field, "value", violation.Value)
	})
}
