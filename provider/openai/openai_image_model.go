package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/transport"
)

const maxImageInputs = 16

// ImageModel implements [llm.ImageModel] using OpenAI's synchronous Images API.
type ImageModel struct {
	modelID   string
	client    *Client
	clientErr error
}

var _ llm.ImageModel = (*ImageModel)(nil)

// NewImageModel creates an OpenAI-backed image model. Configuration errors are
// deferred until Generate for compatibility with NewLanguageModel.
func NewImageModel(modelID string, config Config) *ImageModel {
	client, err := newClient(config, false)
	if err != nil {
		return &ImageModel{modelID: modelID, clientErr: err}
	}
	return client.ImageModel(modelID)
}

// ModelID returns the OpenAI model identifier.
func (model *ImageModel) ModelID() string { return model.modelID }

// Generate creates or edits images. Requests without source images use JSON
// at images/generations; edits use multipart data at images/edits.
func (model *ImageModel) Generate(ctx context.Context, request llm.GenerateImageRequest) (*llm.GenerateImageResult, error) {
	if model.clientErr != nil {
		return nil, fmt.Errorf("openai-image: configure transport: %w", model.clientErr)
	}
	options, err := parseImageOptions(request.ProviderOptions)
	if err != nil {
		return nil, err
	}
	if err := validateImageRequest(model.modelID, request, options); err != nil {
		return nil, err
	}

	var httpRequest *http.Request
	if len(request.Images) == 0 {
		httpRequest, err = model.buildGenerationRequest(ctx, request, options)
	} else {
		httpRequest, err = model.buildEditRequest(ctx, request, options)
	}
	if err != nil {
		return nil, err
	}
	response, err := model.client.images.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("openai-image: http request: %w", err)
	}
	if response == nil || response.Body == nil {
		return nil, fmt.Errorf("openai-image: HTTP response has no body")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, transport.APIErrorFromResponse(ctx, "openai-image", response)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("openai-image: read response: %w", err)
	}
	return parseImageResponse(body, options.OutputFormat)
}

type imageAPIRequest struct {
	Model             string `json:"model"`
	Prompt            string `json:"prompt"`
	N                 int    `json:"n,omitempty"`
	Size              string `json:"size,omitempty"`
	Quality           string `json:"quality,omitempty"`
	Background        string `json:"background,omitempty"`
	OutputFormat      string `json:"output_format,omitempty"`
	OutputCompression *int   `json:"output_compression,omitempty"`
	Moderation        string `json:"moderation,omitempty"`
	User              string `json:"user,omitempty"`
	InputFidelity     string `json:"input_fidelity,omitempty"`
	ResponseFormat    string `json:"response_format,omitempty"`
}

func (model *ImageModel) apiRequest(request llm.GenerateImageRequest, options ImageOptions) imageAPIRequest {
	apiRequest := imageAPIRequest{
		Model: model.modelID, Prompt: request.Prompt, N: request.N, Size: request.Size,
		Quality: options.Quality, Background: options.Background,
		OutputFormat: options.OutputFormat, OutputCompression: options.OutputCompression,
		Moderation: options.Moderation, User: options.User, InputFidelity: options.InputFidelity,
	}
	if !isGPTImageModel(model.modelID) {
		apiRequest.ResponseFormat = "b64_json"
	}
	return apiRequest
}

func (model *ImageModel) buildGenerationRequest(ctx context.Context, request llm.GenerateImageRequest, options ImageOptions) (*http.Request, error) {
	body, err := json.Marshal(model.apiRequest(request, options))
	if err != nil {
		return nil, fmt.Errorf("openai-image: marshal request: %w", err)
	}
	httpRequest, err := model.client.images.NewRequest(ctx, http.MethodPost, "images/generations", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai-image: build request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	return httpRequest, nil
}

func (model *ImageModel) buildEditRequest(ctx context.Context, request llm.GenerateImageRequest, options ImageOptions) (*http.Request, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	apiRequest := model.apiRequest(request, options)
	fields := []struct{ name, value string }{
		{"model", apiRequest.Model}, {"prompt", apiRequest.Prompt}, {"size", apiRequest.Size},
		{"quality", apiRequest.Quality}, {"background", apiRequest.Background},
		{"output_format", apiRequest.OutputFormat}, {"moderation", apiRequest.Moderation},
		{"user", apiRequest.User}, {"input_fidelity", apiRequest.InputFidelity},
		{"response_format", apiRequest.ResponseFormat},
	}
	if apiRequest.N > 0 {
		fields = append(fields, struct{ name, value string }{"n", fmt.Sprint(apiRequest.N)})
	}
	if apiRequest.OutputCompression != nil {
		fields = append(fields, struct{ name, value string }{"output_compression", fmt.Sprint(*apiRequest.OutputCompression)})
	}
	for _, field := range fields {
		if field.value != "" {
			if err := writer.WriteField(field.name, field.value); err != nil {
				return nil, fmt.Errorf("openai-image: encode multipart field %s: %w", field.name, err)
			}
		}
	}
	for index, image := range request.Images {
		if err := writeImagePart(writer, "image[]", index, image); err != nil {
			return nil, err
		}
	}
	if options.Mask != nil {
		if err := writeImagePart(writer, "mask", 0, *options.Mask); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("openai-image: close multipart request: %w", err)
	}
	httpRequest, err := model.client.images.NewRequest(ctx, http.MethodPost, "images/edits", &body)
	if err != nil {
		return nil, fmt.Errorf("openai-image: build request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", writer.FormDataContentType())
	return httpRequest, nil
}

func writeImagePart(writer *multipart.Writer, field string, index int, image llm.ImageInput) error {
	filename := fmt.Sprintf("%s-%d%s", field, index+1, mediaExtension(image.MediaType))
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, filename))
	header.Set("Content-Type", normalizedMediaType(image.MediaType))
	part, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("openai-image: create %s part: %w", field, err)
	}
	if _, err := part.Write(image.Data); err != nil {
		return fmt.Errorf("openai-image: write %s part: %w", field, err)
	}
	return nil
}

func validateImageRequest(modelID string, request llm.GenerateImageRequest, options ImageOptions) error {
	if strings.TrimSpace(request.Prompt) == "" {
		return fmt.Errorf("openai-image: prompt is required")
	}
	if request.N < 0 || request.N > 10 {
		return fmt.Errorf("openai-image: n must be between 1 and 10 when set")
	}
	if len(request.Images) > maxImageInputs {
		return fmt.Errorf("openai-image: at most %d source images are supported", maxImageInputs)
	}
	if request.AspectRatio != "" {
		return fmt.Errorf("openai-image: aspect ratio is not supported; use size")
	}
	if request.Seed != nil {
		return fmt.Errorf("openai-image: seed is not supported")
	}
	for index, image := range request.Images {
		if len(image.Data) == 0 {
			if image.URL != "" {
				return fmt.Errorf("openai-image: image[%d]: URL-only inputs are not supported; provide inline data", index)
			}
			return fmt.Errorf("openai-image: image[%d]: inline data is required", index)
		}
	}
	if options.Mask != nil {
		if len(request.Images) == 0 {
			return fmt.Errorf("openai-image: mask requires at least one source image")
		}
		if len(options.Mask.Data) == 0 {
			if options.Mask.URL != "" {
				return fmt.Errorf("openai-image: mask: URL-only inputs are not supported; provide inline data")
			}
			return fmt.Errorf("openai-image: mask: inline data is required")
		}
	}
	if err := validateChoice("quality", options.Quality, "auto", "low", "medium", "high"); err != nil {
		return err
	}
	if err := validateChoice("background", options.Background, "auto", "opaque", "transparent"); err != nil {
		return err
	}
	if err := validateChoice("output format", options.OutputFormat, "png", "jpeg", "webp"); err != nil {
		return err
	}
	if err := validateChoice("moderation", options.Moderation, "auto", "low"); err != nil {
		return err
	}
	if err := validateChoice("input fidelity", options.InputFidelity, "low", "high"); err != nil {
		return err
	}
	if isGPTImage2(modelID) {
		if options.InputFidelity != "" {
			return fmt.Errorf("openai-image: gpt-image-2 always uses high input fidelity; omit input fidelity")
		}
		if options.Background == "transparent" {
			return fmt.Errorf("openai-image: gpt-image-2 does not support transparent backgrounds")
		}
	}
	if options.Background == "transparent" && options.OutputFormat != "" && options.OutputFormat != "png" && options.OutputFormat != "webp" {
		return fmt.Errorf("openai-image: transparent background requires png or webp output format")
	}
	if options.OutputCompression != nil {
		if *options.OutputCompression < 0 || *options.OutputCompression > 100 {
			return fmt.Errorf("openai-image: output compression must be between 0 and 100")
		}
		if options.OutputFormat != "jpeg" && options.OutputFormat != "webp" {
			return fmt.Errorf("openai-image: output compression requires jpeg or webp output format")
		}
	}
	return nil
}

func validateChoice(name, value string, allowed ...string) error {
	if value == "" {
		return nil
	}
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("openai-image: invalid %s %q", name, value)
}

func isGPTImageModel(modelID string) bool {
	return strings.HasPrefix(modelID, "gpt-image-") || modelID == "chatgpt-image-latest"
}

func isGPTImage2(modelID string) bool {
	return modelID == "gpt-image-2" || strings.HasPrefix(modelID, "gpt-image-2-")
}

func normalizedMediaType(mediaType string) string {
	if mediaType == "" {
		return "application/octet-stream"
	}
	return mediaType
}

func mediaExtension(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(strings.Split(mediaType, ";")[0])) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		if ext := filepath.Ext(mediaType); ext != "" {
			return ext
		}
		return ".bin"
	}
}

type imageAPIResponse struct {
	OutputFormat string `json:"output_format"`
	Data         []struct {
		B64JSON string `json:"b64_json"`
	} `json:"data"`
	Usage *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

func parseImageResponse(body []byte, requestedFormat string) (*llm.GenerateImageResult, error) {
	var response imageAPIResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("openai-image: unmarshal response: %w", err)
	}
	result := &llm.GenerateImageResult{Raw: append(json.RawMessage(nil), body...)}
	responseFormat := requestedFormat
	if responseFormat == "" {
		responseFormat = response.OutputFormat
	}
	mediaType := outputMediaType(responseFormat)
	for index, item := range response.Data {
		if strings.TrimSpace(item.B64JSON) == "" {
			return nil, fmt.Errorf("openai-image: image[%d] is missing b64_json", index)
		}
		decoded, err := decodeImageBase64(item.B64JSON)
		if err != nil {
			return nil, fmt.Errorf("openai-image: decode image[%d]: %w", index, err)
		}
		result.Images = append(result.Images, llm.GeneratedImage{Data: decoded, MediaType: mediaType})
	}
	if len(result.Images) == 0 {
		return nil, fmt.Errorf("openai-image: response contained no image data")
	}
	if response.Usage != nil {
		var raw map[string]any
		var envelope struct {
			Usage map[string]any `json:"usage"`
		}
		if json.Unmarshal(body, &envelope) == nil {
			raw = envelope.Usage
		}
		result.Usage = &aikit.Usage{InputTokens: response.Usage.InputTokens, OutputTokens: response.Usage.OutputTokens, TotalTokens: response.Usage.TotalTokens, Raw: raw}
	}
	return result, nil
}

func decodeImageBase64(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if comma := strings.IndexByte(value, ','); strings.HasPrefix(value, "data:") && comma >= 0 {
		value = value[comma+1:]
	}
	value = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, value)
	encodings := []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding}
	var lastErr error
	for _, encoding := range encodings {
		decoded, err := encoding.DecodeString(value)
		if err == nil {
			return decoded, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func outputMediaType(format string) string {
	switch format {
	case "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}
