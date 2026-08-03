package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

func TestCompleteRetainsRawMessageAndCacheUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if stream, ok := body["stream"].(bool); !ok || stream {
			t.Fatalf("stream = %#v, want false", body["stream"])
		}
		w.Header().Set("Content-Type", "application/json")
		const payload = `{"id":"msg_1","role":"assistant","content":` +
			`[{"type":"thinking","thinking":"think","signature":"sig"},` +
			`{"type":"text","text":"answer"},` +
			`{"type":"tool_use","id":"call_1","name":"lookup","input":{"q":1}}],` +
			`"stop_reason":"tool_use","usage":{"input_tokens":3,"output_tokens":4,` +
			`"cache_creation_input_tokens":2,"cache_read_input_tokens":1}}`
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	model := NewLanguageModel("claude-test", Config{BaseURL: server.URL})
	response, err := model.Complete(context.Background(), llm.Request{Messages: []aikit.Message{
		aikit.UserMessage("question"),
	}})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.MessageID != "msg_1" || response.Text != "answer" || response.Reasoning != "think" ||
		response.Usage.InputTokens != 6 || response.Usage.TotalTokens != 0 {
		t.Fatalf("response = %#v", response)
	}
	raw, ok := llm.RawResponseAs[*MessageResponse](response)
	if !ok || raw.ID != "msg_1" || len(raw.Raw) == 0 {
		t.Fatalf("raw = %#v, ok=%v", raw, ok)
	}
}
