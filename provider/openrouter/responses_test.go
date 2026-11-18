package openrouter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/provider/openai"
)

func TestResponsesModelUsesResponsesEncoder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/responses" || r.Header.Get("Authorization") != "Bearer example-key" ||
			r.Header.Get("HTTP-Referer") != "https://example.org" ||
			r.Header.Get("X-Title") != "Example" ||
			r.Header.Get("X-Custom") != "yes" {
			t.Errorf("request: %s %v", r.URL.Path, r.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["instructions"] != "instructions" || body["model"] != "vendor/model" || body["input"] == nil ||
			body["messages"] != nil {
			t.Errorf("body: %#v", body)
		}
		w.Header().Set("x-request-id", "req_router")
		_, _ = io.WriteString(
			w,
			`{"id":"resp_router","model":"vendor/model","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}`,
		)
	}))
	defer server.Close()
	config := Config{
		APIKey:  "example-key",
		BaseURL: server.URL + "/api/v1",
		Referer: "https://example.org",
		Title:   "Example",
		Headers: map[string]string{"X-Custom": "yes"},
	}
	for _, model := range []*ResponsesModel{NewResponsesModel("vendor/model", config), NewProvider(config).ResponsesModel("vendor/model")} {
		response, err := model.Complete(
			context.Background(),
			llm.Request{
				Instructions: "instructions",
				Messages: []aikit.Message{
					{Role: aikit.RoleUser, Content: []aikit.ContentPart{aikit.TextPart("hello")}},
				},
				ProviderOptions: map[string]any{"openai": openai.ProviderOptions{InstructionsAsTopLevel: true}},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if response.Text != "ok" || response.ProviderMetadata["openai"].(map[string]any)["requestID"] != "req_router" {
			t.Fatalf("response: %#v", response)
		}
	}
}
