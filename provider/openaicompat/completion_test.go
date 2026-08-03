package openaicompat_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/provider/openaicompat"
)

func TestCompleteRetainsNativeResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if stream, ok := body["stream"].(bool); !ok || stream {
			t.Fatalf("stream = %#v, want false", body["stream"])
		}
		w.Header().Set("Content-Type", "application/json")
		const payload = `{"id":"chatcmpl_1","choices":[{"index":0,"message":` +
			`{"role":"assistant","content":"hello","tool_calls":[` +
			`{"id":"call_1","type":"function","function":{"name":"lookup",` +
			`"arguments":"{\"q\":\"go\"}"}}]},"finish_reason":"tool_calls"}],` +
			`"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7,` +
			`"completion_tokens_details":{"reasoning_tokens":1}}}`
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	model := openaicompat.NewModel(openaicompat.Config{
		Provider: externalProvider{endpoint: server.URL + "/v1"}, ModelID: "test",
	})
	response, err := model.Complete(context.Background(), llm.Request{Messages: []aikit.Message{
		aikit.UserMessage("hello"),
	}})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Text != "hello" || response.FinishReason != aikit.FinishReasonToolCalls ||
		response.Usage.OutputTokenDetails.ReasoningTokens != 1 {
		t.Fatalf("response = %#v", response)
	}
	raw, ok := llm.RawResponseAs[*openaicompat.ChatCompletionResponse](response)
	if !ok || raw.ID != "chatcmpl_1" || len(raw.Raw) == 0 {
		t.Fatalf("raw = %#v, ok=%v", raw, ok)
	}
}

func TestCompleteAllowsEmptyLengthFinishedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(
			[]byte(
				`{"id":"chatcmpl_length","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"length"}],"usage":{"prompt_tokens":4,"completion_tokens":8,"total_tokens":12}}`,
			),
		)
	}))
	defer server.Close()

	model := openaicompat.NewModel(openaicompat.Config{
		Provider: externalProvider{endpoint: server.URL + "/v1"}, ModelID: "test",
	})
	response, err := model.Complete(context.Background(), llm.Request{
		Messages: []aikit.Message{aikit.UserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.FinishReason != aikit.FinishReasonLength || response.Text != "" ||
		len(response.Message.Content) != 0 || response.Usage.TotalTokens != 12 {
		t.Fatalf("response = %#v", response)
	}
	if raw, ok := llm.RawResponseAs[*openaicompat.ChatCompletionResponse](
		response,
	); !ok ||
		raw.ID != "chatcmpl_length" {
		t.Fatalf("raw response = %#v, ok = %v", raw, ok)
	}
}
