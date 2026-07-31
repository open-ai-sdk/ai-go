package anthropic

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/open-ai-sdk/ai-go/ai"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/transport"
)

// LanguageModel implements ai.LanguageModel using the Anthropic Messages API.
type LanguageModel struct {
	modelID   string
	config    Config
	client    *transport.Client
	clientErr error
}

var _ llm.Model = (*LanguageModel)(nil)

// NewLanguageModel creates a native Anthropic language model.
func NewLanguageModel(modelID string, cfg Config) *LanguageModel {
	cfg = cfg.withDefaults()
	client, clientErr := transport.NewClient(transport.ClientConfig{
		BaseURL: cfg.BaseURL,
		Headers: http.Header{
			"Content-Type":      []string{"application/json"},
			"Anthropic-Version": []string{cfg.APIVersion},
		},
		Auth: func(req *http.Request) {
			req.Header.Set("X-API-Key", cfg.APIKey)
		},
		HTTPClient: transport.NewStreamingClient(cfg.Timeout),
	})
	return &LanguageModel{
		modelID:   modelID,
		config:    cfg,
		client:    client,
		clientErr: clientErr,
	}
}

// ModelID returns the model identifier.
func (m *LanguageModel) ModelID() string { return m.modelID }

// Stream sends a streaming request to the Anthropic Messages API.
func (m *LanguageModel) Stream(ctx context.Context, req llm.Request) (<-chan ai.StreamEvent, error) {
	body, encodeWarnings, err := m.encodeRequest(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: encode request: %w", err)
	}

	if m.clientErr != nil {
		return nil, fmt.Errorf("anthropic: configure transport: %w", m.clientErr)
	}
	httpReq, err := m.client.NewRequest(
		ctx, http.MethodPost,
		"v1/messages",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("anthropic: build http request: %w", err)
	}
	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: http request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// Typed error carrying status/code/message/request-ID/Retry-After; the
		// raw body is parsed then discarded, never embedded.
		return nil, transport.APIErrorFromResponse(ctx, "anthropic", resp)
	}

	return transport.Stream(
		ctx,
		resp,
		func(
			ctx context.Context,
			reader *transport.SSEReader,
			events chan<- ai.StreamEvent,
		) error {
			return decodeSSEStream(
				ctx,
				reader,
				events,
				encodeWarnings...,
			)
		},
	), nil
}

// anthropicRequest is the Messages API request body.
type anthropicRequest struct {
	Model      string               `json:"model"`
	MaxTokens  int                  `json:"max_tokens"`
	System     string               `json:"system,omitempty"`
	Messages   []anthropicMsg       `json:"messages"`
	Stream     bool                 `json:"stream"`
	Tools      []anthropicTool      `json:"tools,omitempty"`
	ToolChoice *anthropicToolChoice `json:"tool_choice,omitempty"`
	Thinking   *thinkingConfig      `json:"thinking,omitempty"`
}

type anthropicToolChoice struct {
	Type string `json:"type"`           // "auto", "any", "tool", "none"
	Name string `json:"name,omitempty"` // set when Type == "tool"
}

type anthropicMsg struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type         string          `json:"type"`
	Text         string          `json:"text,omitempty"`
	Source       *imageSource    `json:"source,omitempty"`
	ID           string          `json:"id,omitempty"`
	Name         string          `json:"name,omitempty"`
	Input        json.RawMessage `json:"input,omitempty"`
	ToolUseID    string          `json:"tool_use_id,omitempty"`
	Content      string          `json:"content,omitempty"`
	CacheControl *cacheControl   `json:"cache_control,omitempty"`
	Thinking     string          `json:"thinking,omitempty"`
	Signature    string          `json:"signature,omitempty"`
}

type imageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type cacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

type anthropicTool struct {
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	InputSchema  map[string]any `json:"input_schema"`
	CacheControl *cacheControl  `json:"cache_control,omitempty"`
}

type thinkingConfig struct {
	Type         string `json:"type"` // "enabled"
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

// encodeRequest builds the Messages API body. Returned warnings describe content
// the API cannot carry; they are surfaced on the stream's finish event.
func (m *LanguageModel) encodeRequest(req llm.Request) ([]byte, []ai.Warning, error) {
	if req.Output != nil {
		return nil, nil, fmt.Errorf("anthropic: output schema is not yet supported")
	}

	ar := anthropicRequest{
		Model:     m.modelID,
		MaxTokens: req.Settings.MaxTokens,
		Stream:    true,
	}
	if ar.MaxTokens == 0 {
		ar.MaxTokens = 8192
	}

	ar.System = req.Instructions
	ar.ToolChoice = mapToolChoice(req.ToolChoice)
	thinking, err := extractThinkingConfig(req.ProviderOptions)
	if err != nil {
		return nil, nil, err
	}
	ar.Thinking = thinking
	msgs, warnings := encodeMessages(req.Messages)
	ar.Messages = msgs
	ar.Tools = encodeTools(req.Tools)

	// Enable caching on last tool if caching is enabled
	if m.config.EnableCaching && len(ar.Tools) > 0 {
		ar.Tools[len(ar.Tools)-1].CacheControl = &cacheControl{Type: "ephemeral"}
	}

	body, err := json.Marshal(ar)
	if err != nil {
		return nil, nil, err
	}
	return body, warnings, nil
}

func mapToolChoice(tc *ai.ToolChoice) *anthropicToolChoice {
	if tc == nil {
		return nil
	}
	switch tc.Type {
	case "auto":
		return &anthropicToolChoice{Type: "auto"}
	case "none":
		return &anthropicToolChoice{Type: "none"}
	case "required":
		return &anthropicToolChoice{Type: "any"}
	case "tool":
		return &anthropicToolChoice{Type: "tool", Name: tc.ToolName}
	default:
		return nil
	}
}

func extractThinkingConfig(options map[string]any) (*thinkingConfig, error) {
	opts, err := parseProviderOptions(options)
	if err != nil {
		return nil, err
	}
	if !opts.Thinking {
		return nil, nil
	}
	budget := 10000
	if opts.ThinkingBudget != 0 {
		budget = opts.ThinkingBudget
	}
	return &thinkingConfig{Type: "enabled", BudgetTokens: budget}, nil
}

func encodeMessages(msgs []ai.Message) ([]anthropicMsg, []ai.Warning) {
	var out []anthropicMsg
	var warnings []ai.Warning
	for _, msg := range msgs {
		am := anthropicMsg{Role: string(msg.Role)}
		for _, part := range msg.Content {
			cb, w := encodeContentPart(part)
			warnings = append(warnings, w...)
			if cb != nil {
				am.Content = append(am.Content, *cb)
			}
		}
		if len(am.Content) > 0 {
			out = append(out, am)
		}
	}
	return out, warnings
}

func encodeContentPart(part ai.ContentPart) (*contentBlock, []ai.Warning) {
	switch part.Type {
	case ai.ContentPartTypeText:
		return &contentBlock{Type: "text", Text: part.Text}, nil
	case ai.ContentPartTypeFile:
		return encodeFilePart(part)
	case ai.ContentPartTypeToolCall:
		return &contentBlock{
			Type:  "tool_use",
			ID:    part.ToolCallID,
			Name:  part.ToolCallName,
			Input: part.ToolCallArgs,
		}, nil
	case ai.ContentPartTypeToolResult:
		return &contentBlock{
			Type:      "tool_result",
			ToolUseID: part.ToolResultID,
			Content:   part.ToolResultOutput,
		}, nil
	case ai.ContentPartTypeReasoning:
		return &contentBlock{
			Type:      "thinking",
			Thinking:  part.ReasoningText,
			Signature: part.ThoughtSignature,
		}, nil
	default:
		return nil, nil
	}
}

// encodeFilePart maps a file content part onto an Anthropic content block.
// Anthropic splits what the SDK models as one file part into two block kinds:
// "image" (jpeg/png/gif/webp only) and "document" (PDF). Anything else, and any
// part the Messages API cannot reference, is dropped with a warning rather than
// silently vanishing from the request.
func encodeFilePart(part ai.ContentPart) (*contentBlock, []ai.Warning) {
	if len(part.Data) == 0 {
		ref := part.FileURL
		setting := "fileURL"
		if ref == "" {
			ref, setting = part.FileID, "fileID"
		}
		return nil, []ai.Warning{{
			Type:    "unsupported-setting",
			Setting: setting,
			Message: fmt.Sprintf(
				"anthropic: only inline file data is supported; dropping file part %q", ref,
			),
		}}
	}

	blockType, ok := anthropicFileBlockType(part.MediaType)
	if !ok {
		return nil, []ai.Warning{{
			Type:    "unsupported-setting",
			Setting: "mediaType",
			Message: fmt.Sprintf(
				"anthropic: media type %q is not supported as an image or document; dropping file part",
				part.MediaType,
			),
		}}
	}

	return &contentBlock{
		Type: blockType,
		Source: &imageSource{
			Type:      "base64",
			MediaType: part.MediaType,
			Data:      base64.StdEncoding.EncodeToString(part.Data),
		},
	}, nil
}

// anthropicFileBlockType resolves a media type to the Anthropic block kind that
// accepts it. A bare "image" segment is treated as an image; the API rejects the
// bare segment as a source media_type, so callers must supply a full type there.
func anthropicFileBlockType(mediaType string) (string, bool) {
	switch {
	case mediaType == "application/pdf":
		return "document", true
	case strings.HasPrefix(mediaType, "image/"), mediaType == "image":
		return "image", true
	default:
		return "", false
	}
}

func encodeTools(tools []ai.ToolDefinition) []anthropicTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthropicTool, len(tools))
	for i, tool := range tools {
		out[i] = anthropicTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		}
	}
	return out
}
