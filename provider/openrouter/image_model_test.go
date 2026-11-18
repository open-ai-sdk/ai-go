package openrouter_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/provider/openrouter"
)

func TestImageModelJSONReferencesRawUsageAndHeaders(t *testing.T) {
	raw := `{"data":[{}, {"b64_json":"AQID"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13,"cost":0.01,"prompt_tokens_details":{"cached_tokens":2}}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/images" || r.Method != http.MethodPost {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("HTTP-Referer") != "https://example.com" ||
			r.Header.Get("X-Title") != "Example" {
			t.Error(r.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		want := map[string]any{
			"model":         "vendor/image",
			"prompt":        "draw",
			"size":          "1024x1024",
			"quality":       "high",
			"output_format": "png",
			"n":             float64(1),
			"background":    "transparent",
			"input_references": []any{
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,AQID"}},
				map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{"url": "https://example.com/reference.png"},
				},
			},
		}
		if !reflect.DeepEqual(body, want) {
			t.Errorf("body = %#v, want %#v", body, want)
		}
		w.Header().Set("x-request-id", "req-1")
		_, _ = io.WriteString(w, raw)
	}))
	defer server.Close()
	background := "transparent"
	provider := openrouter.NewProvider(
		openrouter.Config{
			APIKey:  "secret",
			BaseURL: server.URL + "/api/v1",
			Referer: "https://example.com",
			Title:   "Example",
		},
	)
	result, err := provider.ImageModel("vendor/image").Generate(context.Background(), llm.GenerateImageRequest{
		Prompt: "draw",
		Size:   "1024x1024",
		N:      1,
		Images: []llm.ImageInput{
			{Data: []byte{1, 2, 3}, MediaType: "image/png"},
			{URL: "https://example.com/reference.png"},
		},
		ProviderOptions: map[string]any{
			"openrouter": openrouter.ImageOptions{
				Quality:        "high",
				Background:     &background,
				OutputFormat:   "png",
				FirstImageOnly: true,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Images) != 1 || string(result.Images[0].Data) != "\x01\x02\x03" ||
		result.Images[0].MediaType != "image/png" ||
		string(result.Raw) != raw {
		t.Fatalf("result = %#v", result)
	}
	if result.Usage == nil || result.Usage.InputTokens != 10 || result.Usage.OutputTokens != 3 ||
		result.Usage.TotalTokens != 13 ||
		result.Usage.Raw["prompt_tokens_details"] == nil {
		t.Fatal(result.Usage)
	}
	if cost, ok := openrouter.Cost(*result.Usage); !ok || cost != 0.01 {
		t.Fatalf("cost = %v %v", cost, ok)
	}
	if result.ResponseHeaders["X-Request-Id"][0] != "req-1" {
		t.Fatal(result.ResponseHeaders)
	}
}

func TestImageModelDownloadsURLResultsWithoutAPIHeaders(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if _, exists := body["input_references"]; exists {
				t.Errorf("unexpected references: %#v", body)
			}
			_, _ = io.WriteString(w, `{"data":[{"url":"`+server.URL+`/blob"}],"usage":42}`)
		case "/blob":
			if r.Header.Get("Authorization") != "" || r.Header.Get("X-Title") != "" {
				t.Error("API headers leaked to image host")
			}
			w.Header().Set("Content-Type", "image/webp")
			_, _ = io.WriteString(w, "image bytes")
		default:
			t.Errorf("path = %s", r.URL.Path)
		}
	}))
	defer server.Close()
	result, err := openrouter.NewImageModel("vendor/image", openrouter.Config{APIKey: "secret", BaseURL: server.URL + "/v1", Title: "Example"}).
		Generate(context.Background(), llm.GenerateImageRequest{Prompt: "draw"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Images) != 1 || string(result.Images[0].Data) != "image bytes" ||
		result.Images[0].MediaType != "image/webp" ||
		!strings.Contains(string(result.Raw), `"usage":42`) {
		t.Fatalf("result = %#v", result)
	}
}

func TestImageModelStructuredHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"code":"slow","message":"try later"}}`)
	}))
	defer server.Close()
	_, err := openrouter.NewImageModel("vendor/image", openrouter.Config{BaseURL: server.URL}).
		Generate(context.Background(), llm.GenerateImageRequest{Prompt: "draw"})
	var apiErr *aikit.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 429 || apiErr.Code != "slow" {
		t.Fatalf("error = %v", err)
	}
}

func TestImageModelRejectsMissingInvalidAndOversizedResults(t *testing.T) {
	for _, test := range []struct {
		name, body string
		limit      int64
	}{
		{"missing", `{"data":[{}]}`, 0},
		{"invalid base64", `{"data":[{"b64_json":"?"}]}`, 0},
		{"empty", `{"data":[]}`, 0},
		{"oversized", `{"data":[{"b64_json":"AQID"}]}`, 8},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, test.body) }),
			)
			defer server.Close()
			_, err := openrouter.NewImageModel("vendor/image", openrouter.Config{BaseURL: server.URL}).
				Generate(context.Background(), llm.GenerateImageRequest{Prompt: "draw", ProviderOptions: map[string]any{"openrouter": openrouter.ImageOptions{ResponseMaxBytes: test.limit}}})
			if err == nil {
				t.Fatal("invalid result accepted")
			}
		})
	}
}
