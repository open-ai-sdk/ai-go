package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/transport"
)

// ImageModel implements aikit.ImageModel using the native Gemini API's
// non-streaming :generateContent endpoint. Unlike NativeLanguageModel,
// this uses the synchronous endpoint since images are returned as
// complete base64 blobs.
//
// Use NewImageModel to construct an instance.
type ImageModel struct {
	modelID   string
	client    *transport.Client
	clientErr error
}

// NewImageModel creates a Gemini-backed aikit.ImageModel that generates images
// using the native Gemini API.
//
// Supported models: gemini-2.5-flash-image, gemini-3-pro-image-preview,
// gemini-3.1-flash-image-preview.
func NewImageModel(modelID string, cfg Config) *ImageModel {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = nativeBaseURL
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	client, clientErr := transport.NewClient(transport.ClientConfig{
		BaseURL: baseURL,
		Headers: http.Header{"Content-Type": []string{"application/json"}},
		Auth: func(request *http.Request) {
			request.Header.Set("x-goog-api-key", cfg.APIKey)
		},
		HTTPClient: httpClient,
		Provider:   "gemini-image",
	})
	return &ImageModel{
		modelID:   modelID,
		client:    client,
		clientErr: clientErr,
	}
}

// ModelID returns the Gemini model identifier.
func (m *ImageModel) ModelID() string { return m.modelID }

// Generate sends a request to the Gemini API and returns generated images.
func (m *ImageModel) Generate(ctx context.Context, req llm.GenerateImageRequest) (*llm.GenerateImageResult, error) {
	nr := m.buildRequest(req)

	body, err := json.Marshal(nr)
	if err != nil {
		return nil, fmt.Errorf("gemini-image: marshal request: %w", err)
	}

	if m.clientErr != nil {
		return nil, fmt.Errorf("gemini-image: configure transport: %w", m.clientErr)
	}
	target := fmt.Sprintf("models/%s:generateContent", m.modelID)
	httpReq, err := m.client.NewRequest(
		ctx,
		http.MethodPost,
		target,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("gemini-image: build http request: %w", err)
	}

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini-image: http request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, transport.APIErrorFromResponse(ctx, "gemini-image", resp)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gemini-image: read response: %w", err)
	}

	return m.parseResponse(respBody)
}

// imageGenerateResponse is the JSON response from the non-streaming generateContent endpoint.
type imageGenerateResponse struct {
	Candidates    []imageCandidate     `json:"candidates"`
	UsageMetadata *nativeUsageMetadata `json:"usageMetadata"`
}

type imageCandidate struct {
	Content *imageCandidateContent `json:"content"`
}

type imageCandidateContent struct {
	Parts []imageResponsePart `json:"parts"`
	Role  string              `json:"role"`
}

type imageResponsePart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *nativeInlineData `json:"inlineData,omitempty"`
}

func (m *ImageModel) buildRequest(req llm.GenerateImageRequest) nativeRequest {
	nr := nativeRequest{}

	var parts []nativePart

	if req.Prompt != "" {
		parts = append(parts, nativePart{Text: req.Prompt})
	}

	for _, img := range req.Images {
		if len(img.Data) > 0 {
			parts = append(parts, nativePart{
				InlineData: &nativeInlineData{
					MediaType: img.MediaType,
					Data:      base64.StdEncoding.EncodeToString(img.Data),
				},
			})
		} else if img.URL != "" {
			parts = append(parts, encodeMediaFromURL(img.URL, img.MediaType))
		}
	}

	nr.Contents = []nativeContent{
		{Role: "user", Parts: parts},
	}

	genCfg := &nativeGenerationConfig{
		ResponseModalities: []string{"IMAGE"},
	}

	if req.AspectRatio != "" || req.Size != "" {
		ic := &nativeImageConfig{}
		if req.AspectRatio != "" {
			ic.AspectRatio = req.AspectRatio
		}
		if req.Size != "" {
			ic.ImageSize = req.Size
		}
		genCfg.ImageConfig = ic
	}

	if req.Seed != nil {
		genCfg.Seed = req.Seed
	}

	nr.GenerationConfig = genCfg

	return nr
}

func (m *ImageModel) parseResponse(data []byte) (*llm.GenerateImageResult, error) {
	var resp imageGenerateResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("gemini-image: unmarshal response: %w", err)
	}

	result := &llm.GenerateImageResult{}

	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		for _, part := range resp.Candidates[0].Content.Parts {
			if part.InlineData != nil {
				decoded, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
				if err != nil {
					continue
				}
				result.Images = append(result.Images, llm.GeneratedImage{
					Data:      decoded,
					MediaType: part.InlineData.MediaType,
				})
			}
		}
	}

	if resp.UsageMetadata != nil {
		result.Usage = &aikit.Usage{
			InputTokens:  resp.UsageMetadata.PromptTokenCount,
			OutputTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:  resp.UsageMetadata.TotalTokenCount,
		}
	}

	return result, nil
}
