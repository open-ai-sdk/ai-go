package agui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func convertMessages(inbound []aguiMessage) ([]aikit.Message, error) {
	toolNames := make(map[string]string)
	for _, message := range inbound {
		for _, call := range message.ToolCalls {
			toolNames[call.ID] = call.Function.Name
		}
	}
	messages := make([]aikit.Message, 0, len(inbound))
	for _, message := range inbound {
		converted, ok, err := convertMessage(message, toolNames)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if err := converted.Validate(); err != nil {
			return nil, fmt.Errorf("agui: invalid %s message: %w", message.Role, err)
		}
		messages = append(messages, converted)
	}
	return messages, nil
}

// convertMessage maps one AG-UI message. The bool reports whether the message
// belongs in the engine's history at all: clients fan assistant turns out into
// extra rows this vocabulary has no home for.
func convertMessage(message aguiMessage, toolNames map[string]string) (aikit.Message, bool, error) {
	role := aikit.Role(message.Role)
	switch message.Role {
	case "developer":
		role = aikit.RoleSystem
	case "reasoning", "activity":
		// Assistant thinking is already carried by the anchor assistant message,
		// and activity rows are UI-only. Dropping them keeps a normal
		// conversation from failing the request.
		return aikit.Message{}, false, nil
	}

	converted := aikit.Message{Role: role}
	switch role {
	case aikit.RoleSystem:
		text, err := stringContent(message.Content)
		if err != nil {
			return aikit.Message{}, false, err
		}
		converted.Content = []aikit.ContentPart{aikit.TextPart(text)}
	case aikit.RoleUser:
		parts, err := userContent(message.Content)
		if err != nil {
			return aikit.Message{}, false, err
		}
		converted.Content = parts
	case aikit.RoleAssistant:
		if err := assistantContent(&converted, message); err != nil {
			return aikit.Message{}, false, err
		}
	case aikit.RoleTool:
		text, err := stringContent(message.Content)
		if err != nil {
			return aikit.Message{}, false, err
		}
		name := message.Name
		if name == "" {
			name = toolNames[message.ToolCallID]
		}
		if name == "" {
			return aikit.Message{}, false,
				fmt.Errorf("agui: tool result %q has no preceding tool call", message.ToolCallID)
		}
		converted.Content = []aikit.ContentPart{
			aikit.ToolResultPart(message.ToolCallID, name, text),
		}
	default:
		return aikit.Message{}, false, fmt.Errorf("agui: unsupported message role %q", message.Role)
	}
	return converted, true, nil
}

func assistantContent(converted *aikit.Message, message aguiMessage) error {
	converted.ID = message.ID
	text, err := optionalStringContent(message.Content)
	if err != nil {
		return err
	}
	if text != "" {
		converted.Content = append(converted.Content, aikit.TextPart(text))
	}
	for _, call := range message.ToolCalls {
		if call.Type != "function" || !json.Valid([]byte(call.Function.Arguments)) {
			return fmt.Errorf("agui: invalid function tool call %q", call.ID)
		}
		converted.Content = append(converted.Content,
			aikit.ToolCallPart(call.ID, call.Function.Name, json.RawMessage(call.Function.Arguments)))
	}
	if len(converted.Content) == 0 {
		return errors.New("agui: assistant message has no content or tool calls")
	}
	return nil
}

func stringContent(raw json.RawMessage) (string, error) {
	text, err := optionalStringContent(raw)
	if err != nil {
		return "", err
	}
	if isAbsent(raw) {
		return "", errors.New("agui: message content is required")
	}
	return text, nil
}

func optionalStringContent(raw json.RawMessage) (string, error) {
	if isAbsent(raw) {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return "", errors.New("agui: message content must be a string")
	}
	return text, nil
}

func isAbsent(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}
