package agui

import (
	"encoding/base64"
	"encoding/json"
	"strconv"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream"
)

// reasoningDelta opens the REASONING_* block on first use. THINKING_* is
// deliberately not emitted: it is deprecated in @ag-ui/core and absent from
// TanStack AI's StreamChunk union, so its client would ignore it entirely.
func (e *encoder) reasoningDelta(delta string) ([]uistream.Frame, error) {
	var frames []uistream.Frame
	if !e.reasoningOpen {
		e.reasoningOpen = true
		e.reasoningID = e.reasoningMessageID()
		start, err := e.event(eventReasoningStart, map[string]any{"messageId": e.reasoningID})
		if err != nil {
			return nil, err
		}
		frames = append(frames, start...)
		msgStart, err := e.event(eventReasoningMessageStart,
			map[string]any{"messageId": e.reasoningID, "role": "reasoning"})
		if err != nil {
			return nil, err
		}
		frames = append(frames, msgStart...)
	}
	content, err := e.event(eventReasoningMessageContent,
		map[string]any{"messageId": e.reasoningID, "delta": delta})
	return append(frames, content...), err
}

func (e *encoder) closeReasoning() ([]uistream.Frame, error) {
	if !e.reasoningOpen {
		return nil, nil
	}
	e.reasoningOpen = false
	frames, err := e.event(eventReasoningMessageEnd, map[string]any{"messageId": e.reasoningID})
	if err != nil {
		return nil, err
	}
	end, err := e.event(eventReasoningEnd, map[string]any{"messageId": e.reasoningID})
	return append(frames, end...), err
}

func (e *encoder) reasoningMessageID() string {
	return e.runID + "_reasoning_" + strconv.Itoa(e.step)
}

// sourceEvent carries a provider-native source reference. AG-UI has no source
// event, so it travels as CUSTOM where TanStack clients can narrow on the name.
func (e *encoder) sourceEvent(event aikit.StepEvent) ([]uistream.Frame, error) {
	if event.Source == nil {
		return nil, nil
	}
	value := map[string]any{"id": event.Source.ID}
	if event.Source.SourceType != "" {
		value["sourceType"] = event.Source.SourceType
	}
	if event.Source.URL != "" {
		value["url"] = event.Source.URL
	}
	if event.Source.Title != "" {
		value["title"] = event.Source.Title
	}
	return e.customEvent(customSourceName, value)
}

// fileEvent carries a model-emitted file as a data URL, matching how the AI SDK
// serializes binary parts onto a text-only wire.
func (e *encoder) fileEvent(event aikit.StepEvent) ([]uistream.Frame, error) {
	if len(event.FileData) == 0 {
		return nil, nil
	}
	mediaType := event.FileMediaType
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	return e.customEvent(customFileName, map[string]any{
		"mediaType": mediaType,
		"url":       "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(event.FileData),
	})
}

func (e *encoder) structuredOutputEvent(event aikit.StepEvent) ([]uistream.Frame, error) {
	if len(event.StructuredOutput) == 0 {
		return nil, nil
	}
	value := map[string]any{"raw": string(event.StructuredOutput)}
	var object any
	if err := json.Unmarshal(event.StructuredOutput, &object); err == nil {
		value["object"] = object
	}
	return e.customEvent(customStructuredOutputName, value)
}

func (e *encoder) customEvent(name string, value map[string]any) ([]uistream.Frame, error) {
	return e.event(eventCustom, map[string]any{
		"name": name, "value": value,
		"threadId": e.threadID, "runId": e.runID,
	})
}

// stateSnapshot echoes caller-supplied run state. TanStack's stream processor
// ignores STATE_*, but the events are part of AG-UI proper and are consumed by
// other clients.
func (e *encoder) stateSnapshot() ([]uistream.Frame, error) {
	if !e.hasState {
		return nil, nil
	}
	return e.event(eventStateSnapshot, map[string]any{"snapshot": e.state})
}
