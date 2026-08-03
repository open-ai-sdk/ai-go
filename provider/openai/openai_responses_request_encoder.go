package openai

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

// responsesRequest is the JSON body sent to the OpenAI Responses API POST /v1/responses.
type responsesRequest struct {
	Model              string            `json:"model"`
	Input              []inputItem       `json:"input"`
	Stream             bool              `json:"stream,omitempty"`
	MaxOutputTokens    int               `json:"max_output_tokens,omitempty"`
	Temperature        *float32          `json:"temperature,omitempty"`
	TopP               *float32          `json:"top_p,omitempty"`
	PreviousResponseID string            `json:"previous_response_id,omitempty"`
	Reasoning          *reasoningConfig  `json:"reasoning,omitempty"`
	Tools              []responsesTool   `json:"tools,omitempty"`
	Text               *textConfig       `json:"text,omitempty"`
	Store              *bool             `json:"store,omitempty"`
	User               string            `json:"user,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Include            []string          `json:"include,omitempty"`
}

type reasoningConfig struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type textConfig struct {
	Format *textFormat `json:"format,omitempty"`
}

type textFormat struct {
	Type       string         `json:"type"` // "text" or "json_schema"
	JSONSchema *jsonSchemaRef `json:"json_schema,omitempty"`
}

type jsonSchemaRef struct {
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema"`
	Strict bool           `json:"strict"`
}

// inputItem is a union type for all Responses API input items.
type inputItem struct {
	// Role-based messages.
	Role    string      `json:"role,omitempty"`
	Content []inputPart `json:"content,omitempty"`

	// Function call (assistant tool use).
	Type      string `json:"type,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`

	// Function call output (tool result).
	Output any `json:"output,omitempty"`
}

// inputPart is a single part inside a user or assistant message.
type inputPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	FileID   string `json:"file_id,omitempty"`
	FileURL  string `json:"file_url,omitempty"`
	Filename string `json:"filename,omitempty"`
}

// responsesTool describes a tool available to the model.
type responsesTool struct {
	Type        string         `json:"type"` // "function" or built-in name
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// encodeRequest builds a responsesRequest from an llm.Request.
func encodeRequest(modelID string, req llm.Request, stream bool) (responsesRequest, []aikit.Warning, error) {
	opts, err := parseProviderOptions(req.ProviderOptions)
	if err != nil {
		return responsesRequest{}, nil, err
	}
	var warnings []aikit.Warning

	input, encWarnings, err := encodeInput(req)
	if err != nil {
		return responsesRequest{}, nil, err
	}
	warnings = append(warnings, encWarnings...)

	r := responsesRequest{
		Model:  modelID,
		Input:  input,
		Stream: stream,
	}

	// Token limit: provider option takes precedence over settings.
	if opts.MaxOutputTokens > 0 {
		r.MaxOutputTokens = opts.MaxOutputTokens
	} else if req.Settings.MaxTokens > 0 {
		r.MaxOutputTokens = req.Settings.MaxTokens
	}

	if req.Settings.Temperature != nil {
		r.Temperature = req.Settings.Temperature
	}
	if req.Settings.TopP != nil {
		r.TopP = req.Settings.TopP
	}

	if len(req.Settings.StopSequences) > 0 {
		warnings = append(warnings, aikit.Warning{
			Type:    "unsupported-setting",
			Setting: "stopSequences",
			Message: "stopSequences is not supported by the OpenAI Responses API",
		})
	}

	if opts.PreviousResponseID != "" {
		r.PreviousResponseID = opts.PreviousResponseID
	}

	// Reasoning settings for o-series and gpt-5 reasoning models.
	if opts.ReasoningEffort != "" || opts.ReasoningSummary != "" {
		r.Reasoning = &reasoningConfig{
			Effort:  opts.ReasoningEffort,
			Summary: opts.ReasoningSummary,
		}
	}

	if opts.User != "" {
		r.User = opts.User
	}
	if opts.Metadata != nil {
		r.Metadata = opts.Metadata
	}
	if opts.Store != nil {
		r.Store = opts.Store
	}

	// Tools: function tools + optional built-in web search.
	tools, toolWarnings := encodeTools(req.Tools, opts)
	warnings = append(warnings, toolWarnings...)
	r.Tools = tools

	// Include list: add sources when web search + IncludeSources requested.
	if opts.EnableWebSearch && opts.IncludeSources {
		r.Include = append(r.Include, "web_search_call.action.sources")
	}

	// Structured output schema.
	if req.Output != nil && req.Output.Type != "text" {
		r.Text = encodeOutputSchema(req.Output)
	}

	return r, warnings, nil
}

// encodeInput converts system prompt + messages to Responses API input items.
func encodeInput(req llm.Request) ([]inputItem, []aikit.Warning, error) {
	var items []inputItem
	var warnings []aikit.Warning

	if req.Instructions != "" {
		items = append(items, inputItem{
			Role:    "system",
			Content: []inputPart{{Type: "input_text", Text: req.Instructions}},
		})
	}

	for _, m := range req.Messages {
		encoded, w, err := encodeMessage(m)
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, w...)
		items = append(items, encoded...)
	}
	return items, warnings, nil
}

func encodeMessage(m aikit.Message) ([]inputItem, []aikit.Warning, error) {
	switch m.Role {
	case aikit.RoleSystem:
		return encodeSystemMessage(m)
	case aikit.RoleUser:
		return encodeUserMessage(m)
	case aikit.RoleAssistant:
		return encodeAssistantMessage(m)
	case aikit.RoleTool:
		return encodeToolResultMessage(m)
	default:
		return nil, nil, fmt.Errorf("openai: unsupported message role %q", m.Role)
	}
}

// encodeSystemMessage handles instructions already materialized into the
// conversation by the agent runtime. The Responses API accepts system input
// items with text content; other message-part kinds have no system equivalent.
func encodeSystemMessage(m aikit.Message) ([]inputItem, []aikit.Warning, error) {
	var parts []inputPart
	var warnings []aikit.Warning

	for _, p := range m.Content {
		if p.Type == aikit.ContentPartTypeText {
			parts = append(parts, inputPart{Type: "input_text", Text: p.Text})
			continue
		}
		warnings = append(warnings, aikit.Warning{
			Type:    "unsupported-setting",
			Setting: string(p.Type),
			Message: fmt.Sprintf("openai: unsupported system content part type %q, skipping", p.Type),
		})
	}

	if len(parts) == 0 {
		return nil, warnings, nil
	}
	return []inputItem{{Role: "system", Content: parts}}, warnings, nil
}

func encodeUserMessage(m aikit.Message) ([]inputItem, []aikit.Warning, error) {
	var parts []inputPart
	var warnings []aikit.Warning

	for _, p := range m.Content {
		switch p.Type {
		case aikit.ContentPartTypeText:
			parts = append(parts, inputPart{Type: "input_text", Text: p.Text})

		case aikit.ContentPartTypeFile, aikit.ContentPartTypeImage:
			if strings.HasPrefix(p.MediaType, "image/") || p.MediaType == "image" {
				switch {
				case p.FileID != "":
					parts = append(parts, inputPart{Type: "input_image", FileID: p.FileID})
				case len(p.Data) > 0:
					mediaType := p.MediaType
					if mediaType == "" {
						mediaType = "image/png"
					}
					parts = append(
						parts,
						inputPart{
							Type:     "input_image",
							ImageURL: "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(p.Data),
						},
					)
				default:
					parts = append(parts, inputPart{Type: "input_image", ImageURL: p.FileURL})
				}
			} else if p.FileID != "" {
				parts = append(parts, inputPart{Type: "input_file", FileID: p.FileID, Filename: p.Filename})
			} else if len(p.Data) > 0 {
				mediaType := p.MediaType
				if mediaType == "" {
					mediaType = "application/octet-stream"
				}
				parts = append(
					parts,
					inputPart{
						Type:     "input_file",
						FileURL:  "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(p.Data),
						Filename: p.Filename,
					},
				)
			} else {
				parts = append(parts, inputPart{Type: "input_file", FileURL: p.FileURL, Filename: p.Filename})
			}
		case aikit.ContentPartTypeDocument:
			switch {
			case p.FileID != "":
				parts = append(parts, inputPart{Type: "input_file", FileID: p.FileID, Filename: p.Filename})
			case p.FileURL != "":
				parts = append(parts, inputPart{Type: "input_file", FileURL: p.FileURL, Filename: p.Filename})
			case len(p.Data) > 0:
				mediaType := p.MediaType
				if mediaType == "" {
					mediaType = "application/octet-stream"
				}
				parts = append(parts, inputPart{
					Type: "input_file", FileURL: "data:" + mediaType + ";base64," +
						base64.StdEncoding.EncodeToString(p.Data), Filename: p.Filename,
				})
			default:
				return nil, warnings, fmt.Errorf("openai: document part has no source")
			}
		case aikit.ContentPartTypeAudio, aikit.ContentPartTypeVideo:
			return nil, warnings, fmt.Errorf("openai: Responses API does not support %s input", p.Type)

		default:
			warnings = append(warnings, aikit.Warning{
				Type:    "unsupported-setting",
				Setting: string(p.Type),
				Message: fmt.Sprintf("openai: unsupported user content part type %q, skipping", p.Type),
			})
		}
	}

	if len(parts) == 0 {
		return nil, warnings, nil
	}
	return []inputItem{{Role: "user", Content: parts}}, warnings, nil
}

func encodeAssistantMessage(m aikit.Message) ([]inputItem, []aikit.Warning, error) {
	var items []inputItem

	var textParts []inputPart
	for _, p := range m.Content {
		switch p.Type {
		case aikit.ContentPartTypeText:
			textParts = append(textParts, inputPart{Type: "output_text", Text: p.Text})
		case aikit.ContentPartTypeToolCall:
			// Flush any accumulated text first.
			if len(textParts) > 0 {
				items = append(items, inputItem{Role: "assistant", Content: textParts})
				textParts = nil
			}
			items = append(items, inputItem{
				Type:      "function_call",
				CallID:    p.ToolCallID,
				Name:      p.ToolCallName,
				Arguments: string(p.ToolCallArgs),
			})
		case aikit.ContentPartTypeImage:
			return nil, nil, fmt.Errorf(
				"openai: Responses API cannot replay assistant image content",
			)
		}
	}
	if len(textParts) > 0 {
		items = append(items, inputItem{Role: "assistant", Content: textParts})
	}
	return items, nil, nil
}

func encodeToolResultMessage(m aikit.Message) ([]inputItem, []aikit.Warning, error) {
	var items []inputItem
	for _, p := range m.Content {
		if p.Type == aikit.ContentPartTypeToolResult {
			output, err := encodeResponsesToolResultOutput(p)
			if err != nil {
				return nil, nil, err
			}
			items = append(items, inputItem{
				Type:   "function_call_output",
				CallID: p.ToolResultID,
				Output: output,
			})
		}
	}
	return items, nil, nil
}

func encodeResponsesToolResultOutput(part aikit.ContentPart) (any, error) {
	if len(part.ToolResultContent) == 0 {
		return part.ToolResultOutput, nil
	}
	output := make([]inputPart, 0, len(part.ToolResultContent))
	for _, content := range part.ToolResultContent {
		switch content.Type {
		case aikit.ToolResultContentTypeText:
			output = append(output, inputPart{Type: "input_text", Text: content.Text})
		case aikit.ToolResultContentTypeJSON:
			if !json.Valid(content.JSON) {
				return nil, fmt.Errorf("openai: tool result contains invalid JSON")
			}
			output = append(output, inputPart{Type: "input_text", Text: string(content.JSON)})
		case aikit.ToolResultContentTypeImage:
			if len(content.Data) == 0 {
				return nil, fmt.Errorf("openai: image tool result has no data")
			}
			mediaType := content.MediaType
			if mediaType == "" {
				mediaType = "image/png"
			}
			output = append(output, inputPart{Type: "input_image", ImageURL: "data:" + mediaType +
				";base64," + base64.StdEncoding.EncodeToString(content.Data)})
		default:
			return nil, fmt.Errorf("openai: unsupported tool result content type %q", content.Type)
		}
	}
	return output, nil
}

func encodeTools(defs []aikit.ToolDefinition, opts ProviderOptions) ([]responsesTool, []aikit.Warning) {
	var tools []responsesTool

	for _, d := range defs {
		tools = append(tools, responsesTool{
			Type:        "function",
			Name:        d.Name,
			Description: d.Description,
			Parameters:  d.InputSchema,
		})
	}

	if opts.EnableWebSearch {
		tools = append(tools, responsesTool{Type: "web_search_preview"})
	}

	return tools, nil
}

func encodeOutputSchema(o *llm.OutputSchema) *textConfig {
	schema := o.Schema
	if o.Type == "object" && schema != nil {
		if _, ok := schema["type"]; !ok {
			wrapped := make(map[string]any, len(schema)+1)
			wrapped["type"] = "object"
			for k, v := range schema {
				wrapped[k] = v
			}
			schema = wrapped
		}
	}
	return &textConfig{
		Format: &textFormat{
			Type: "json_schema",
			JSONSchema: &jsonSchemaRef{
				Name:   "structured_output",
				Schema: schema,
				Strict: true,
			},
		},
	}
}
