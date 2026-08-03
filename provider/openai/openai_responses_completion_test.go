package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

func TestResponsesCompleteRetainsRawAndMessageID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if _, found := body["stream"]; found {
			t.Fatalf("non-stream request unexpectedly has stream: %#v", body["stream"])
		}
		w.Header().Set("Content-Type", "application/json")
		const payload = `{"id":"resp_1","status":"completed","output":[` +
			`{"type":"reasoning","id":"rs_1","summary":[` +
			`{"type":"summary_text","text":"think"}]},` +
			`{"type":"message","id":"msg_1","role":"assistant","content":[` +
			`{"type":"output_text","text":"answer"}]},` +
			`{"type":"function_call","call_id":"call_1","name":"lookup",` +
			`"arguments":"{\"q\":1}"}],"usage":{"input_tokens":5,"output_tokens":3,` +
			`"total_tokens":8,"input_tokens_details":{"cached_tokens":2},` +
			`"output_tokens_details":{"reasoning_tokens":1}}}`
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	model := NewLanguageModel("gpt-test", Config{BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
	response, err := model.Complete(context.Background(), llm.Request{Messages: []aikit.Message{
		aikit.UserMessage("question"),
	}})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.MessageID != "msg_1" || response.Message.ID != "msg_1" ||
		response.Text != "answer" || response.Reasoning != "think" ||
		response.Usage.InputTokenDetails.CacheReadTokens != 2 {
		t.Fatalf("response = %#v", response)
	}
	raw, ok := llm.RawResponseAs[*ResponsesResponse](response)
	if !ok || raw.ID != "resp_1" || len(raw.Raw) == 0 {
		t.Fatalf("raw = %#v, ok=%v", raw, ok)
	}
}

func TestNormalizeResponsesIncompleteReason(t *testing.T) {
	response, err := normalizeResponsesResponse(&ResponsesResponse{
		Status: "incomplete",
		IncompleteDetails: &ResponsesIncompleteDetail{
			Reason: "max_output_tokens",
		},
		Output: []ResponsesOutputItem{{
			Type: "message", Role: "assistant",
			Content: []ResponsesOutputContent{{Type: "output_text", Text: "partial"}},
		}},
	}, nil)
	if err != nil || response.FinishReason != aikit.FinishReasonLength ||
		response.RawFinishReason != "max_output_tokens" {
		t.Fatalf("response = %#v, err = %v", response, err)
	}
}

func TestResponsesCompleteAppliesConfiguredBodyTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	model := NewLanguageModel("gpt-test", Config{
		BaseURL: server.URL + "/v1", Timeout: 25 * time.Millisecond,
		HTTPClient: server.Client(),
	})
	started := time.Now()
	_, err := model.Complete(context.Background(), llm.Request{
		Messages: []aikit.Message{aikit.UserMessage("question")},
	})
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("body timeout err=%v elapsed=%v", err, time.Since(started))
	}
	var completionErr *llm.CompletionError
	if !errors.As(err, &completionErr) || completionErr.Kind != llm.CompletionErrorKindTransport {
		t.Fatalf("timeout classification = %T %v", err, err)
	}
}

func TestResponsesRejectsAssistantImageReplay(t *testing.T) {
	_, _, err := encodeRequest("gpt-test", llm.Request{Messages: []aikit.Message{{
		Role: aikit.RoleAssistant,
		Content: []aikit.ContentPart{
			aikit.ImageDataPart([]byte("image"), "image/png"),
		},
	}}}, false)
	if err == nil {
		t.Fatal("expected assistant image replay to be rejected")
	}
}
