package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/transport"
)

// defaultEmbeddingTimeout bounds one embeddings request when neither Timeout
// nor HTTPClient is configured.
const defaultEmbeddingTimeout = 120 * time.Second

// EmbeddingConfig configures an [EmbeddingModel].
type EmbeddingConfig struct {
	// Provider supplies the base URL and authentication header. The request
	// goes to {BaseURL}/embeddings, which Ollama serves under /v1.
	Provider Compat
	// ModelID is the model identifier sent to the API.
	ModelID string
	// APIKey is passed to Provider.AuthHeader.
	APIKey string
	// Headers holds additional HTTP headers to include on every request.
	Headers map[string]string
	// Timeout is the HTTP client timeout. Defaults to 120s. It is ignored when
	// HTTPClient is set.
	Timeout time.Duration
	// HTTPClient is an optional custom doer, e.g. for logging or proxying.
	HTTPClient transport.Doer
	// Dimensions asks the model to shorten its vectors. Zero omits the field.
	Dimensions int
	// BatchSize is the largest number of inputs per request; larger batches
	// are split into sequential requests. Zero sends everything in one request.
	BatchSize int
	// OmitEncodingFormat leaves encoding_format out of the request instead of
	// sending the default "float".
	OmitEncodingFormat bool
	// AllowDuplicateIndices accepts repeated indices; the last vector wins.
	AllowDuplicateIndices bool
	// AllowEmptyVectors accepts [] vectors, but not missing or null vectors.
	AllowEmptyVectors bool
	// RequireIndices rejects response rows without an explicit index.
	RequireIndices bool
}

// EmbeddingModel implements [llm.EmbeddingModel] for the OpenAI-compatible
// POST /embeddings endpoint.
type EmbeddingModel struct {
	cfg       EmbeddingConfig
	name      string
	client    *transport.Client
	clientErr error
}

var _ llm.EmbeddingModel = (*EmbeddingModel)(nil)

// EmbeddingResult is a batch of embeddings in input order plus the token usage
// the provider reported, summed across requests.
type EmbeddingResult struct {
	Embeddings [][]float32
	// Usage carries InputTokens and TotalTokens when the provider reported
	// prompt_tokens / total_tokens; otherwise it is the zero value.
	Usage aikit.Usage
}

// NewEmbeddingModel creates an embedding model. Configuration errors (nil
// provider, invalid base URL) are reported by the first Embed call.
func NewEmbeddingModel(cfg EmbeddingConfig) *EmbeddingModel {
	if cfg.Provider == nil {
		return &EmbeddingModel{
			cfg:       cfg,
			name:      providerName(nil),
			clientErr: errors.New("openaicompat: provider is required"),
		}
	}
	name := providerName(cfg.Provider)
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		timeout := cfg.Timeout
		if timeout == 0 {
			timeout = defaultEmbeddingTimeout
		}
		httpClient = &http.Client{Timeout: timeout}
	}
	headers := make(http.Header, len(cfg.Headers)+1)
	headers.Set("Content-Type", "application/json")
	for key, value := range cfg.Headers {
		headers.Set(key, value)
	}
	authName, authValue := cfg.Provider.AuthHeader(cfg.APIKey)
	client, err := transport.NewClient(transport.ClientConfig{
		BaseURL: cfg.Provider.BaseURL(),
		Headers: headers,
		Auth: func(req *http.Request) {
			if authName != "" {
				req.Header.Set(authName, authValue)
			}
		},
		HTTPClient: httpClient,
		Provider:   name,
	})
	return &EmbeddingModel{cfg: cfg, name: name, client: client, clientErr: err}
}

// ModelID returns the configured model identifier.
func (m *EmbeddingModel) ModelID() string { return m.cfg.ModelID }

// Embed returns the embedding of one text.
func (m *EmbeddingModel) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := m.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

// EmbedBatch returns one embedding per text, in input order.
func (m *EmbeddingModel) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	result, err := m.EmbedBatchWithUsage(ctx, texts)
	if err != nil {
		return nil, err
	}
	return result.Embeddings, nil
}

// EmbedBatchWithUsage is [EmbeddingModel.EmbedBatch] that also returns the
// token usage reported by the provider. An empty texts slice makes no request.
func (m *EmbeddingModel) EmbedBatchWithUsage(ctx context.Context, texts []string) (EmbeddingResult, error) {
	if m.clientErr != nil {
		return EmbeddingResult{}, fmt.Errorf("%s: configure transport: %w", m.name, m.clientErr)
	}
	if m.cfg.BatchSize < 0 || m.cfg.Dimensions < 0 {
		return EmbeddingResult{}, fmt.Errorf("%s: BatchSize and Dimensions must not be negative", m.name)
	}
	result := EmbeddingResult{Embeddings: make([][]float32, 0, len(texts))}
	size := m.cfg.BatchSize
	if size == 0 {
		size = len(texts)
	}
	for start := 0; start < len(texts); start += size {
		end := min(start+size, len(texts))
		vectors, usage, err := m.embedChunk(ctx, texts[start:end])
		if err != nil {
			return EmbeddingResult{}, err
		}
		result.Embeddings = append(result.Embeddings, vectors...)
		result.Usage = result.Usage.Add(usage)
	}
	return result, nil
}

type embeddingRequest struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	Dimensions     int      `json:"dimensions,omitempty"`
	EncodingFormat string   `json:"encoding_format,omitempty"`
}

type embeddingResponse struct {
	Data []struct {
		Index     *int      `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Usage *struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

func (m *EmbeddingModel) embedChunk(ctx context.Context, texts []string) ([][]float32, aikit.Usage, error) {
	encodingFormat := "float"
	if m.cfg.OmitEncodingFormat {
		encodingFormat = ""
	}
	body, err := json.Marshal(embeddingRequest{
		Model:          m.cfg.ModelID,
		Input:          texts,
		Dimensions:     m.cfg.Dimensions,
		EncodingFormat: encodingFormat,
	})
	if err != nil {
		return nil, aikit.Usage{}, fmt.Errorf("%s: marshal embedding request: %w", m.name, err)
	}
	req, err := m.client.NewRequest(ctx, http.MethodPost, "embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, aikit.Usage{}, fmt.Errorf("%s: build http request: %w", m.name, err)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, aikit.Usage{}, fmt.Errorf("%s: http request: %w", m.name, err)
	}
	if resp == nil || resp.Body == nil {
		return nil, aikit.Usage{}, fmt.Errorf("%s: http response has no body", m.name)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, aikit.Usage{}, transport.APIErrorFromResponse(ctx, m.name, resp)
	}
	defer resp.Body.Close()

	var decoded embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, aikit.Usage{}, fmt.Errorf("%s: decode embedding response: %w", m.name, err)
	}
	vectors, err := m.orderEmbeddings(decoded, len(texts))
	if err != nil {
		return nil, aikit.Usage{}, fmt.Errorf("%s: %w", m.name, err)
	}
	var usage aikit.Usage
	if decoded.Usage != nil {
		usage.InputTokens = decoded.Usage.PromptTokens
		usage.TotalTokens = decoded.Usage.TotalTokens
	}
	return vectors, usage, nil
}

// orderEmbeddings places each vector at its reported index, preserving strict
// defaults unless the endpoint's explicitly configured policy permits otherwise.
func (m *EmbeddingModel) orderEmbeddings(decoded embeddingResponse, want int) ([][]float32, error) {
	if !m.cfg.AllowDuplicateIndices && len(decoded.Data) != want {
		return nil, fmt.Errorf("embedding response has %d items, want %d", len(decoded.Data), want)
	}
	vectors := make([][]float32, want)
	for position, item := range decoded.Data {
		index := position
		if item.Index != nil {
			index = *item.Index
		} else if m.cfg.RequireIndices {
			return nil, fmt.Errorf("embedding response item %d has no index", position)
		}
		if index < 0 || index >= want {
			return nil, fmt.Errorf("embedding response index %d out of range [0,%d)", index, want)
		}
		if vectors[index] != nil && !m.cfg.AllowDuplicateIndices {
			return nil, fmt.Errorf("embedding response repeats index %d", index)
		}
		if len(item.Embedding) == 0 && (!m.cfg.AllowEmptyVectors || item.Embedding == nil) {
			return nil, fmt.Errorf("embedding response item %d has no vector", index)
		}
		vectors[index] = item.Embedding
	}
	for index, vector := range vectors {
		if vector == nil {
			return nil, fmt.Errorf("embedding response missing index %d", index)
		}
	}
	return vectors, nil
}
