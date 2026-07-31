package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// makeFakeServer creates an httptest server that returns a batchEmbedContents response
// with one embedding per request entry. Each embedding is a fixed [0.1, 0.2, 0.3] vector.
func makeFakeServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req geminiBatchEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		resp := geminiBatchEmbedResponse{}
		for range req.Requests {
			resp.Embeddings = append(resp.Embeddings, struct {
				Values []float32 `json:"values"`
			}{Values: []float32{0.1, 0.2, 0.3}})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// newTestEmbeddingModel creates an EmbeddingModel pointed at a fake server URL.
func newTestEmbeddingModel(t *testing.T, server *httptest.Server) *EmbeddingModel {
	t.Helper()
	return NewEmbeddingModel("text-embedding-004", Config{
		APIKey: "fake-key", BaseURL: server.URL,
	})
}

func TestEmbeddingModel_ModelID(t *testing.T) {
	m := &EmbeddingModel{modelID: "text-embedding-004"}
	if m.ModelID() != "text-embedding-004" {
		t.Errorf("expected ModelID=%q, got %q", "text-embedding-004", m.ModelID())
	}
}

func TestEmbeddingModel_Embed(t *testing.T) {
	srv := makeFakeServer(t)
	defer srv.Close()

	m := newTestEmbeddingModel(t, srv)
	vec, err := m.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vec) != 3 {
		t.Errorf("expected 3 dimensions, got %d", len(vec))
	}
}

func TestEmbeddingModel_EmbedBatch(t *testing.T) {
	srv := makeFakeServer(t)
	defer srv.Close()

	texts := []string{"chunk one", "chunk two", "chunk three"}
	m := newTestEmbeddingModel(t, srv)

	vecs, err := m.EmbedBatch(context.Background(), texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vecs) != len(texts) {
		t.Errorf("expected %d embeddings, got %d", len(texts), len(vecs))
	}
	for i, v := range vecs {
		if len(v) == 0 {
			t.Errorf("embedding[%d] is empty", i)
		}
	}
}

func TestEmbeddingModel_EmbedBatch_Empty(t *testing.T) {
	m := NewEmbeddingModel("text-embedding-004", Config{APIKey: "k"})
	vecs, err := m.EmbedBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vecs != nil {
		t.Error("expected nil result for empty input")
	}
}

func TestEmbeddingModel_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	m := newTestEmbeddingModel(t, srv)
	_, err := m.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got: %v", err)
	}
}

func TestBuildBatchEmbedRequest(t *testing.T) {
	texts := []string{"a", "b"}
	data, err := buildBatchEmbedRequest("text-embedding-004", texts, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var req geminiBatchEmbedRequest
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(req.Requests) != 2 {
		t.Errorf("expected 2 requests, got %d", len(req.Requests))
	}
	for i, r := range req.Requests {
		if r.Model != "models/text-embedding-004" {
			t.Errorf("request[%d]: expected model prefix, got %q", i, r.Model)
		}
		if len(r.Content.Parts) != 1 || r.Content.Parts[0].Text != texts[i] {
			t.Errorf("request[%d]: expected text %q", i, texts[i])
		}
	}
}

func TestParseBatchEmbedResponse_CountMismatch(t *testing.T) {
	resp := geminiBatchEmbedResponse{
		Embeddings: []struct {
			Values []float32 `json:"values"`
		}{
			{Values: []float32{0.1}},
		},
	}
	data, _ := json.Marshal(resp)
	_, err := parseBatchEmbedResponse(data, 3)
	if err == nil {
		t.Fatal("expected error for count mismatch")
	}
}
