package agui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream"
)

// toolCallRecord accumulates one tool call so an interrupt boundary can publish
// it in MESSAGES_SNAPSHOT, which the client rebuilds its message list from.
type toolCallRecord struct {
	id   string
	name string
	args strings.Builder
}

func (e *encoder) toolStart(event aikit.StepEvent) ([]uistream.Frame, error) {
	if event.ToolCallID == "" {
		return nil, errors.New("agui: tool call start requires an ID")
	}
	frames, err := e.closeReasoning()
	if err != nil {
		return nil, err
	}
	if !e.openTools[event.ToolCallID] {
		e.openTools[event.ToolCallID] = true
		e.toolOrder = append(e.toolOrder, event.ToolCallID)
	}
	e.record(event.ToolCallID, event.ToolCallName)

	// parentMessageId ties the call to the assistant message it belongs to.
	start, err := e.event(eventToolCallStart, map[string]any{
		"toolCallId": event.ToolCallID, "toolCallName": event.ToolCallName,
		"parentMessageId": e.messageID(),
	})
	if err != nil {
		return nil, err
	}
	frames = append(frames, start...)
	if event.ToolCallArgsDelta == "" {
		return frames, nil
	}
	args, err := e.appendArgs(event.ToolCallID, event.ToolCallArgsDelta)
	return append(frames, args...), err
}

func (e *encoder) toolArgs(event aikit.StepEvent) ([]uistream.Frame, error) {
	if event.ToolCallArgsDelta == "" {
		return nil, nil
	}
	// A delta for a call the client never saw start would be dropped by the
	// client anyway; skipping it keeps the wire self-consistent.
	if !e.openTools[event.ToolCallID] {
		return nil, nil
	}
	return e.appendArgs(event.ToolCallID, event.ToolCallArgsDelta)
}

func (e *encoder) appendArgs(toolCallID, delta string) ([]uistream.Frame, error) {
	if record := e.toolCallIndex[toolCallID]; record != nil {
		record.args.WriteString(delta)
	}
	return e.event(eventToolCallArgs, map[string]any{"toolCallId": toolCallID, "delta": delta})
}

func (e *encoder) closeTool(id string) ([]uistream.Frame, error) {
	if id == "" || !e.openTools[id] {
		return nil, nil
	}
	delete(e.openTools, id)
	return e.event(eventToolCallEnd, map[string]any{"toolCallId": id})
}

func (e *encoder) toolResult(event aikit.StepEvent) ([]uistream.Frame, error) {
	if event.ToolResult == nil {
		return nil, nil
	}
	return e.resultEvent(event.ToolResult.ID, event.ToolResult.Output)
}

func (e *encoder) toolInvalid(event aikit.StepEvent) ([]uistream.Frame, error) {
	frames, err := e.closeTool(event.ToolCallID)
	if err != nil {
		return nil, err
	}
	result, err := e.resultEvent(
		event.ToolCallID,
		fmt.Sprintf("invalid JSON arguments for tool %q", event.ToolCallName),
	)
	return append(frames, result...), err
}

func (e *encoder) toolDenied(event aikit.StepEvent) ([]uistream.Frame, error) {
	return e.resultEvent(event.ToolCallID, "tool execution denied")
}

// resultEvent emits TOOL_CALL_RESULT under its own message ID. Reusing the
// assistant message ID here would give two distinct AG-UI messages the same
// identity and corrupt the client's message list.
func (e *encoder) resultEvent(toolCallID, content string) ([]uistream.Frame, error) {
	e.toolResultSeq++
	messageID := e.runID + "_toolresult_" + strconv.Itoa(e.toolResultSeq)
	return e.event(eventToolCallResult, map[string]any{
		"messageId": messageID, "toolCallId": toolCallID,
		"content": content, "role": "tool",
	})
}

func (e *encoder) record(toolCallID, name string) {
	if e.toolCallIndex == nil {
		e.toolCallIndex = make(map[string]*toolCallRecord)
	}
	if existing := e.toolCallIndex[toolCallID]; existing != nil {
		if existing.name == "" {
			existing.name = name
		}
		return
	}
	record := &toolCallRecord{id: toolCallID, name: name}
	e.toolCallIndex[toolCallID] = record
	e.toolCalls = append(e.toolCalls, record)
}
