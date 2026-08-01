package openaicompat_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/provider/openaicompat"
)

type externalProvider struct{ endpoint string }

func (provider externalProvider) BaseURL() string { return provider.endpoint }
func (externalProvider) AuthHeader(key string) (string, string) {
	return "X-External-Key", key
}
func (externalProvider) ProviderName() string { return "external" }

func TestExternalProviderUsesOnlyPublicSurface(t *testing.T) {
	var path, auth, modelID string
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		path = request.URL.Path
		auth = request.Header.Get("X-External-Key")
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		modelID, _ = body["model"].(string)
		writer.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(
			writer,
			"data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"+
				"data: [DONE]\n\n",
		)
	}))
	defer server.Close()

	model := openaicompat.NewModel(openaicompat.Config{
		Provider: externalProvider{endpoint: server.URL + "/v1"},
		ModelID:  "external-model",
		APIKey:   "secret",
	})
	events, err := model.Stream(context.Background(), llm.Request{
		Messages: []aikit.Message{{
			Role: aikit.RoleUser,
			Content: []aikit.ContentPart{{
				Type: aikit.ContentPartTypeText,
				Text: "hello",
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	var text string
	for event := range events {
		if event.Type == aikit.StreamEventTextDelta {
			text += event.TextDelta
		}
	}
	if path != "/v1/chat/completions" ||
		auth != "secret" ||
		modelID != "external-model" ||
		text != "ok" {
		t.Fatalf(
			"path=%q auth=%q model=%q text=%q",
			path,
			auth,
			modelID,
			text,
		)
	}
}
