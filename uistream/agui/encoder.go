package agui

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream"
)

// encoder holds the per-run AG-UI state for one Pipe invocation.
type encoder struct {
	runID       string
	threadID    string
	parentRunID string
	// state mirrors RunAgentInput.state so an interrupt boundary can echo it.
	state    any
	hasState bool
	// requestMessages is the request's verbatim history, republished with the
	// assistant turn when a run suspends. See messagesSnapshot.
	requestMessages []json.RawMessage
	step            int
	// emitSteps gates STEP_STARTED/STEP_FINISHED. See WithStepEvents.
	emitSteps bool
	// structuredStart gates the structured-output.start announcement. See
	// WithStructuredOutputStart.
	structuredStart bool

	textOpen bool
	textID   string
	textBuf  strings.Builder

	reasoningOpen bool
	reasoningID   string

	openTools     map[string]bool
	toolOrder     []string
	toolCalls     []*toolCallRecord
	toolCallIndex map[string]*toolCallRecord
	toolResultSeq int

	interrupts []map[string]any

	stepUsage    aikit.Usage
	totalUsage   aikit.Usage
	finishReason string
}

func (e *encoder) event(typ string, data map[string]any) ([]uistream.Frame, error) {
	data["type"] = typ
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return []uistream.Frame{{Data: payload}}, nil
}

func (e *encoder) Start() ([]uistream.Frame, error) {
	started := map[string]any{"threadId": e.threadID, "runId": e.runID}
	if e.parentRunID != "" {
		started["parentRunId"] = e.parentRunID
	}
	frames, err := e.event(eventRunStarted, started)
	if err != nil || !e.structuredStart {
		return frames, err
	}
	// No messageId: at Start the step counter is still zero, so an ID minted
	// here would not match the one textDelta generates later. The client falls
	// back to the active assistant message when the field is absent.
	announce, err := e.customEvent(customStructuredOutputStartName, map[string]any{})
	return append(frames, announce...), err
}

//nolint:gocyclo // One explicit branch per engine event is the mapping contract.
func (e *encoder) Encode(event aikit.StepEvent) ([]uistream.Frame, error) {
	switch event.Type {
	case aikit.StepEventStepStart:
		return e.stepStart(event)
	case aikit.StepEventStepEnd:
		return e.stepEnd(event)
	case aikit.StepEventTextDelta:
		if event.StepNumber != 0 {
			e.step = event.StepNumber
		}
		return e.textDelta(event.TextDelta)
	case aikit.StepEventReasoningDelta:
		return e.reasoningDelta(event.ReasoningDelta)
	case aikit.StepEventToolCallStart:
		return e.toolStart(event)
	case aikit.StepEventToolCallDelta:
		return e.toolArgs(event)
	case aikit.StepEventToolCallReady:
		return e.closeTool(event.ToolCallID)
	case aikit.StepEventToolResult:
		return e.toolResult(event)
	case aikit.StepEventToolCallInvalid:
		return e.toolInvalid(event)
	case aikit.StepEventToolOutputDenied:
		return e.toolDenied(event)
	case aikit.StepEventToolApprovalRequest:
		e.recordInterrupt(event)
		return nil, nil
	case aikit.StepEventClientToolRequest:
		e.recordClientToolInterrupt(event)
		return nil, nil
	case aikit.StepEventUsage:
		if event.Usage != nil {
			e.stepUsage = e.stepUsage.Merge(*event.Usage)
		}
		return nil, nil
	case aikit.StepEventSource:
		return e.sourceEvent(event)
	case aikit.StepEventFileDelta:
		return e.fileEvent(event)
	case aikit.StepEventStructuredOutput:
		return e.structuredOutputEvent(event)
	case aikit.StepEventStateSnapshot:
		return e.stateSnapshotEvent(event)
	case aikit.StepEventStateDelta:
		return e.stateDeltaEvent(event)
	case aikit.StepEventDone:
		// The driver's Finish call owns the single terminal AG-UI event.
		return nil, nil
	case aikit.StepEventError:
		return nil, errors.New("agui: StepEventError must be normalized by uistream.Pipe")
	default:
		return nil, fmt.Errorf("agui: unsupported step event type %d", event.Type)
	}
}

func (e *encoder) stepStart(event aikit.StepEvent) ([]uistream.Frame, error) {
	e.step = event.StepNumber
	// Tool bookkeeping is per step; carrying it forward would re-scan every
	// prior step's calls on each close.
	e.openTools = make(map[string]bool)
	e.toolOrder = nil
	if !e.emitSteps {
		return nil, nil
	}
	return e.event(eventStepStarted, map[string]any{"stepName": e.stepName()})
}

func (e *encoder) stepEnd(event aikit.StepEvent) ([]uistream.Frame, error) {
	frames, err := e.closeOpenContent()
	if err != nil {
		return nil, err
	}
	e.commitStepUsage()
	if reason := wireFinishReason(event.FinishReason); reason != "" {
		e.finishReason = reason
	}
	if !e.emitSteps {
		return frames, nil
	}
	finished, err := e.event(eventStepFinished, map[string]any{"stepName": e.stepName()})
	return append(frames, finished...), err
}
