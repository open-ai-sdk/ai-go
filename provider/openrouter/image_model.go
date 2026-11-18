package openrouter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/transport"
)

// ImageOptions configures OpenRouter's synchronous Images API.
type ImageOptions struct {
	Quality      string  `json:"quality"`
	Background   *string `json:"background"`
	OutputFormat string  `json:"outputFormat"`
	// FirstImageOnly selects the first available result instead of all results.
	FirstImageOnly bool `json:"firstImageOnly"`
	// ResponseMaxBytes bounds the API response. Zero uses 64 MiB.
	ResponseMaxBytes int64 `json:"responseMaxBytes"`
}

func (ImageOptions) ProviderName() string { return providerName }

// ImageModel implements the JSON POST /images API, including input_references.
// Unlike the chat image modality this endpoint returns complete image blobs.
type ImageModel struct {
	modelID        string
	client         *transport.Client
	downloadClient transport.Doer
	clientErr      error
}

var (
	_ llm.ImageModel    = (*ImageModel)(nil)
	_ llm.ImageProvider = (*Provider)(nil)
)

// ImageModel constructs a synchronous image model handle.
func (p *Provider) ImageModel(modelID string) llm.ImageModel {
	return NewImageModel(modelID, p.config)
}

// NewImageModel creates an image model. BaseURL is the API root, including /v1
// when required by the upstream endpoint. Configuration errors surface on use.
func NewImageModel(modelID string, cfg Config) *ImageModel {
	base := cfg.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	headers := make(http.Header, len(cfg.Headers)+3)
	headers.Set("Content-Type", "application/json")
	for key, value := range cfg.Headers {
		headers.Set(key, value)
	}
	if cfg.Referer != "" {
		headers.Set("HTTP-Referer", cfg.Referer)
	}
	if cfg.Title != "" {
		headers.Set("X-Title", cfg.Title)
	}
	doer := cfg.HTTPClient
	if doer == nil {
		timeout := cfg.Timeout
		if timeout == 0 {
			timeout = 5 * time.Minute
		}
		doer = &http.Client{Timeout: timeout}
	}
	client, err := transport.NewClient(transport.ClientConfig{
		BaseURL: base, Headers: headers, Provider: providerName,
		HTTPClient: doer,
		Auth:       func(req *http.Request) { req.Header.Set("Authorization", "Bearer "+cfg.APIKey) },
	})
	return &ImageModel{modelID: modelID, client: client, downloadClient: doer, clientErr: err}
}

func (m *ImageModel) ModelID() string { return m.modelID }

func parseImageOptions(options map[string]any) (ImageOptions, error) {
	raw, ok := options[providerName]
	if !ok {
		return ImageOptions{}, nil
	}
	switch value := raw.(type) {
	case ImageOptions:
		return value, nil
	case *ImageOptions:
		if value != nil {
			return *value, nil
		}
	case map[string]any:
		var decoded ImageOptions
		if err := llm.DecodeJSONProviderOptions(providerName, value, &decoded); err != nil {
			return ImageOptions{}, err
		}
		return decoded, nil
	}
	return ImageOptions{}, llm.ProviderOptionTypeError(providerName, raw)
}

// Generate sends inline references as image_url data URLs and URL references
// without downloading them. Generated URL results are downloaded without API
// authentication headers; only the configured HTTP doer is reused.
func (m *ImageModel) Generate(ctx context.Context, request llm.GenerateImageRequest) (*llm.GenerateImageResult, error) {
	if m.clientErr != nil {
		return nil, fmt.Errorf("openrouter-image: configure transport: %w", m.clientErr)
	}
	options, maxBytes, err := validateImageRequest(request)
	if err != nil {
		return nil, err
	}
	encoded, err := m.encodeImageRequest(request, options)
	if err != nil {
		return nil, err
	}
	req, err := m.client.NewRequest(ctx, http.MethodPost, "images", bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter-image: http request: %w", err)
	}
	if resp == nil || resp.Body == nil {
		return nil, fmt.Errorf("openrouter-image: response has no body")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, transport.APIErrorFromResponse(ctx, providerName, resp)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("openrouter-image: response exceeds maximum size of %d bytes", maxBytes)
	}
	var envelope imageResponse
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("openrouter-image: decode response: %w", err)
	}
	result := &llm.GenerateImageResult{Raw: raw, ResponseHeaders: resp.Header.Clone()}
	result.Images, err = m.normalizeImages(ctx, envelope.Data, options)
	if err != nil {
		return nil, err
	}
	result.Usage = imageUsage(envelope.Usage)
	return result, nil
}

func validateImageRequest(request llm.GenerateImageRequest) (ImageOptions, int64, error) {
	options, err := parseImageOptions(request.ProviderOptions)
	if err != nil {
		return ImageOptions{}, 0, err
	}
	if request.AspectRatio != "" || request.Seed != nil {
		return ImageOptions{}, 0, fmt.Errorf("openrouter-image: aspect ratio and seed are unsupported; use size")
	}
	maxBytes := options.ResponseMaxBytes
	if maxBytes == 0 {
		maxBytes = 64 << 20
	}
	if maxBytes < 0 || maxBytes == int64(^uint64(0)>>1) {
		return ImageOptions{}, 0, fmt.Errorf("openrouter-image: response max bytes must be positive")
	}
	return options, maxBytes, nil
}

func (m *ImageModel) encodeImageRequest(request llm.GenerateImageRequest, options ImageOptions) ([]byte, error) {
	body := map[string]any{"model": m.modelID, "prompt": request.Prompt}
	if request.N != 0 {
		body["n"] = request.N
	}
	if request.Size != "" {
		body["size"] = request.Size
	}
	if options.Quality != "" {
		body["quality"] = options.Quality
	}
	if options.Background != nil {
		body["background"] = *options.Background
	}
	if options.OutputFormat != "" {
		body["output_format"] = options.OutputFormat
	}
	if len(request.Images) != 0 {
		references := make([]any, 0, len(request.Images))
		for _, image := range request.Images {
			url := image.URL
			if len(image.Data) != 0 {
				mediaType := image.MediaType
				if mediaType == "" {
					mediaType = "application/octet-stream"
				}
				url = "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(image.Data)
			}
			references = append(
				references,
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}},
			)
		}
		body["input_references"] = references
	}
	return json.Marshal(body)
}

type imageResponseItem struct {
	B64JSON *string `json:"b64_json"`
	URL     string  `json:"url"`
}

type imageResponse struct {
	Data  []imageResponseItem `json:"data"`
	Usage json.RawMessage     `json:"usage"`
}

func (m *ImageModel) normalizeImages(
	ctx context.Context,
	items []imageResponseItem,
	options ImageOptions,
) ([]llm.GeneratedImage, error) {
	var images []llm.GeneratedImage
	mediaType := "image/png"
	if options.OutputFormat == "jpeg" {
		mediaType = "image/jpeg"
	}
	if options.OutputFormat == "webp" {
		mediaType = "image/webp"
	}
	for index, item := range items {
		var err error
		var data []byte
		contentType := mediaType
		switch {
		case item.B64JSON != nil:
			data, err = base64.StdEncoding.DecodeString(strings.TrimSpace(*item.B64JSON))
		case item.URL != "":
			data, contentType, err = llm.DownloadImage(
				ctx,
				item.URL,
				llm.ImageDownloadOptions{HTTPClient: m.downloadClient},
			)
			if contentType == "" {
				contentType = mediaType
			}
		default:
			if options.FirstImageOnly {
				continue
			}
			return nil, fmt.Errorf("openrouter-image: image[%d] has no b64_json or url", index)
		}
		if err != nil {
			return nil, fmt.Errorf("openrouter-image: decode image[%d]: %w", index, err)
		}
		images = append(images, llm.GeneratedImage{Data: data, MediaType: contentType})
		if options.FirstImageOnly {
			break
		}
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("openrouter-image: response contained no image data")
	}
	return images, nil
}

func imageUsage(raw json.RawMessage) *aikit.Usage {
	var usage map[string]any
	if json.Unmarshal(raw, &usage) != nil || usage == nil {
		return nil
	}
	result := &aikit.Usage{Raw: usage}
	if n, ok := usage["prompt_tokens"].(float64); ok {
		result.InputTokens = int(n)
	}
	if n, ok := usage["completion_tokens"].(float64); ok {
		result.OutputTokens = int(n)
	}
	if n, ok := usage["total_tokens"].(float64); ok {
		result.TotalTokens = int(n)
	}
	return result
}
