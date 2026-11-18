package openaicompat_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/provider/openaicompat"
	"github.com/open-ai-sdk/ai-go/transport"
)

type embeddingProvider struct {
	endpoint string
	header   string
}

func (p embeddingProvider) BaseURL() string { return p.endpoint }
func (p embeddingProvider) AuthHeader(key string) (string, string) {
	if p.header == "" {
		return "Authorization", "Bearer " + key
	}
	return p.header, key
}
func (embeddingProvider) ProviderName() string { return "embed-test" }

type embeddingServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []map[string]any
	paths    []string
	headers  []http.Header
}

// newEmbeddingServer answers each request with handler(request body). Items
// are returned in the order handler produces them.
func newEmbeddingServer(t *testing.T, handler func(body map[string]any) (int, string)) *embeddingServer {
	t.Helper()
	server := &embeddingServer{}
	server.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("decode body %q: %v", raw, err)
		}
		server.mu.Lock()
		server.requests = append(server.requests, body)
		server.paths = append(server.paths, r.Method+" "+r.URL.Path)
		server.headers = append(server.headers, r.Header.Clone())
		server.mu.Unlock()
		status, payload := handler(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, payload)
	}))
	t.Cleanup(server.Close)
	return server
}

// echoHandler returns, for input "text-N", the vector [N], in reverse order
// with explicit indexes, and usage proportional to the input count.
func echoHandler(body map[string]any) (int, string) {
	inputs, _ := body["input"].([]any)
	items := make([]string, 0, len(inputs))
	for i := len(inputs) - 1; i >= 0; i-- {
		text, _ := inputs[i].(string)
		var n int
		_, _ = fmt.Sscanf(text, "text-%d", &n)
		items = append(items, fmt.Sprintf(`{"object":"embedding","index":%d,"embedding":[%d,0.5]}`, i, n))
	}
	return http.StatusOK, fmt.Sprintf(
		`{"object":"list","data":[%s],"usage":{"prompt_tokens":%d,"total_tokens":%d}}`,
		strings.Join(items, ","), len(inputs)*2, len(inputs)*2,
	)
}

func newModel(server *embeddingServer, mutate func(*openaicompat.EmbeddingConfig)) *openaicompat.EmbeddingModel {
	cfg := openaicompat.EmbeddingConfig{
		Provider: embeddingProvider{endpoint: server.URL + "/v1"},
		ModelID:  "embed-model",
		APIKey:   "secret",
	}
	if mutate != nil {
		mutate(&cfg)
	}
	return openaicompat.NewEmbeddingModel(cfg)
}

func TestEmbeddingModelImplementsInterfaceAndSendsRequest(t *testing.T) {
	server := newEmbeddingServer(t, echoHandler)
	model := newModel(server, func(cfg *openaicompat.EmbeddingConfig) {
		cfg.Headers = map[string]string{"X-Trace": "abc"}
	})
	var _ llm.EmbeddingModel = model
	if model.ModelID() != "embed-model" {
		t.Fatalf("ModelID() = %q", model.ModelID())
	}

	vector, err := model.Embed(context.Background(), "text-7")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if !reflect.DeepEqual(vector, []float32{7, 0.5}) {
		t.Fatalf("vector = %v", vector)
	}
	if len(server.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(server.requests))
	}
	wantBody := map[string]any{
		"model": "embed-model", "input": []any{"text-7"}, "encoding_format": "float",
	}
	if !reflect.DeepEqual(server.requests[0], wantBody) {
		t.Fatalf("body = %v, want %v", server.requests[0], wantBody)
	}
	if server.paths[0] != "POST /v1/embeddings" {
		t.Fatalf("request = %q", server.paths[0])
	}
	header := server.headers[0]
	if header.Get("Authorization") != "Bearer secret" ||
		header.Get("X-Trace") != "abc" ||
		header.Get("Content-Type") != "application/json" {
		t.Fatalf("headers = %v", header)
	}
}

func TestEmbedBatchReordersByIndexAndReportsUsage(t *testing.T) {
	server := newEmbeddingServer(t, echoHandler)
	model := newModel(server, nil)

	result, err := model.EmbedBatchWithUsage(context.Background(), []string{"text-1", "text-2", "text-3"})
	if err != nil {
		t.Fatalf("EmbedBatchWithUsage() error = %v", err)
	}
	want := [][]float32{{1, 0.5}, {2, 0.5}, {3, 0.5}}
	if !reflect.DeepEqual(result.Embeddings, want) {
		t.Fatalf("embeddings = %v, want %v", result.Embeddings, want)
	}
	if result.Usage.InputTokens != 6 || result.Usage.TotalTokens != 6 {
		t.Fatalf("usage = %+v", result.Usage)
	}

	plain, err := model.EmbedBatch(context.Background(), []string{"text-4", "text-5"})
	if err != nil || !reflect.DeepEqual(plain, [][]float32{{4, 0.5}, {5, 0.5}}) {
		t.Fatalf("EmbedBatch() = %v, %v", plain, err)
	}
}

func TestEmbedBatchSplitsByBatchSizeAndSumsUsage(t *testing.T) {
	server := newEmbeddingServer(t, echoHandler)
	model := newModel(server, func(cfg *openaicompat.EmbeddingConfig) { cfg.BatchSize = 2 })

	texts := []string{"text-0", "text-1", "text-2", "text-3", "text-4"}
	result, err := model.EmbedBatchWithUsage(context.Background(), texts)
	if err != nil {
		t.Fatalf("EmbedBatchWithUsage() error = %v", err)
	}
	if len(server.requests) != 3 {
		t.Fatalf("requests = %d, want 3", len(server.requests))
	}
	sizes := make([]int, len(server.requests))
	for i, body := range server.requests {
		sizes[i] = len(body["input"].([]any))
	}
	if !reflect.DeepEqual(sizes, []int{2, 2, 1}) {
		t.Fatalf("batch sizes = %v", sizes)
	}
	for i, vector := range result.Embeddings {
		if vector[0] != float32(i) {
			t.Fatalf("embedding %d = %v", i, vector)
		}
	}
	if result.Usage.InputTokens != 10 || result.Usage.TotalTokens != 10 {
		t.Fatalf("usage = %+v, want sum 10", result.Usage)
	}
}

func TestEmbedBatchWithoutBatchSizeSendsOneRequest(t *testing.T) {
	server := newEmbeddingServer(t, echoHandler)
	model := newModel(server, nil)
	texts := make([]string, 25)
	for i := range texts {
		texts[i] = fmt.Sprintf("text-%d", i)
	}
	if _, err := model.EmbedBatch(context.Background(), texts); err != nil {
		t.Fatal(err)
	}
	if len(server.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(server.requests))
	}
}

func TestEmbedBatchEmptyMakesNoRequest(t *testing.T) {
	server := newEmbeddingServer(t, echoHandler)
	model := newModel(server, nil)
	got, err := model.EmbedBatch(context.Background(), nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("EmbedBatch(nil) = %v, %v", got, err)
	}
	if len(server.requests) != 0 {
		t.Fatalf("requests = %d, want 0", len(server.requests))
	}
}

func TestEmbeddingDimensionsSentOnlyWhenSet(t *testing.T) {
	server := newEmbeddingServer(t, echoHandler)
	model := newModel(server, func(cfg *openaicompat.EmbeddingConfig) { cfg.Dimensions = 256 })
	if _, err := model.Embed(context.Background(), "text-1"); err != nil {
		t.Fatal(err)
	}
	if got := server.requests[0]["dimensions"]; got != float64(256) {
		t.Fatalf("dimensions = %v, want 256", got)
	}
}

func TestEmbeddingCustomAuthHeaderAndDoer(t *testing.T) {
	var seen *http.Request
	doer := transport.DoerFunc(func(req *http.Request) (*http.Response, error) {
		seen = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"embedding":[1,2]}]}`)),
		}, nil
	})
	model := openaicompat.NewEmbeddingModel(openaicompat.EmbeddingConfig{
		Provider:   embeddingProvider{endpoint: "http://embeddings.invalid/v1/", header: "X-Api-Key"},
		ModelID:    "m",
		APIKey:     "k1",
		HTTPClient: doer,
	})
	vector, err := model.Embed(context.Background(), "x")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	// An item without an index keeps its position, and usage may be absent.
	if !reflect.DeepEqual(vector, []float32{1, 2}) {
		t.Fatalf("vector = %v", vector)
	}
	if seen.URL.String() != "http://embeddings.invalid/v1/embeddings" {
		t.Fatalf("url = %s", seen.URL)
	}
	if seen.Header.Get("X-Api-Key") != "k1" || seen.Header.Get("Authorization") != "" {
		t.Fatalf("headers = %v", seen.Header)
	}
}

func TestEmbeddingAPIErrorIsStructured(t *testing.T) {
	server := newEmbeddingServer(t, func(map[string]any) (int, string) {
		return http.StatusTooManyRequests, `{"error":{"message":"slow down","code":"rate_limited"}}`
	})
	model := newModel(server, nil)
	_, err := model.Embed(context.Background(), "x")
	var apiErr *aikit.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *aikit.APIError", err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests || apiErr.Message != "slow down" ||
		apiErr.Code != "rate_limited" || apiErr.Provider != "embed-test" {
		t.Fatalf("apiErr = %+v", apiErr)
	}
}

func TestEmbeddingMalformedResponses(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{"not json", `nope`, "decode embedding response"},
		{"too few items", `{"data":[]}`, "has 0 items, want 2"},
		{"index out of range", `{"data":[{"index":0,"embedding":[1]},{"index":5,"embedding":[1]}]}`, "out of range"},
		{"negative index", `{"data":[{"index":0,"embedding":[1]},{"index":-1,"embedding":[1]}]}`, "out of range"},
		{"duplicate index", `{"data":[{"index":1,"embedding":[1]},{"index":1,"embedding":[1]}]}`, "repeats index 1"},
		{"empty vector", `{"data":[{"index":0,"embedding":[1]},{"index":1,"embedding":[]}]}`, "has no vector"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newEmbeddingServer(t, func(map[string]any) (int, string) { return http.StatusOK, test.payload })
			_, err := newModel(server, nil).EmbedBatch(context.Background(), []string{"a", "b"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestEmbeddingConfigurationErrors(t *testing.T) {
	tests := []struct {
		name string
		cfg  openaicompat.EmbeddingConfig
		want string
	}{
		{"nil provider", openaicompat.EmbeddingConfig{ModelID: "m"}, "provider is required"},
		{"relative base url", openaicompat.EmbeddingConfig{
			Provider: embeddingProvider{endpoint: "/v1"}, ModelID: "m",
		}, "base URL must be absolute"},
		{"negative batch size", openaicompat.EmbeddingConfig{
			Provider: embeddingProvider{endpoint: "http://x.invalid/v1"}, ModelID: "m", BatchSize: -1,
		}, "must not be negative"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := openaicompat.NewEmbeddingModel(test.cfg).Embed(context.Background(), "x")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestEmbeddingTransportFailureAndCancellation(t *testing.T) {
	model := openaicompat.NewEmbeddingModel(openaicompat.EmbeddingConfig{
		Provider: embeddingProvider{endpoint: "http://embeddings.invalid/v1"},
		ModelID:  "m",
		HTTPClient: transport.DoerFunc(func(req *http.Request) (*http.Response, error) {
			return nil, req.Context().Err()
		}),
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := model.Embed(ctx, "x"); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestEmbeddingWorksWithLLMEmbedHelpers(t *testing.T) {
	server := newEmbeddingServer(t, echoHandler)
	model := newModel(server, nil)
	one, err := llm.Embed(context.Background(), llm.EmbedRequest{Model: model, Text: "text-3"})
	if err != nil || one.ModelID != "embed-model" || !reflect.DeepEqual(one.Embedding, []float32{3, 0.5}) {
		t.Fatalf("Embed() = %+v, %v", one, err)
	}
	many, err := llm.EmbedMany(
		context.Background(),
		llm.EmbedManyRequest{Model: model, Texts: []string{"text-1", "text-2"}},
	)
	if err != nil || len(many.Embeddings) != 2 {
		t.Fatalf("EmbedMany() = %+v, %v", many, err)
	}
}
