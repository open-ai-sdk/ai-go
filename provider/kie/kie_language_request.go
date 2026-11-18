package kie

import (
	"encoding/json"
	"fmt"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/provider/openaicompat"
)

// LanguageOptions supplies request-local Kie language controls. The SDK does
// not select tools, reasoning effort, schema names, or endpoint routes by model.
// NativeTools contains provider-native wire objects, separate from Request.Tools
// (ordinary callable functions). For example, Responses accepts {"type":
// "web_search"}; Chat search can use {"type":"function","function":
// {"name":"googleSearch"}} without adding a parameters field.
type LanguageOptions struct {
	ReasoningEffort string           `json:"reasoningEffort,omitempty"`
	SchemaName      string           `json:"schemaName,omitempty"`
	NativeTools     []map[string]any `json:"nativeTools,omitempty"`
}

func (LanguageOptions) ProviderName() string { return "kie" }

func languageOptions(req llm.Request) (LanguageOptions, error) {
	raw, exists := req.ProviderOptions["kie"]
	if !exists {
		return LanguageOptions{}, nil
	}
	switch options := raw.(type) {
	case LanguageOptions:
		return options, nil
	case *LanguageOptions:
		if options != nil {
			return *options, nil
		}
	case map[string]any:
		var decoded LanguageOptions
		if err := llm.DecodeJSONProviderOptions("kie", options, &decoded); err != nil {
			return LanguageOptions{}, err
		}
		return decoded, nil
	}
	return LanguageOptions{}, llm.ProviderOptionTypeError("kie", raw)
}

// RequestBody exposes the same request object used by Complete for diagnostics.
// It performs no network I/O. The returned object may contain private prompts,
// image URLs and schemas; callers own redaction and must not mutate nested
// request-owned schemas or native tool objects.
func (m *LanguageModel) RequestBody(req llm.Request) (map[string]any, error) {
	body, _, err := m.languageRequestBody(req)
	return body, err
}

func (m *LanguageModel) languageRequestBody(req llm.Request) (map[string]any, []aikit.Warning, error) {
	options, err := languageOptions(req)
	if err != nil {
		return nil, nil, err
	}
	// Reuse the compatible encoder for function schemas, normalized messages,
	// tool results, inline images and native structured-output formatting.
	encoded, warnings, err := openaicompat.EncodeRequest(openaicompat.EncodeRequestParams{
		ModelID: m.modelID, SupportsStructuredOutput: true,
	}, req, false)
	if err != nil {
		return nil, nil, err
	}
	if encoded.ResponseFormat != nil && encoded.ResponseFormat.JSONSchema != nil {
		schema := encoded.ResponseFormat.JSONSchema
		schema.Schema = req.Output.Schema
		if options.SchemaName != "" {
			schema.Name = options.SchemaName
		} else if req.Output.Name != "" {
			schema.Name = req.Output.Name
		}
	}
	var body map[string]any
	if m.chat {
		body = chatLanguageRequestBody(req, options, &encoded)
	} else {
		body, err = m.responsesLanguageRequestBody(req, options, &encoded)
		if err != nil {
			return nil, nil, err
		}
	}
	if req.Settings.Temperature != nil {
		body["temperature"] = *req.Settings.Temperature
	}
	if req.Settings.TopP != nil {
		body["top_p"] = *req.Settings.TopP
	}
	if req.Settings.Seed != nil {
		body["seed"] = *req.Settings.Seed
	}
	if len(req.Settings.StopSequences) > 0 {
		body["stop"] = req.Settings.StopSequences
	}
	return body, warnings, nil
}

func chatLanguageRequestBody(
	req llm.Request,
	options LanguageOptions,
	encoded *openaicompat.ChatRequest,
) map[string]any {
	for _, message := range encoded.Messages {
		// Kie requires typed arrays even for text-only messages.
		// Tool results keep their Chat protocol representation.
		if message["role"] != "tool" {
			if text, ok := message["content"].(string); ok {
				message["content"] = []map[string]any{{"type": "text", "text": text}}
			}
		}
	}
	body := map[string]any{"messages": encoded.Messages, "stream": false, "include_thoughts": false}
	if options.ReasoningEffort != "" {
		body["reasoning_effort"] = options.ReasoningEffort
	}
	if encoded.ResponseFormat != nil {
		body["response_format"] = encoded.ResponseFormat
	}
	tools := append(encoded.Tools, options.NativeTools...)
	if len(tools) > 0 {
		body["tools"] = tools
	}
	if encoded.ToolChoice != nil {
		body["tool_choice"] = encoded.ToolChoice
	}
	if req.Settings.MaxTokens > 0 {
		body["max_tokens"] = req.Settings.MaxTokens
	}
	return body
}

func (m *LanguageModel) responsesLanguageRequestBody(
	req llm.Request,
	options LanguageOptions,
	encoded *openaicompat.ChatRequest,
) (map[string]any, error) {
	input, err := responsesLanguageInput(encoded.Messages)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"model": m.modelID, "input": input}
	if options.ReasoningEffort != "" {
		body["reasoning"] = map[string]any{"effort": options.ReasoningEffort}
	}
	if format := encoded.ResponseFormat; format != nil {
		wire := map[string]any{"type": format.Type}
		if schema := format.JSONSchema; schema != nil {
			wire["name"], wire["schema"], wire["strict"] = schema.Name, schema.Schema, schema.Strict
		}
		body["text"] = map[string]any{"format": wire}
	}
	tools := make([]map[string]any, 0, len(req.Tools)+len(options.NativeTools))
	for _, tool := range req.Tools {
		wire := map[string]any{
			"type": "function", "name": tool.Name,
			"description": tool.Description, "parameters": tool.InputSchema,
		}
		if tool.Strict {
			wire["strict"] = true
		}
		tools = append(tools, wire)
	}
	tools = append(tools, options.NativeTools...)
	if len(tools) > 0 {
		body["tools"] = tools
	}
	if choice := req.ToolChoice; choice != nil && len(tools) > 0 {
		if choice.Type == "tool" {
			body["tool_choice"] = map[string]any{"type": "function", "name": choice.ToolName}
		} else {
			body["tool_choice"] = choice.Type
		}
	}
	if req.Settings.MaxTokens > 0 {
		body["max_output_tokens"] = req.Settings.MaxTokens
	}
	return body, nil
}

func responsesLanguageInput(messages []map[string]any) ([]map[string]any, error) {
	input := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		if message["role"] == "tool" {
			output := message["content"]
			if _, ok := output.(string); !ok {
				encoded, err := json.Marshal(output)
				if err != nil {
					return nil, err
				}
				output = string(encoded)
			}
			input = append(
				input,
				map[string]any{"type": "function_call_output", "call_id": message["tool_call_id"], "output": output},
			)
			continue
		}
		parts := make([]map[string]any, 0)
		switch content := message["content"].(type) {
		case string:
			parts = append(parts, map[string]any{"type": "input_text", "text": content})
		case []map[string]any:
			for _, part := range content {
				switch part["type"] {
				case "text":
					parts = append(parts, map[string]any{"type": "input_text", "text": part["text"]})
				case "image_url":
					image, ok := part["image_url"].(map[string]string)
					if !ok {
						return nil, fmt.Errorf("kie: invalid encoded image content")
					}
					parts = append(parts, map[string]any{"type": "input_image", "image_url": image["url"]})
				default:
					return nil, fmt.Errorf("kie: unsupported Responses content type %v", part["type"])
				}
			}
		}
		if len(parts) > 0 {
			input = append(input, map[string]any{"role": message["role"], "content": parts})
		}
		if calls, ok := message["tool_calls"].([]map[string]any); ok {
			for _, call := range calls {
				function, ok := call["function"].(map[string]string)
				if !ok {
					function = nil
				}
				input = append(input, map[string]any{
					"type": "function_call", "call_id": call["id"],
					"name": function["name"], "arguments": function["arguments"],
				})
			}
		}
	}
	return input, nil
}
