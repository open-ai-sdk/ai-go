package gemini

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

// nativeRequest is the top-level JSON body for :streamGenerateContent.
type nativeRequest struct {
	Contents          []nativeContent          `json:"contents"`
	SystemInstruction *nativeSystemInstruction `json:"systemInstruction,omitempty"`
	GenerationConfig  *nativeGenerationConfig  `json:"generationConfig,omitempty"`
	SafetySettings    []any                    `json:"safetySettings,omitempty"`
	Tools             []any                    `json:"tools,omitempty"`
	ToolConfig        any                      `json:"toolConfig,omitempty"`
	CachedContent     string                   `json:"cachedContent,omitempty"`
	Labels            map[string]string        `json:"labels,omitempty"`
}

type nativeContent struct {
	Role  string       `json:"role"`
	Parts []nativePart `json:"parts"`
}

type nativePart struct {
	Text             string            `json:"text,omitempty"`
	Thought          *bool             `json:"thought,omitempty"`
	ThoughtSignature string            `json:"thoughtSignature,omitempty"`
	InlineData       *nativeInlineData `json:"inlineData,omitempty"`
	FileData         *nativeFileData   `json:"fileData,omitempty"`
	FunctionCall     *nativeFuncCall   `json:"functionCall,omitempty"`
	FunctionResponse *nativeFuncResp   `json:"functionResponse,omitempty"`
}

type nativeInlineData struct {
	MediaType string `json:"mimeType"`
	Data      string `json:"data"`
}

type nativeFileData struct {
	MediaType string `json:"mimeType"`
	FileUri   string `json:"fileUri"`
}

type nativeFuncCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type nativeFuncResp struct {
	Name     string                       `json:"name"`
	Response any                          `json:"response"`
	Parts    []nativeFunctionResponsePart `json:"parts,omitempty"`
}

type nativeFunctionResponsePart struct {
	InlineData *nativeInlineData `json:"inlineData,omitempty"`
}

type nativeSystemInstruction struct {
	Parts []nativeTextPart `json:"parts"`
}

type nativeTextPart struct {
	Text string `json:"text"`
}

type nativeGenerationConfig struct {
	MaxOutputTokens    *int               `json:"maxOutputTokens,omitempty"`
	Temperature        *float32           `json:"temperature,omitempty"`
	TopK               *int               `json:"topK,omitempty"`
	TopP               *float32           `json:"topP,omitempty"`
	StopSequences      []string           `json:"stopSequences,omitempty"`
	Seed               *int               `json:"seed,omitempty"`
	ResponseMimeType   string             `json:"responseMimeType,omitempty"`
	ResponseSchema     map[string]any     `json:"responseSchema,omitempty"`
	ThinkingConfig     *nativeThinkingCfg `json:"thinkingConfig,omitempty"`
	ResponseModalities []string           `json:"responseModalities,omitempty"`
	ImageConfig        *nativeImageConfig `json:"imageConfig,omitempty"`
}

type nativeThinkingCfg struct {
	ThinkingBudget  *int   `json:"thinkingBudget,omitempty"`
	IncludeThoughts *bool  `json:"includeThoughts,omitempty"`
	ThinkingLevel   string `json:"thinkingLevel,omitempty"`
}

type nativeImageConfig struct {
	AspectRatio string `json:"aspectRatio,omitempty"`
	ImageSize   string `json:"imageSize,omitempty"`
}

// encodeNativeRequest converts an aikit.LanguageModelRequest to the native Gemini
// request body for the :streamGenerateContent endpoint.
func encodeNativeRequest(req llm.Request) nativeRequest {
	return encodeNativeRequestForModel("", req)
}

func encodeNativeRequestForModel(modelID string, req llm.Request) nativeRequest {
	nr := nativeRequest{}

	// System instruction: collect from req.Instructions and any leading system messages.
	nr.SystemInstruction = buildSystemInstruction(req)

	// Convert messages to native contents.
	nr.Contents = buildContents(req.Messages, usesGemini3Features(modelID))

	// Generation config.
	nr.GenerationConfig = buildGenerationConfig(req)

	return nr
}

// buildSystemInstruction collects system text from req.Instructions and any leading
// RoleSystem messages in req.Messages.
func buildSystemInstruction(req llm.Request) *nativeSystemInstruction {
	var parts []nativeTextPart

	if req.Instructions != "" {
		parts = append(parts, nativeTextPart{Text: req.Instructions})
	}

	for _, msg := range req.Messages {
		if msg.Role != aikit.RoleSystem {
			break
		}
		for _, p := range msg.Content {
			if p.Type == aikit.ContentPartTypeText && p.Text != "" {
				parts = append(parts, nativeTextPart{Text: p.Text})
			}
		}
	}

	if len(parts) == 0 {
		return nil
	}
	return &nativeSystemInstruction{Parts: parts}
}

// buildContents converts the message list to native Gemini contents,
// skipping leading system messages (already handled by buildSystemInstruction).
func buildContents(messages []aikit.Message, supportsFunctionResponseParts bool) []nativeContent {
	var contents []nativeContent

	// Skip leading system messages.
	startIdx := 0
	for startIdx < len(messages) && messages[startIdx].Role == aikit.RoleSystem {
		startIdx++
	}

	for _, msg := range messages[startIdx:] {
		switch msg.Role {
		case aikit.RoleUser:
			contents = append(contents, nativeContent{
				Role:  "user",
				Parts: encodeUserParts(msg.Content),
			})
		case aikit.RoleAssistant:
			contents = append(contents, nativeContent{
				Role:  "model",
				Parts: encodeAssistantParts(msg.Content),
			})
		case aikit.RoleTool:
			contents = append(contents, nativeContent{
				Role:  "user",
				Parts: encodeToolParts(msg.Content, supportsFunctionResponseParts),
			})
		}
	}

	return contents
}

func usesGemini3Features(modelID string) bool {
	id := strings.ToLower(modelID)
	if slash := strings.LastIndexByte(id, '/'); slash >= 0 {
		id = id[slash+1:]
	}
	if !strings.HasPrefix(id, "gemini-") {
		return false
	}
	if id == "gemini-1" || id == "gemini-2" {
		return false
	}
	legacyPrefix := []string{
		"gemini-1.", "gemini-1-", "gemini-2.", "gemini-2-",
		"gemini-pro", "gemini-robotics-er-1.5",
	}
	for _, prefix := range legacyPrefix {
		if strings.HasPrefix(id, prefix) {
			return false
		}
	}
	return true
}

// encodeUserParts converts user content parts to native parts.
func encodeUserParts(parts []aikit.ContentPart) []nativePart {
	var out []nativePart
	for _, p := range parts {
		switch p.Type {
		case aikit.ContentPartTypeText:
			out = append(out, nativePart{Text: p.Text})
		case aikit.ContentPartTypeFile, aikit.ContentPartTypeImage:
			if strings.HasPrefix(p.MediaType, "image/") || p.MediaType == "image" {
				out = append(out, encodeImagePart(p))
			} else {
				out = append(out, encodeFilePart(p))
			}
		case aikit.ContentPartTypeAudio, aikit.ContentPartTypeDocument, aikit.ContentPartTypeVideo:
			out = append(out, encodeFilePart(p))
		}
	}
	return out
}

// encodeAssistantParts converts assistant content parts to native parts.
func encodeAssistantParts(parts []aikit.ContentPart) []nativePart {
	var out []nativePart
	for _, p := range parts {
		switch p.Type {
		case aikit.ContentPartTypeText:
			out = append(out, nativePart{Text: p.Text})
		case aikit.ContentPartTypeReasoning:
			np := nativePart{Text: p.ReasoningText, Thought: boolPtr(true)}
			if p.ThoughtSignature != "" {
				np.ThoughtSignature = p.ThoughtSignature
			}
			out = append(out, np)
		case aikit.ContentPartTypeToolCall:
			np := nativePart{
				FunctionCall: &nativeFuncCall{
					Name: p.ToolCallName,
					Args: p.ToolCallArgs,
				},
			}
			if p.ThoughtSignature != "" {
				np.ThoughtSignature = p.ThoughtSignature
			}
			out = append(out, np)
		case aikit.ContentPartTypeFile, aikit.ContentPartTypeImage:
			if strings.HasPrefix(p.MediaType, "image/") || p.MediaType == "image" {
				out = append(out, encodeImagePart(p))
			} else {
				out = append(out, encodeFilePart(p))
			}
		case aikit.ContentPartTypeAudio, aikit.ContentPartTypeDocument, aikit.ContentPartTypeVideo:
			out = append(out, encodeFilePart(p))
		}
	}
	return out
}

// encodeToolParts converts tool-result content parts to native functionResponse parts.
func encodeToolParts(parts []aikit.ContentPart, supportsFunctionResponseParts bool) []nativePart {
	var out []nativePart
	for _, p := range parts {
		if p.Type == aikit.ContentPartTypeToolResult {
			response := any(map[string]string{"name": p.ToolResultName, "content": p.ToolResultOutput})
			var responseParts []nativeFunctionResponsePart
			if len(p.ToolResultContent) > 0 {
				items := make([]any, 0, len(p.ToolResultContent))
				for _, item := range p.ToolResultContent {
					switch item.Type {
					case aikit.ToolResultContentTypeText:
						items = append(items, item.Text)
					case aikit.ToolResultContentTypeJSON:
						var value any
						if err := json.Unmarshal(item.JSON, &value); err != nil {
							value = string(item.JSON)
						}
						items = append(items, value)
					case aikit.ToolResultContentTypeImage:
						responseParts = append(responseParts, nativeFunctionResponsePart{
							InlineData: &nativeInlineData{
								MediaType: item.MediaType,
								Data:      base64.StdEncoding.EncodeToString(item.Data),
							},
						})
					}
				}
				content := any(p.ToolResultOutput)
				if len(items) == 1 {
					content = items[0]
				} else if len(items) > 1 {
					content = items
				} else if p.ToolResultOutput == "" && len(responseParts) > 0 {
					content = "Tool executed successfully."
				}
				response = map[string]any{"name": p.ToolResultName, "content": content}
			}
			functionResponse := &nativeFuncResp{
				Name: p.ToolResultName, Response: response,
			}
			if supportsFunctionResponseParts {
				functionResponse.Parts = responseParts
			}
			out = append(out, nativePart{FunctionResponse: functionResponse})
			if !supportsFunctionResponseParts {
				for _, responsePart := range responseParts {
					out = append(out, nativePart{InlineData: responsePart.InlineData})
					kind := "file"
					if responsePart.InlineData != nil && strings.HasPrefix(responsePart.InlineData.MediaType, "image/") {
						kind = "image"
					}
					out = append(out, nativePart{Text: "Tool executed successfully and returned this " + kind + " as a response"})
				}
			}
		}
	}
	return out
}

// encodeImagePart encodes an image ContentPart as inlineData or fileData.
// It handles both inline binary data (from ImageDataPart) and URL references (from ImageURLPart).
func encodeImagePart(p aikit.ContentPart) nativePart {
	if len(p.Data) > 0 {
		return nativePart{
			InlineData: &nativeInlineData{
				MediaType: p.MediaType,
				Data:      base64.StdEncoding.EncodeToString(p.Data),
			},
		}
	}
	return encodeMediaFromURL(p.FileURL, p.MediaType)
}

// encodeMediaFromURL converts a URL (possibly data: URI) to an inlineData or fileData part.
func encodeMediaFromURL(url, mimeType string) nativePart {
	if strings.HasPrefix(url, "data:") {
		mime, data, ok := parseDataURI(url)
		if ok {
			return nativePart{InlineData: &nativeInlineData{MediaType: mime, Data: data}}
		}
	}
	m := mimeType
	if m == "" {
		m = guessMediaTypeFromURL(url)
	}
	return nativePart{FileData: &nativeFileData{MediaType: m, FileUri: url}}
}

// encodeFilePart encodes a file ContentPart as inlineData or fileData.
func encodeFilePart(p aikit.ContentPart) nativePart {
	if len(p.Data) > 0 {
		return nativePart{
			InlineData: &nativeInlineData{
				MediaType: p.MediaType,
				Data:      base64.StdEncoding.EncodeToString(p.Data),
			},
		}
	}
	if strings.HasPrefix(p.FileURL, "data:") {
		mime, data, ok := parseDataURI(p.FileURL)
		if ok {
			return nativePart{InlineData: &nativeInlineData{MediaType: mime, Data: data}}
		}
	}
	m := p.MediaType
	if m == "" {
		m = guessMediaTypeFromURL(p.FileURL)
	}
	return nativePart{FileData: &nativeFileData{MediaType: m, FileUri: p.FileURL}}
}

// buildGenerationConfig constructs the generationConfig from settings, provider options, and output schema.
func buildGenerationConfig(req llm.Request) *nativeGenerationConfig {
	cfg := &nativeGenerationConfig{}
	opts := parseProviderOptions(req.ProviderOptions)

	s := applyCallSettings(cfg, req.Settings)
	p := applyProviderOptions(cfg, opts)
	o := applyOutputSchema(cfg, req.Output)

	if !s && !p && !o {
		return nil
	}
	return cfg
}

// applyCallSettings maps CallSettings fields onto the generation config.
// Returns true if any field was set.
func applyCallSettings(cfg *nativeGenerationConfig, s llm.CallSettings) bool {
	set := false
	if s.MaxTokens > 0 {
		cfg.MaxOutputTokens = &s.MaxTokens
		set = true
	}
	if s.Temperature != nil {
		cfg.Temperature = s.Temperature
		set = true
	}
	if s.TopP != nil {
		cfg.TopP = s.TopP
		set = true
	}
	if s.TopK != nil {
		cfg.TopK = s.TopK
		set = true
	}
	if s.Seed != nil {
		cfg.Seed = s.Seed
		set = true
	}
	if len(s.StopSequences) > 0 {
		cfg.StopSequences = s.StopSequences
		set = true
	}
	return set
}

// applyProviderOptions maps Gemini-specific provider options onto the generation config.
// Returns true if any field was set.
func applyProviderOptions(cfg *nativeGenerationConfig, opts ProviderOptions) bool {
	set := false

	if opts.ThinkingConfig != nil {
		if ntc := buildNativeThinkingConfig(opts.ThinkingConfig); ntc != nil {
			cfg.ThinkingConfig = ntc
			set = true
		}
	}

	if len(opts.ResponseModalities) > 0 {
		cfg.ResponseModalities = opts.ResponseModalities
		set = true
	}

	if opts.ImageConfig != nil {
		if ic := buildNativeImageConfig(opts.ImageConfig); ic != nil {
			cfg.ImageConfig = ic
			set = true
		}
	}

	return set
}

// buildNativeThinkingConfig converts a ThinkingConfig to its native representation.
// Returns nil if no fields are set.
func buildNativeThinkingConfig(tc *ThinkingConfig) *nativeThinkingCfg {
	ntc := &nativeThinkingCfg{}
	empty := true
	if tc.ThinkingBudget != nil {
		ntc.ThinkingBudget = tc.ThinkingBudget
		empty = false
	}
	if tc.IncludeThoughts != nil {
		ntc.IncludeThoughts = tc.IncludeThoughts
		empty = false
	}
	if tc.ThinkingLevel != "" {
		ntc.ThinkingLevel = tc.ThinkingLevel
		empty = false
	}
	if empty {
		return nil
	}
	return ntc
}

// buildNativeImageConfig converts an ImageConfig to its native representation.
// Returns nil if no fields are set.
func buildNativeImageConfig(ic *ImageConfig) *nativeImageConfig {
	nic := &nativeImageConfig{}
	empty := true
	if ic.AspectRatio != "" {
		nic.AspectRatio = ic.AspectRatio
		empty = false
	}
	if ic.ImageSize != "" {
		nic.ImageSize = ic.ImageSize
		empty = false
	}
	if empty {
		return nil
	}
	return nic
}

// applyOutputSchema maps output schema settings onto the generation config.
// Returns true if any field was set.
func applyOutputSchema(cfg *nativeGenerationConfig, output *llm.OutputSchema) bool {
	if output == nil {
		return false
	}
	set := false
	switch output.Type {
	case "json_object", "object", "array":
		cfg.ResponseMimeType = "application/json"
		set = true
	}
	if (output.Type == "object" || output.Type == "array") && output.Schema != nil {
		cfg.ResponseSchema = sanitizeMap(output.Schema)
		set = true
	}
	return set
}

// parseDataURI parses a data: URI and returns the MIME type and base64-encoded data.
// Returns ok=false if the URI is not a valid data URI.
func parseDataURI(uri string) (mimeType, data string, ok bool) {
	// data:[<mediatype>][;base64],<data>
	rest, found := strings.CutPrefix(uri, "data:")
	if !found {
		return "", "", false
	}
	commaIdx := strings.Index(rest, ",")
	if commaIdx < 0 {
		return "", "", false
	}
	meta := rest[:commaIdx]
	payload := rest[commaIdx+1:]

	var mime string
	isBase64 := false
	if strings.HasSuffix(meta, ";base64") {
		isBase64 = true
		mime = strings.TrimSuffix(meta, ";base64")
	} else {
		mime = meta
	}
	if mime == "" {
		mime = "application/octet-stream"
	}

	if isBase64 {
		return mime, payload, true
	}
	// Not base64-encoded: encode the raw data.
	return mime, base64.StdEncoding.EncodeToString([]byte(payload)), true
}

// guessMediaTypeFromURL returns a media type based on common URL file extensions.
func guessMediaTypeFromURL(url string) string {
	lower := strings.ToLower(url)
	switch {
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(lower, ".mp4"):
		return "video/mp4"
	case strings.HasSuffix(lower, ".mp3"):
		return "audio/mpeg"
	case strings.HasSuffix(lower, ".wav"):
		return "audio/wav"
	default:
		return "application/octet-stream"
	}
}

func boolPtr(b bool) *bool {
	return &b
}
