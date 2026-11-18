package openaicompat_test

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/provider/openaicompat"
)

func TestEmbeddingOptionalWireAndIndexPolicies(t *testing.T) {
	server := newEmbeddingServer(t, func(body map[string]any) (int, string) {
		if _, present := body["encoding_format"]; present {
			t.Errorf("encoding_format must be omitted: %#v", body)
		}
		return http.StatusOK, `{"data":[{"index":1,"embedding":[2]},{"index":0,"embedding":[1]},{"index":1,"embedding":[]}]}`
	})
	model := newModel(server, func(cfg *openaicompat.EmbeddingConfig) {
		cfg.OmitEncodingFormat = true
		cfg.AllowDuplicateIndices = true
		cfg.AllowEmptyVectors = true
		cfg.RequireIndices = true
	})
	got, err := model.EmbedBatch(context.Background(), []string{"a", "b"})
	if err != nil || !reflect.DeepEqual(got, [][]float32{{1}, {}}) {
		t.Fatalf("embeddings = %#v, error = %v", got, err)
	}
}

func TestEmbeddingOptionalPoliciesStillRequireCompleteVectors(t *testing.T) {
	for _, test := range []struct{ name, body, want string }{
		{"missing index", `{"data":[{"embedding":[]}]}`, "has no index"},
		{"null vector", `{"data":[{"index":0,"embedding":null}]}`, "has no vector"},
		{"missing vector", `{"data":[{"index":0}]}`, "has no vector"},
		{"missing row", `{"data":[]}`, "missing index 0"},
		{"out of range", `{"data":[{"index":1,"embedding":[]}]}`, "out of range"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := newEmbeddingServer(t, func(map[string]any) (int, string) { return http.StatusOK, test.body })
			model := newModel(server, func(cfg *openaicompat.EmbeddingConfig) {
				cfg.AllowDuplicateIndices = true
				cfg.AllowEmptyVectors = true
				cfg.RequireIndices = true
			})
			_, err := model.Embed(context.Background(), "a")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestEmbeddingPoliciesAreIndependent(t *testing.T) {
	for _, test := range []struct {
		name      string
		body      string
		configure func(*openaicompat.EmbeddingConfig)
		want      string
	}{
		{"duplicate only still rejects empty", `{"data":[{"index":0,"embedding":[]}]}`, func(c *openaicompat.EmbeddingConfig) { c.AllowDuplicateIndices = true }, "has no vector"},
		{"empty only still rejects duplicates", `{"data":[{"index":0,"embedding":[]},{"index":0,"embedding":[]}]}`, func(c *openaicompat.EmbeddingConfig) { c.AllowEmptyVectors = true }, "repeats index"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := newEmbeddingServer(t, func(map[string]any) (int, string) { return http.StatusOK, test.body })
			_, err := newModel(server, test.configure).EmbedBatch(context.Background(), []string{"a", "b"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
