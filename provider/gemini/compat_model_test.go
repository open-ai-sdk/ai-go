package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

func TestCompatModelThinkingOptionsReachWire(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(writer, "data: [DONE]\n\n")
	}))
	defer server.Close()

	budget, thoughts := 1024, true
	model := NewLanguageModel("gemini-test", Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	events, err := model.Stream(context.Background(), llm.Request{
		Messages: []aikit.Message{{
			Role: aikit.RoleUser,
			Content: []aikit.ContentPart{{
				Type: aikit.ContentPartTypeText,
				Text: "think",
			}},
		}},
		ProviderOptions: map[string]any{"gemini": ProviderOptions{
			ThinkingConfig: &ThinkingConfig{
				ThinkingBudget:  &budget,
				IncludeThoughts: &thoughts,
				ThinkingLevel:   "high",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	for range events {
	}

	extra := received["extra_body"].(map[string]any)
	google := extra["google"].(map[string]any)
	thinking := google["thinking_config"].(map[string]any)
	if thinking["thinking_budget"] != float64(budget) ||
		thinking["include_thoughts"] != true ||
		thinking["thinking_level"] != "high" {
		t.Fatalf("thinking_config = %#v", thinking)
	}
}

func TestCompatModelRejectsNativeOnlyOptions(t *testing.T) {
	model := NewLanguageModel("gemini-test", Config{
		APIKey:  "test-key",
		BaseURL: "https://example.com",
	})
	for _, options := range []ProviderOptions{
		{EnableGoogleSearch: true},
		{ResponseModalities: []string{"IMAGE"}},
		{ImageConfig: &ImageConfig{AspectRatio: "1:1"}},
	} {
		_, err := model.Stream(context.Background(), llm.Request{
			ProviderOptions: map[string]any{"gemini": options},
		})
		if err == nil {
			t.Fatalf("options %#v were silently accepted", options)
		}
	}
}

func TestNativeSchemaSupportDependsOnGeminiModelGeneration(t *testing.T) {
	tests := []struct {
		modelID string
		want    llm.NativeSchemaSupport
	}{
		{modelID: "gemini-2.5-flash", want: llm.NativeSchemaSuppressesTools},
		{modelID: "gemini-3-pro-preview", want: llm.NativeSchemaFull},
		{modelID: "gemini-30-pro", want: llm.NativeSchemaSuppressesTools},
	}
	for _, test := range tests {
		t.Run(test.modelID, func(t *testing.T) {
			compat := NewLanguageModel(test.modelID, Config{APIKey: "test"})
			native := NewNativeLanguageModel(test.modelID, Config{APIKey: "test"})
			if got := compat.NativeSchemaSupport(); got != test.want {
				t.Fatalf("compat NativeSchemaSupport() = %v, want %v", got, test.want)
			}
			if got := native.NativeSchemaSupport(); got != test.want {
				t.Fatalf("native NativeSchemaSupport() = %v, want %v", got, test.want)
			}
		})
	}
}
