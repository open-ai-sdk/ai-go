package agui

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream"
)

// ErrToolApprovalUnsupported reports that this minimal adapter cannot express
// ai-go's approval suspension through AG-UI's interrupt/resume lifecycle yet.
var ErrToolApprovalUnsupported = errors.New("agui: tool approval requires interrupt/resume support")

type (
	Option func(*config)
	config struct{ runID func() string }
)

// WithRunID supplies fallback run IDs, primarily for deterministic tests.
func WithRunID(fn func() string) Option { return func(c *config) { c.runID = fn } }

// Protocol returns the minimal AG-UI SSE protocol.
func Protocol(opts ...Option) uistream.Protocol {
	c := config{runID: newRunID}
	for _, option := range opts {
		option(&c)
	}
	if c.runID == nil {
		panic("agui: nil run ID function")
	}
	return uistream.Protocol{
		NewEncoder: func(options uistream.Options) uistream.Encoder {
			runID := c.runID()
			threadID := ""
			if value, ok := options.Extra["runId"].(string); ok && value != "" {
				runID = value
			}
			if value, ok := options.Extra["threadId"].(string); ok {
				threadID = value
			}
			return &encoder{
				runID:        runID,
				threadID:     threadID,
				openTools:    make(map[string]bool),
				toolMessages: make(map[string]string),
			}
		},
		Decoder: decoder{},
		Framer:  framer{},
	}
}

type framer struct{}

func (framer) ApplyHeaders(header http.Header) { (uistream.SSEFramer{}).ApplyHeaders(header) }
func (framer) WriteFrame(w io.Writer, frame uistream.Frame) error {
	return (uistream.SSEFramer{}).WriteFrame(w, frame)
}

type encoder struct {
	runID        string
	threadID     string
	step         int
	textOpen     bool
	textID       string
	openTools    map[string]bool
	toolOrder    []string
	toolMessages map[string]string
	stepUsage    aikit.Usage
	totalUsage   aikit.Usage
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
	return e.event("RUN_STARTED", map[string]any{"threadId": e.threadID, "runId": e.runID})
}

func (e *encoder) Encode(event aikit.StepEvent) ([]uistream.Frame, error) {
	switch event.Type {
	case aikit.StepEventStepStart:
		e.step = event.StepNumber
		return e.event("STEP_STARTED", map[string]any{"stepName": e.stepName()})
	case aikit.StepEventStepEnd:
		frames, err := e.closeOpenContent()
		if err != nil {
			return nil, err
		}
		e.commitStepUsage()
		finished, err := e.event("STEP_FINISHED", map[string]any{"stepName": e.stepName()})
		return append(frames, finished...), err
	case aikit.StepEventTextDelta:
		if event.StepNumber != 0 {
			e.step = event.StepNumber
		}
		return e.textDelta(event.TextDelta)
	case aikit.StepEventToolCallStart:
		return e.toolStart(event)
	case aikit.StepEventToolCallDelta:
		if event.ToolCallArgsDelta == "" {
			return nil, nil
		}
		return e.event(
			"TOOL_CALL_ARGS",
			map[string]any{"toolCallId": event.ToolCallID, "delta": event.ToolCallArgsDelta},
		)
	case aikit.StepEventToolCallReady:
		return e.closeTool(event.ToolCallID)
	case aikit.StepEventToolResult:
		if event.ToolResult == nil {
			return nil, nil
		}
		messageID := e.toolMessageID(event.ToolResult.ID)
		return e.event("TOOL_CALL_RESULT", map[string]any{
			"messageId": messageID, "toolCallId": event.ToolResult.ID,
			"content": event.ToolResult.Output, "role": "tool",
		})
	case aikit.StepEventToolCallInvalid:
		frames, err := e.closeTool(event.ToolCallID)
		if err != nil {
			return nil, err
		}
		result, err := e.event("TOOL_CALL_RESULT", map[string]any{
			"messageId": e.toolMessageID(event.ToolCallID), "toolCallId": event.ToolCallID,
			"content": fmt.Sprintf("invalid JSON arguments for tool %q", event.ToolCallName), "role": "tool",
		})
		return append(frames, result...), err
	case aikit.StepEventToolOutputDenied:
		return e.event("TOOL_CALL_RESULT", map[string]any{
			"messageId": e.toolMessageID(event.ToolCallID), "toolCallId": event.ToolCallID,
			"content": "tool execution denied", "role": "tool",
		})
	case aikit.StepEventToolApprovalRequest:
		return nil, fmt.Errorf("%w: %s", ErrToolApprovalUnsupported, event.ToolCallName)
	case aikit.StepEventUsage:
		if event.Usage != nil {
			e.stepUsage = e.stepUsage.Merge(*event.Usage)
		}
		return nil, nil
	case aikit.StepEventDone,
		aikit.StepEventReasoningDelta,
		aikit.StepEventStructuredOutput,
		aikit.StepEventSource,
		aikit.StepEventFileDelta:
		return nil, nil
	case aikit.StepEventError:
		return nil, errors.New("agui: StepEventError must be normalized by uistream.Pipe")
	default:
		return nil, fmt.Errorf("agui: unsupported step event type %d", event.Type)
	}
}

func (e *encoder) Finish(terminal error) ([]uistream.Frame, error) {
	frames, err := e.closeOpenContent()
	if err != nil {
		return nil, err
	}
	e.commitStepUsage()
	if terminal != nil {
		last, err := e.event("RUN_ERROR", map[string]any{"message": "stream error"})
		return append(frames, last...), err
	}
	data := map[string]any{"threadId": e.threadID, "runId": e.runID}
	if e.totalUsage.HasValues() {
		data["result"] = map[string]any{"usage": usagePayload(e.totalUsage)}
	}
	last, err := e.event("RUN_FINISHED", data)
	return append(frames, last...), err
}

func (e *encoder) textDelta(delta string) ([]uistream.Frame, error) {
	if !e.textOpen {
		e.textOpen = true
		e.textID = e.messageID()
		start, err := e.event("TEXT_MESSAGE_START", map[string]any{"messageId": e.textID, "role": "assistant"})
		if err != nil {
			return nil, err
		}
		content, err := e.event("TEXT_MESSAGE_CONTENT", map[string]any{"messageId": e.textID, "delta": delta})
		return append(start, content...), err
	}
	return e.event("TEXT_MESSAGE_CONTENT", map[string]any{"messageId": e.textID, "delta": delta})
}

func (e *encoder) toolStart(event aikit.StepEvent) ([]uistream.Frame, error) {
	if event.ToolCallID == "" {
		return nil, errors.New("agui: tool call start requires an ID")
	}
	if !e.openTools[event.ToolCallID] {
		e.openTools[event.ToolCallID] = true
		e.toolOrder = append(e.toolOrder, event.ToolCallID)
		e.toolMessages[event.ToolCallID] = e.messageID()
	}
	frames, err := e.event("TOOL_CALL_START", map[string]any{
		"toolCallId": event.ToolCallID, "toolCallName": event.ToolCallName,
		"parentMessageId": e.toolMessages[event.ToolCallID],
	})
	if err != nil || event.ToolCallArgsDelta == "" {
		return frames, err
	}
	args, err := e.event(
		"TOOL_CALL_ARGS",
		map[string]any{"toolCallId": event.ToolCallID, "delta": event.ToolCallArgsDelta},
	)
	return append(frames, args...), err
}

func (e *encoder) closeOpenContent() ([]uistream.Frame, error) {
	var frames []uistream.Frame
	if e.textOpen {
		end, err := e.event("TEXT_MESSAGE_END", map[string]any{"messageId": e.textID})
		if err != nil {
			return nil, err
		}
		frames = append(frames, end...)
		e.textOpen = false
	}
	for _, id := range e.toolOrder {
		end, err := e.closeTool(id)
		if err != nil {
			return nil, err
		}
		frames = append(frames, end...)
	}
	return frames, nil
}

func (e *encoder) closeTool(id string) ([]uistream.Frame, error) {
	if id == "" || !e.openTools[id] {
		return nil, nil
	}
	delete(e.openTools, id)
	return e.event("TOOL_CALL_END", map[string]any{"toolCallId": id})
}

func (e *encoder) commitStepUsage() {
	if e.stepUsage.HasValues() || e.stepUsage.Raw != nil {
		e.totalUsage = e.totalUsage.Add(e.stepUsage)
		e.stepUsage = aikit.Usage{}
	}
}

func (e *encoder) stepName() string  { return "step_" + strconv.Itoa(e.step) }
func (e *encoder) messageID() string { return e.runID + "_message_" + strconv.Itoa(e.step) }
func (e *encoder) toolMessageID(toolCallID string) string {
	if id := e.toolMessages[toolCallID]; id != "" {
		return id
	}
	return e.messageID()
}

func usagePayload(usage aikit.Usage) map[string]any {
	return map[string]any{
		"inputTokens": usage.InputTokens, "outputTokens": usage.OutputTokens,
		"totalTokens": usage.TotalTokens, "toolUsePromptTokens": usage.ToolUsePromptTokens,
	}
}

type decoder struct{}

type runAgentInput struct {
	ThreadID       string          `json:"threadId"`
	RunID          string          `json:"runId"`
	ParentRunID    string          `json:"parentRunId,omitempty"`
	State          any             `json:"state"`
	Messages       []aguiMessage   `json:"messages"`
	Tools          json.RawMessage `json:"tools"`
	Context        json.RawMessage `json:"context"`
	ForwardedProps any             `json:"forwardedProps"`
}

type aguiMessage struct {
	ID         string          `json:"id"`
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	Name       string          `json:"name,omitempty"`
	ToolCallID string          `json:"toolCallId,omitempty"`
	ToolCalls  []aguiToolCall  `json:"toolCalls,omitempty"`
}

type aguiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type aguiInputContent struct {
	Type   string `json:"type"`
	Text   string `json:"text,omitempty"`
	Source struct {
		Type     string `json:"type"`
		Value    string `json:"value"`
		MimeType string `json:"mimeType,omitempty"`
	} `json:"source,omitempty"`
}

func (decoder) Decode(reader io.Reader) (uistream.Request, error) {
	var input *runAgentInput
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&input); err != nil {
		return uistream.Request{}, err
	}
	if input == nil {
		return uistream.Request{}, errors.New("agui: null request")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return uistream.Request{}, errors.New("agui: multiple JSON values")
		}
		return uistream.Request{}, err
	}
	if input.ThreadID == "" || input.RunID == "" {
		return uistream.Request{}, errors.New("agui: threadId and runId are required")
	}

	messages := make([]aikit.Message, 0, len(input.Messages))
	toolNames := make(map[string]string)
	for _, message := range input.Messages {
		for _, call := range message.ToolCalls {
			toolNames[call.ID] = call.Function.Name
		}
	}
	for _, message := range input.Messages {
		converted, err := convertMessage(message, toolNames)
		if err != nil {
			return uistream.Request{}, err
		}
		if err := converted.Validate(); err != nil {
			return uistream.Request{}, fmt.Errorf("agui: invalid %s message: %w", message.Role, err)
		}
		messages = append(messages, converted)
	}
	extra := map[string]any{
		"runId": input.RunID, "threadId": input.ThreadID, "parentRunId": input.ParentRunID,
		"state": input.State, "tools": input.Tools, "context": input.Context,
		"forwardedProps": input.ForwardedProps,
	}
	return uistream.Request{Messages: messages, ID: input.ThreadID, Extra: extra}, nil
}

func convertMessage(message aguiMessage, toolNames map[string]string) (aikit.Message, error) {
	role := aikit.Role(message.Role)
	if message.Role == "developer" {
		role = aikit.RoleSystem
	}
	converted := aikit.Message{Role: role}
	switch role {
	case aikit.RoleSystem:
		text, err := stringContent(message.Content)
		if err != nil {
			return aikit.Message{}, err
		}
		converted.Content = []aikit.ContentPart{aikit.TextPart(text)}
	case aikit.RoleUser:
		parts, err := userContent(message.Content)
		if err != nil {
			return aikit.Message{}, err
		}
		converted.Content = parts
	case aikit.RoleAssistant:
		converted.ID = message.ID
		text, err := optionalStringContent(message.Content)
		if err != nil {
			return aikit.Message{}, err
		}
		if text != "" {
			converted.Content = append(converted.Content, aikit.TextPart(text))
		}
		for _, call := range message.ToolCalls {
			if call.Type != "function" || !json.Valid([]byte(call.Function.Arguments)) {
				return aikit.Message{}, fmt.Errorf("agui: invalid function tool call %q", call.ID)
			}
			converted.Content = append(
				converted.Content,
				aikit.ToolCallPart(call.ID, call.Function.Name, json.RawMessage(call.Function.Arguments)),
			)
		}
		if len(converted.Content) == 0 {
			return aikit.Message{}, errors.New("agui: assistant message has no content or tool calls")
		}
	case aikit.RoleTool:
		text, err := stringContent(message.Content)
		if err != nil {
			return aikit.Message{}, err
		}
		name := message.Name
		if name == "" {
			name = toolNames[message.ToolCallID]
		}
		if name == "" {
			return aikit.Message{}, fmt.Errorf("agui: tool result %q has no preceding tool call", message.ToolCallID)
		}
		converted.Content = []aikit.ContentPart{aikit.ToolResultPart(message.ToolCallID, name, text)}
	default:
		return aikit.Message{}, fmt.Errorf("agui: unsupported message role %q", message.Role)
	}
	return converted, nil
}

func stringContent(raw json.RawMessage) (string, error) {
	text, err := optionalStringContent(raw)
	if err != nil {
		return "", err
	}
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", errors.New("agui: message content is required")
	}
	return text, nil
}

func optionalStringContent(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return "", errors.New("agui: message content must be a string")
	}
	return text, nil
}

func userContent(raw json.RawMessage) ([]aikit.ContentPart, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, errors.New("agui: user content is required")
	}
	if text, err := optionalStringContent(raw); err == nil {
		return []aikit.ContentPart{aikit.TextPart(text)}, nil
	}
	var content []aguiInputContent
	if err := json.Unmarshal(raw, &content); err != nil {
		return nil, errors.New("agui: user content must be a string or InputContent array")
	}
	parts := make([]aikit.ContentPart, 0, len(content))
	for _, item := range content {
		part, err := convertInputContent(item)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return nil, errors.New("agui: user content must not be empty")
	}
	return parts, nil
}

func convertInputContent(content aguiInputContent) (aikit.ContentPart, error) {
	if content.Type == "text" {
		return aikit.TextPart(content.Text), nil
	}
	if content.Source.Value == "" {
		return aikit.ContentPart{}, fmt.Errorf("agui: %s content requires a source", content.Type)
	}
	if content.Source.Type == "url" {
		switch content.Type {
		case "image":
			return aikit.ImageURLPart(content.Source.Value), nil
		case "audio":
			return aikit.AudioURLPart(content.Source.Value, content.Source.MimeType), nil
		case "video":
			return aikit.VideoURLPart(content.Source.Value, content.Source.MimeType), nil
		case "document":
			return aikit.DocumentURLPart(content.Source.Value, content.Source.MimeType), nil
		}
	}
	if content.Source.Type == "data" {
		data, err := base64.StdEncoding.DecodeString(content.Source.Value)
		if err != nil {
			return aikit.ContentPart{}, fmt.Errorf("agui: decode %s content: %w", content.Type, err)
		}
		switch content.Type {
		case "image":
			return aikit.ImageDataPart(data, content.Source.MimeType), nil
		case "audio":
			return aikit.AudioDataPart(data, content.Source.MimeType), nil
		case "video":
			return aikit.VideoDataPart(data, content.Source.MimeType), nil
		case "document":
			return aikit.DocumentDataPart(data, content.Source.MimeType), nil
		}
	}
	return aikit.ContentPart{}, fmt.Errorf("agui: unsupported %s source type %q", content.Type, content.Source.Type)
}

func newRunID() string {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "run"
	}
	return "run_" + hex.EncodeToString(value[:])
}
