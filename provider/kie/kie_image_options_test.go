package kie

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/open-ai-sdk/ai-go/llm"
)

func TestImageOptionsStrictJSONFallback(t *testing.T) {
	options, err := extractOptions(llm.GenerateImageRequest{
		ProviderOptions: map[string]any{
			"kie": map[string]any{
				"resolution":   "2K",
				"outputFormat": "png",
				"callBackUrl":  "https://example.com/callback",
				"extra":        map[string]any{"seed": float64(7)},
			},
		},
	})
	if err != nil {
		t.Fatalf("extractOptions() error = %v", err)
	}
	if options.Resolution != "2K" ||
		options.OutputFormat != "png" ||
		options.CallBackURL != "https://example.com/callback" {
		t.Fatalf("options = %#v", options)
	}
	if options.Extra["seed"] != float64(7) {
		t.Fatalf("Extra = %#v", options.Extra)
	}
}

func TestImageOptionsAcceptTypedPointer(t *testing.T) {
	options, err := extractOptions(llm.GenerateImageRequest{
		ProviderOptions: map[string]any{
			"kie": &ImageOptions{Resolution: "4K"},
		},
	})
	if err != nil || options.Resolution != "4K" {
		t.Fatalf("options = %#v, error = %v", options, err)
	}
}

func TestInvalidImageOptionsReturnBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	t.Cleanup(server.Close)

	model := newImageModel(ModelNanoBanana2, Config{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}.resolved())
	tests := []any{
		"wrong",
		map[string]any{"resolution": true},
		map[string]any{"unknown": true},
		(*ImageOptions)(nil),
	}
	for _, value := range tests {
		_, err := model.Generate(context.Background(), llm.GenerateImageRequest{
			Prompt:          "test",
			ProviderOptions: map[string]any{"kie": value},
		})
		var optionErr *llm.ProviderOptionsError
		if !errors.As(err, &optionErr) {
			t.Fatalf("value %#v error = %v, want *ProviderOptionsError", value, err)
		}
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("HTTP calls = %d, want 0", got)
	}
}
