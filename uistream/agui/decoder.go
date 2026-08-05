package agui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/open-ai-sdk/ai-go/uistream"
)

type decoder struct{}

type runAgentInput struct {
	ThreadID    string          `json:"threadId"`
	RunID       string          `json:"runId"`
	ParentRunID string          `json:"parentRunId,omitempty"`
	State       json.RawMessage `json:"state"`
	// Messages stay raw so the exact bytes the client sent can be republished
	// in MESSAGES_SNAPSHOT. Re-marshalling the decoded form would drop every
	// field this package does not model — including the `parts` passthrough
	// that carries tool-call UI state.
	Messages       []json.RawMessage `json:"messages"`
	Tools          json.RawMessage   `json:"tools"`
	Context        json.RawMessage   `json:"context"`
	ForwardedProps any               `json:"forwardedProps"`
	Resume         []ResumeEntry     `json:"resume,omitempty"`
}

// decodeMessages parses the raw message array into the shape this package
// works with, keeping the raw form available alongside it.
func decodeMessages(raw []json.RawMessage) ([]aguiMessage, error) {
	messages := make([]aguiMessage, 0, len(raw))
	for _, item := range raw {
		var message aguiMessage
		if err := json.Unmarshal(item, &message); err != nil {
			return nil, fmt.Errorf("agui: invalid message: %w", err)
		}
		messages = append(messages, message)
	}
	return messages, nil
}

// ResumeEntry is one client decision resolving a pending interrupt. It is
// published on Request.Extra["resume"] as []ResumeEntry.
//
// Status is "resolved" when the client supplied a decision and "cancelled" when
// it abandoned the interrupt. Payload holds the decision itself: for a tool
// approval that is {"approved": bool, "reason": string}; for a client tool it
// is the tool's raw output, unwrapped.
type ResumeEntry struct {
	InterruptID string          `json:"interruptId"`
	Status      string          `json:"status"`
	Payload     json.RawMessage `json:"payload,omitempty"`
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
	} `json:"source"`
}

func (decoder) Decode(reader io.Reader) (uistream.Request, error) {
	var input *runAgentInput
	jsonDecoder := json.NewDecoder(reader)
	if err := jsonDecoder.Decode(&input); err != nil {
		return uistream.Request{}, err
	}
	if input == nil {
		return uistream.Request{}, errors.New("agui: null request")
	}
	var trailing any
	if err := jsonDecoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return uistream.Request{}, errors.New("agui: multiple JSON values")
		}
		return uistream.Request{}, err
	}
	if input.ThreadID == "" || input.RunID == "" {
		return uistream.Request{}, errors.New("agui: threadId and runId are required")
	}

	inbound, err := decodeMessages(input.Messages)
	if err != nil {
		return uistream.Request{}, err
	}
	messages, err := convertMessages(inbound)
	if err != nil {
		return uistream.Request{}, err
	}

	extra := map[string]any{
		"runId": input.RunID, "threadId": input.ThreadID, "parentRunId": input.ParentRunID,
		"state": input.State, "tools": input.Tools, "context": input.Context,
		"forwardedProps": input.ForwardedProps,
		// The verbatim request history, republished in MESSAGES_SNAPSHOT so an
		// interrupt does not truncate the client's transcript.
		messagesExtraKey: input.Messages,
	}
	if len(input.Resume) > 0 {
		extra["resume"] = input.Resume
	}
	return uistream.Request{Messages: messages, ID: input.ThreadID, Extra: extra}, nil
}
