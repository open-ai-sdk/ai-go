package generate

import (
	"context"
	"errors"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/tool"
)

// PromptErrorKind is a stable failure class for an agent prompt/run.
type PromptErrorKind string

const (
	PromptErrorKindCompletion     PromptErrorKind = "completion"
	PromptErrorKindMaxTurns       PromptErrorKind = "max_turns"
	PromptErrorKindUnknownTool    PromptErrorKind = "unknown_tool"
	PromptErrorKindDisallowedTool PromptErrorKind = "disallowed_tool"
	PromptErrorKindToolExecution  PromptErrorKind = "tool_execution"
	PromptErrorKindCancellation   PromptErrorKind = "cancellation"
	PromptErrorKindMemory         PromptErrorKind = "memory"

	PromptErrorKindMemoryHistory = PromptErrorKindMemory
)

// PromptError classifies a failed agent prompt while retaining independently
// owned partial state. Cause remains visible to errors.Is/errors.As.
type PromptError struct {
	Kind      PromptErrorKind
	Operation string
	Cause     error
	Partial   *GenerateTextResult
	History   []Message
}

func (e *PromptError) Error() string {
	if e == nil {
		return "ai: prompt failed"
	}
	message := "ai: prompt"
	if e.Operation != "" {
		message += " " + e.Operation
	}
	if e.Kind != "" {
		message += ": " + string(e.Kind)
	}
	if e.Cause != nil {
		message += ": " + e.Cause.Error()
	}
	return message
}

func (e *PromptError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Is permits kind matching with errors.Is(err, &PromptError{Kind: kind}).
func (e *PromptError) Is(target error) bool {
	want, ok := target.(*PromptError)
	return ok && e != nil && want != nil && want.Kind != "" && e.Kind == want.Kind
}

// NewPromptError constructs a prompt wrapper and snapshots partial state so
// callers may mutate the returned result/history without changing the error.
func NewPromptError(
	kind PromptErrorKind,
	operation string,
	cause error,
	partial *GenerateTextResult,
	history []Message,
) *PromptError {
	return &PromptError{
		Kind: kind, Operation: operation, Cause: cause,
		Partial: snapshotGenerateTextResult(partial),
		History: snapshotResponse(Response{Messages: history}).Messages,
	}
}

func wrapPromptError(cause error, partial *GenerateTextResult, history []Message) error {
	if cause == nil {
		return nil
	}
	var existing *PromptError
	if errors.As(cause, &existing) {
		return cause
	}

	kind := PromptErrorKindCompletion
	switch {
	case errors.Is(cause, context.Canceled), errors.Is(cause, context.DeadlineExceeded):
		kind = PromptErrorKindCancellation
	case errors.Is(cause, tool.ErrNoSuchTool):
		kind = PromptErrorKindUnknownTool
	case errors.Is(cause, tool.ErrDenied):
		kind = PromptErrorKindDisallowedTool
	case errors.Is(cause, tool.ErrExecution):
		kind = PromptErrorKindToolExecution
	}

	wrappedCause := cause
	if kind == PromptErrorKindCompletion {
		var completionErr *llm.CompletionError
		var structuredErr *agent.StructuredOutputError
		if !errors.As(cause, &completionErr) && !errors.As(cause, &structuredErr) {
			wrappedCause = llm.NewCompletionError(
				llm.CompletionErrorKindProvider, "prompt", "", cause,
			)
		}
	}

	if partial != nil && len(partial.Response.Messages) > 0 {
		combined := make([]Message, 0, len(history)+len(partial.Response.Messages))
		combined = append(combined, history...)
		combined = append(combined, partial.Response.Messages...)
		history = combined
	}
	return NewPromptError(kind, "generate", wrappedCause, partial, history)
}

func snapshotGenerateTextResult(result *GenerateTextResult) *GenerateTextResult {
	if result == nil {
		return nil
	}
	snapshot := *result
	snapshot.Steps = make([]StepOutput, len(result.Steps))
	for i, step := range result.Steps {
		snapshot.Steps[i] = snapshotStepOutput(step)
	}
	snapshot.FinalStep = snapshotStepOutput(result.FinalStep)
	snapshot.ToolResults = snapshotToolResults(result.ToolResults)
	snapshot.ToolApprovalRequests = append([]ToolApprovalRequest(nil), result.ToolApprovalRequests...)
	for i := range snapshot.ToolApprovalRequests {
		snapshot.ToolApprovalRequests[i].Args = append(
			snapshot.ToolApprovalRequests[i].Args[:0:0], result.ToolApprovalRequests[i].Args...,
		)
	}
	snapshot.Usage = *snapshotUsage(&result.Usage)
	snapshot.ProviderMetadata = snapshotJSONMap(result.ProviderMetadata)
	snapshot.Warnings = append([]Warning(nil), result.Warnings...)
	if result.Sources != nil {
		snapshot.Sources = make([]Source, len(result.Sources))
		for i, source := range result.Sources {
			snapshot.Sources[i] = source
			snapshot.Sources[i].ProviderMetadata = snapshotJSONMap(source.ProviderMetadata)
		}
	}
	snapshot.Files = cloneGeneratedFiles(result.Files)
	snapshot.StructuredOutput = append(snapshot.StructuredOutput[:0:0], result.StructuredOutput...)
	snapshot.Response = snapshotResponse(result.Response)
	snapshot.Transcript = snapshotResponse(Response{Messages: result.Transcript}).Messages
	return &snapshot
}
