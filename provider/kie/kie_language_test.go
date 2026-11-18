package kie_test

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
	"github.com/open-ai-sdk/ai-go/provider/kie"
	"github.com/open-ai-sdk/ai-go/transport"
)

const languageJSONReply = `{"id":"response-1","model":"configured-model","status":"completed","extension":{"retained":true},"output":[{"type":"message","id":"message-1","role":"assistant","content":[{"type":"output_text","text":"answer","annotations":[{"type":"url_citation","url":"https://example.test/source"}]}]}],"usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120,"input_tokens_details":{"cached_tokens":30},"output_tokens_details":{"reasoning_tokens":5},"extension":7}}`

func languageRequest() llm.Request {
	return llm.Request{
		Instructions: "Use the supplied context.",
		Messages: []aikit.Message{{Role: aikit.RoleUser, Content: []aikit.ContentPart{
			aikit.TextPart("Describe the image."), aikit.ImageURLPart("https://example.test/input.png"),
		}}},
		Output: &llm.OutputSchema{Name: "answer_record", Type: "object", Schema: map[string]any{
			"type": "object", "properties": map[string]any{"answer": map[string]any{"type": "string"}},
			"required": []any{"answer"}, "additionalProperties": false,
		}},
		ProviderOptions: map[string]any{"kie": kie.LanguageOptions{
			ReasoningEffort: "medium", NativeTools: []map[string]any{{"type": "web_search"}},
		}},
	}
}

func languageTestProvider(t *testing.T, path, expected, reply, contentType string) *kie.Provider {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != path {
			t.Errorf("request = %s %s, want POST %s", r.Method, r.URL.Path, path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer example-key" {
			t.Errorf("authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content type = %q", got)
		}
		var got, want map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if err := json.Unmarshal([]byte(expected), &want); err != nil {
			t.Errorf("decode expectation: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("request body = %#v, want %#v", got, want)
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(server.Close)
	return kie.NewProvider(
		"example-key",
		kie.WithConfig(kie.Config{BaseURL: server.URL + "/gateway", HTTPClient: server.Client()}),
	)
}

func TestLanguageResponsesJSON(t *testing.T) {
	provider := languageTestProvider(t, "/gateway/custom/responses", `{
		"model":"configured-model",
		"input":[
			{"role":"system","content":[{"type":"input_text","text":"Use the supplied context."}]},
			{"role":"user","content":[{"type":"input_text","text":"Describe the image."},{"type":"input_image","image_url":"https://example.test/input.png"}]}
		],
		"reasoning":{"effort":"medium"},"tools":[{"type":"web_search"}],
		"text":{"format":{"type":"json_schema","name":"answer_record","strict":true,"schema":{
			"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false
		}}}
	}`, languageJSONReply, "application/json")
	response, err := provider.Responses("configured-model", kie.WithLanguagePath("custom/responses")).
		Complete(context.Background(), languageRequest())
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "answer" || response.MessageID != "message-1" ||
		response.FinishReason != aikit.FinishReasonStop {
		t.Fatalf("completion = %#v", response)
	}
	if response.Usage.InputTokens != 100 || response.Usage.OutputTokens != 20 || response.Usage.TotalTokens != 120 ||
		response.Usage.InputTokenDetails.CacheReadTokens != 30 || response.Usage.InputTokenDetails.NoCacheTokens != 70 ||
		response.Usage.OutputTokenDetails.ReasoningTokens != 5 || response.Usage.OutputTokenDetails.TextTokens != 15 {
		t.Fatalf("usage = %#v", response.Usage)
	}
	raw, ok := llm.RawResponseAs[map[string]any](response)
	if !ok || raw["extension"].(map[string]any)["retained"] != true || response.Usage.Raw["extension"] != float64(7) {
		t.Fatalf("provider fields were not retained: %#v", response)
	}
}

func TestLanguageResponsesNonstreamSSE(t *testing.T) {
	sse := ": keepalive\r\n\r\nevent: response.created\r\ndata: {\"type\":\"response.created\",\"extension\":1}\r\n\r\n" +
		"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"answer\"}\n\n" +
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":" + languageJSONReply + "}\n\n" +
		"data: [DONE]\n\n"
	provider := languageTestProvider(
		t,
		"/gateway/responses",
		`{"model":"configured-model","input":[{"role":"user","content":[{"type":"input_text","text":"Question"}]}]}`,
		sse,
		"text/event-stream",
	)
	response, err := provider.Responses("configured-model").
		Complete(context.Background(), llm.NewRequest("Question").Build())
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "answer" || response.Usage.InputTokens != 100 || response.Usage.OutputTokens != 20 {
		t.Fatalf("completion = %#v", response)
	}
	raw, ok := llm.RawResponseAs[map[string]any](response)
	if !ok {
		t.Fatalf("raw response type = %T", response.RawResponse)
	}
	events, ok := raw["events"].([]any)
	if !ok || len(events) != 3 || events[0].(map[string]any)["extension"] != float64(1) {
		t.Fatalf("raw events = %#v", raw)
	}
	if events[2].(map[string]any)["response"].(map[string]any)["extension"].(map[string]any)["retained"] != true {
		t.Fatalf("terminal provider response lost fields: %#v", raw)
	}
}

func TestLanguageChatNamedSchemaImagesAndSearch(t *testing.T) {
	provider := languageTestProvider(t, "/gateway/selected-model/v1/chat/completions", `{
		"messages":[
			{"role":"system","content":[{"type":"text","text":"Use the supplied context."}]},
			{"role":"user","content":[{"type":"text","text":"Describe the image."},{"type":"image_url","image_url":{"url":"https://example.test/input.png"}}]}
		],
		"stream":false,"include_thoughts":false,"reasoning_effort":"high",
		"tools":[{"type":"function","function":{"name":"googleSearch"}}],
		"response_format":{"type":"json_schema","json_schema":{"name":"custom_name","strict":true,"schema":{
			"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false
		}}}
	}`, `{"id":"chat-1","extension":true,"choices":[{"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13,"prompt_tokens_details":{"cached_tokens":2},"completion_tokens_details":{"reasoning_tokens":1}}}`, "application/json")
	req := languageRequest()
	req.ProviderOptions["kie"] = kie.LanguageOptions{
		ReasoningEffort: "high", SchemaName: "custom_name",
		NativeTools: []map[string]any{{"type": "function", "function": map[string]any{"name": "googleSearch"}}},
	}
	model := provider.Chat("selected-model", "selected-model/v1/chat/completions")
	response, err := llm.Complete(context.Background(), model, req)
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "answer" || response.Usage.InputTokens != 9 || response.Usage.OutputTokens != 4 ||
		response.Usage.InputTokenDetails.CacheReadTokens != 2 || response.Usage.OutputTokenDetails.ReasoningTokens != 1 {
		t.Fatalf("completion = %#v", response)
	}
	raw, ok := llm.RawResponseAs[map[string]any](response)
	if !ok || raw["extension"] != true {
		t.Fatalf("raw response = %#v", response.RawResponse)
	}
}

func TestLanguageSchemaIsNotRewritten(t *testing.T) {
	provider := languageTestProvider(
		t,
		"/gateway/chat",
		`{"messages":[{"role":"user","content":[{"type":"text","text":"Question"}]}],"stream":false,"include_thoughts":false,"response_format":{"type":"json_schema","json_schema":{"name":"answer_record","strict":true,"schema":{"properties":{"answer":{"type":"string"}}}}}}`,
		`{"choices":[{"message":{"content":"answer"},"finish_reason":"stop"}]}`,
		"application/json",
	)
	req := llm.NewRequest("Question").Build()
	req.Output = &llm.OutputSchema{
		Name:   "answer_record",
		Type:   "object",
		Schema: map[string]any{"properties": map[string]any{"answer": map[string]any{"type": "string"}}},
	}
	if _, err := provider.Chat("configured-model", "chat").Complete(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(req.Output.Schema) != 1 {
		t.Fatalf("request schema mutated: %#v", req.Output.Schema)
	}
}

func TestLanguageProviderErrors(t *testing.T) {
	for _, capture := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "capture"}[capture], func(t *testing.T) {
			body := `{"error":{"code":"rate_limit","message":"try later"},"private":"diagnostic"}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			provider := kie.NewProvider("example-key", kie.WithBaseURL(server.URL))
			ctx := context.Background()
			if capture {
				ctx = transport.WithErrorBodyCapture(ctx)
			}
			_, err := provider.Responses("configured-model").Complete(ctx, llm.NewRequest("Question").Build())
			var apiError *aikit.APIError
			var completionError *llm.CompletionError
			if !errors.As(err, &apiError) || apiError.StatusCode != 429 || apiError.Provider != "kie" ||
				apiError.Message != "try later" ||
				!errors.As(err, &completionError) ||
				completionError.Kind != llm.CompletionErrorProvider {
				t.Fatalf("error = %#v", err)
			}
			if capture && string(apiError.RawBody) != body {
				t.Fatalf("raw error body = %q", apiError.RawBody)
			}
			if !capture && len(apiError.RawBody) != 0 {
				t.Fatal("error diagnostics captured without opt-in")
			}
			if strings.Contains(err.Error(), "diagnostic") {
				t.Fatalf("raw body leaked into printable error: %v", err)
			}
		})
	}
}

func TestLanguageInvalidProviderResponsesRetainRaw(t *testing.T) {
	cases := []struct{ name, reply string }{
		{"failed", `{"status":"failed","error":{"message":"generation failed"},"extension":true}`},
		{"incomplete", `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"extension":true}`},
		{
			"refusal",
			`{"status":"completed","output":[{"type":"message","content":[{"type":"refusal","refusal":"declined"}]}],"extension":true}`,
		},
		{"missing-content", `{"status":"completed","output":[],"extension":true}`},
		{
			"missing-completed-event",
			"event: response.created\ndata: {\"type\":\"response.created\",\"extension\":true}\n\n",
		},
		{
			"missing-terminal-payload",
			"event: response.completed\ndata: {\"type\":\"response.completed\",\"extension\":true}\n\n",
		},
		{"error-event", "event: error\ndata: {\"type\":\"error\",\"message\":\"failed\",\"extension\":true}\n\n"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			provider := languageTestProvider(
				t,
				"/gateway/responses",
				`{"model":"configured-model","input":[{"role":"user","content":[{"type":"input_text","text":"Question"}]}]}`,
				test.reply,
				"application/json",
			)
			response, err := provider.Responses("configured-model").
				Complete(context.Background(), llm.NewRequest("Question").Build())
			var completionError *llm.CompletionError
			if !errors.As(err, &completionError) || completionError.Kind != llm.CompletionErrorResponse {
				t.Fatalf("error = %v", err)
			}
			if response == nil || response.RawResponse == nil {
				t.Fatalf("raw failure response not retained: %#v", response)
			}
		})
	}
}

func TestLanguageStreamNormalizesCompletedResponse(t *testing.T) {
	provider := languageTestProvider(
		t,
		"/gateway/chat",
		`{"messages":[{"role":"user","content":[{"type":"text","text":"Question"}]}],"stream":false,"include_thoughts":false}`,
		`{"choices":[{"message":{"content":"answer","reasoning_content":"summary","tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`,
		"application/json",
	)
	events, err := provider.Chat("configured-model", "chat").
		Stream(context.Background(), llm.NewRequest("Question").Build())
	if err != nil {
		t.Fatal(err)
	}
	var got []aikit.StreamEvent
	for event := range events {
		got = append(got, event)
	}
	if len(got) != 5 || got[0].TextDelta != "answer" || got[1].Type != aikit.StreamEventReasoningDelta ||
		got[2].ToolCallID != "call-1" || got[2].ToolCallName != "lookup" || got[2].ToolCallArgsDelta != "{}" ||
		got[3].Usage.TotalTokens != 5 || got[4].FinishReason != aikit.FinishReasonToolCalls {
		t.Fatalf("events = %#v", got)
	}
}

func TestLanguageRequestBodyDiagnostics(t *testing.T) {
	provider := kie.NewProvider("example-key")
	req := llm.NewRequest("Question").Build()
	req.ProviderOptions = map[string]any{"kie": map[string]any{
		"reasoningEffort": "low", "schemaName": "diagnostic_schema",
		"nativeTools": []any{map[string]any{"type": "web_search"}},
	}}
	req.Output = &llm.OutputSchema{Name: "original_name", Type: "object", Schema: map[string]any{"type": "object"}}
	body, err := provider.Responses("configured-model").RequestBody(req)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var got, want map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(
		[]byte(
			`{"model":"configured-model","input":[{"role":"user","content":[{"type":"input_text","text":"Question"}]}],"reasoning":{"effort":"low"},"tools":[{"type":"web_search"}],"text":{"format":{"type":"json_schema","name":"diagnostic_schema","strict":true,"schema":{"type":"object"}}}}`,
		),
		&want,
	); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diagnostic request = %#v, want %#v", got, want)
	}
}

func TestLanguageInvalidOptionsAndPath(t *testing.T) {
	provider := kie.NewProvider("example-key")
	for _, value := range []any{123, (*kie.LanguageOptions)(nil), map[string]any{"unknown": true}} {
		req := llm.NewRequest("Question").Build()
		req.ProviderOptions = map[string]any{"kie": value}
		_, err := provider.Responses("configured-model").RequestBody(req)
		if err == nil {
			t.Fatalf("expected invalid provider options error for %#v", value)
		}
	}
	_, err := provider.Chat("configured-model", "").Complete(context.Background(), llm.NewRequest("Question").Build())
	var completionError *llm.CompletionError
	if !errors.As(err, &completionError) || completionError.Kind != llm.CompletionErrorRequest {
		t.Fatalf("empty path error = %v", err)
	}
}

func TestLanguageMalformedJSON(t *testing.T) {
	provider := languageTestProvider(
		t,
		"/gateway/responses",
		`{"model":"configured-model","input":[{"role":"user","content":[{"type":"input_text","text":"Question"}]}]}`,
		`{"status":`,
		"application/json",
	)
	_, err := provider.Responses("configured-model").Complete(context.Background(), llm.NewRequest("Question").Build())
	var completionError *llm.CompletionError
	if !errors.As(err, &completionError) || completionError.Kind != llm.CompletionErrorJSON {
		t.Fatalf("malformed JSON error = %v", err)
	}
}
