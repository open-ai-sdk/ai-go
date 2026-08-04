package anthropic

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/transport"
)

// LanguageModel implements aikit.LanguageModel using the Anthropic Messages API.
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
		BaseURL:  cfg.BaseURL,
		Provider: "anthropic",
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
func (m *LanguageModel) Stream(ctx context.Context, req llm.Request) (<-chan aikit.StreamEvent, error) {
	body, encodeWarnings, err := m.encodeRequest(req, true)
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
	return m.client.DoStream(
		ctx,
		httpReq,
		nil,
		func(
			ctx context.Context,
			reader *transport.SSEReader,
			events chan<- aikit.StreamEvent,
		) error {
			return decodeSSEStream(
				ctx,
				reader,
				events,
				encodeWarnings...,
			)
		},
	)
}

// anthropicRequest is the Messages API request body.
type anthropicRequest struct {
	Model      string                 `json:"model"`
	MaxTokens  int                    `json:"max_tokens"`
	System     string                 `json:"system,omitempty"`
	Messages   []anthropicMsg         `json:"messages"`
	Stream     bool                   `json:"stream"`
	Tools      []anthropicTool        `json:"tools,omitempty"`
	ToolChoice *anthropicToolChoice   `json:"tool_choice,omitempty"`
	Thinking   *thinkingConfig        `json:"thinking,omitempty"`
	Output     *anthropicOutputConfig `json:"output_config,omitempty"`
}

type anthropicOutputConfig struct {
	Format anthropicOutputFormat `json:"format"`
}

type anthropicOutputFormat struct {
	Type   string         `json:"type"`
	Schema map[string]any `json:"schema"`
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
	Content      any             `json:"content,omitempty"`
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
func (m *LanguageModel) encodeRequest(req llm.Request, streaming bool) ([]byte, []aikit.Warning, error) {
	ar := anthropicRequest{
		Model:     m.modelID,
		MaxTokens: req.Settings.MaxTokens,
		Stream:    streaming,
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
	if req.Output != nil && req.Output.Type != "text" {
		if !m.supportsStructuredOutput() {
			return nil, nil, &llm.StructuredOutputError{
				Kind:   llm.StructuredOutputErrorKindPrompt,
				Reason: "anthropic model does not support native structured output",
			}
		}
		schema := req.Output.Schema
		if schema == nil {
			schema = map[string]any{"type": req.Output.Type}
		}
		ar.Output = &anthropicOutputConfig{Format: anthropicOutputFormat{Type: "json_schema", Schema: schema}}
	}

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

// supportsStructuredOutput mirrors the capability list maintained by the
// upstream Anthropic AI SDK. Unknown and older model IDs fail closed so callers
// get a local typed error rather than an avoidable provider 400.
func (m *LanguageModel) supportsStructuredOutput() bool {
	for _, model := range []string{
		"claude-opus-4-8", "claude-opus-4-7", "claude-fable-5", "claude-sonnet-5",
		"claude-sonnet-4-6", "claude-opus-4-6", "claude-sonnet-4-5",
		"claude-opus-4-5", "claude-haiku-4-5", "claude-opus-4-1",
	} {
		if modelIDMatches(m.modelID, model) {
			return true
		}
	}
	return false
}

func modelIDMatches(modelID, supported string) bool {
	if modelID == supported {
		return true
	}
	suffix := strings.TrimPrefix(modelID, supported)
	if len(suffix) != 9 || suffix[0] != '-' {
		return false
	}
	for _, char := range suffix[1:] {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

// NativeSchemaSupport exposes the per-model structured-output capability.
func (m *LanguageModel) NativeSchemaSupport() llm.NativeSchemaSupport {
	if m.supportsStructuredOutput() {
		return llm.NativeSchemaFull
	}
	return llm.NativeSchemaNone
}

func mapToolChoice(tc *aikit.ToolChoice) *anthropicToolChoice {
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

func encodeMessages(msgs []aikit.Message) ([]anthropicMsg, []aikit.Warning) {
	var out []anthropicMsg
	var warnings []aikit.Warning
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

func encodeContentPart(part aikit.ContentPart) (*contentBlock, []aikit.Warning) {
	switch part.Type {
	case aikit.ContentPartTypeText:
		return &contentBlock{Type: "text", Text: part.Text}, nil
	case aikit.ContentPartTypeFile, aikit.ContentPartTypeImage, aikit.ContentPartTypeDocument:
		return encodeFilePart(part)
	case aikit.ContentPartTypeAudio, aikit.ContentPartTypeVideo:
		return nil, []aikit.Warning{{
			Type: "unsupported-setting", Setting: string(part.Type),
			Message: fmt.Sprintf("anthropic: content part type %q is not supported", part.Type),
		}}
	case aikit.ContentPartTypeToolCall:
		return &contentBlock{
			Type:  "tool_use",
			ID:    part.ToolCallID,
			Name:  part.ToolCallName,
			Input: part.ToolCallArgs,
		}, nil
	case aikit.ContentPartTypeToolResult:
		content, warnings := encodeAnthropicToolResult(part)
		return &contentBlock{
			Type:      "tool_result",
			ToolUseID: part.ToolResultID,
			Content:   content,
		}, warnings
	case aikit.ContentPartTypeReasoning:
		return &contentBlock{
			Type:      "thinking",
			Thinking:  part.ReasoningText,
			Signature: part.ThoughtSignature,
		}, nil
	default:
		return nil, nil
	}
}

func encodeAnthropicToolResult(part aikit.ContentPart) (any, []aikit.Warning) {
	if len(part.ToolResultContent) == 0 {
		return part.ToolResultOutput, nil
	}
	content := make([]map[string]any, 0, len(part.ToolResultContent))
	var warnings []aikit.Warning
	for _, item := range part.ToolResultContent {
		switch item.Type {
		case aikit.ToolResultContentTypeText:
			content = append(content, map[string]any{"type": "text", "text": item.Text})
		case aikit.ToolResultContentTypeJSON:
			content = append(content, map[string]any{"type": "text", "text": string(item.JSON)})
		case aikit.ToolResultContentTypeImage:
			mediaType := item.MediaType
			if mediaType == "" {
				mediaType = "image/png"
			}
			content = append(content, map[string]any{"type": "image", "source": map[string]any{
				"type": "base64", "media_type": mediaType,
				"data": base64.StdEncoding.EncodeToString(item.Data),
			}})
		default:
			warnings = append(warnings, aikit.Warning{
				Type: "unsupported-setting", Setting: item.Type,
				Message: fmt.Sprintf("anthropic: unsupported tool result content type %q", item.Type),
			})
		}
	}
	if len(content) == 0 && part.ToolResultOutput != "" {
		return part.ToolResultOutput, warnings
	}
	return content, warnings
}

// encodeFilePart maps a file content part onto an Anthropic content block.
// Anthropic splits what the SDK models as one file part into two block kinds:
// "image" (jpeg/png/gif/webp only) and "document" (PDF). Anything else, and any
// part the Messages API cannot reference, is dropped with a warning rather than
// silently vanishing from the request.
func encodeFilePart(part aikit.ContentPart) (*contentBlock, []aikit.Warning) {
	if len(part.Data) == 0 {
		ref := part.FileURL
		setting := "fileURL"
		if ref == "" {
			ref, setting = part.FileID, "fileID"
		}
		return nil, []aikit.Warning{{
			Type:    "unsupported-setting",
			Setting: setting,
			Message: fmt.Sprintf(
				"anthropic: only inline file data is supported; dropping file part %q", ref,
			),
		}}
	}

	blockType, ok := anthropicFileBlockType(part.MediaType)
	if !ok {
		return nil, []aikit.Warning{{
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

func encodeTools(tools []aikit.ToolDefinition) []anthropicTool {
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
