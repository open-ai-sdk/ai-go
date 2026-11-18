package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/transport"
)

func TestResponsesOptionalWirePresentation(t *testing.T) {
	parallel := false
	call := aikit.ToolCallPart("call_1", "lookup", json.RawMessage(`{"q":"x"}`))
	call.ItemID = "fc_original"
	req := llm.Request{
		Instructions: "Use exact data",
		Messages: []aikit.Message{
			{
				Role: aikit.RoleUser,
				Content: []aikit.ContentPart{
					aikit.TextPart("Look"),
					aikit.ImageURLPart("https://example.org/image.png"),
				},
			},
			{Role: aikit.RoleAssistant, Content: []aikit.ContentPart{call}},
			{Role: aikit.RoleTool, Content: []aikit.ContentPart{aikit.ToolResultPart("call_1", "lookup", "found")}},
		},
		ToolChoice: &aikit.ToolChoice{Type: "auto"},
		Output:     &llm.OutputSchema{Type: "object", Name: "generic_name", Schema: map[string]any{"type": "object"}},
	}
	baseline, _, err := encodeRequest("example-model", req, false)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Instructions != "" || baseline.Input[0].Role != "system" || baseline.Input[0].Type != "" ||
		len(baseline.Input[1].Content) != 2 ||
		baseline.ToolChoice != nil ||
		baseline.ParallelToolCalls != nil ||
		len(baseline.Include) != 0 ||
		baseline.Input[2].Status != "" {
		t.Fatalf("default changed: %#v", baseline)
	}
	if baseline.Text.Format.Name != "generic_name" {
		t.Fatal(baseline.Text.Format.Name)
	}
	req.ProviderOptions = map[string]any{"openai": ProviderOptions{
		InstructionsAsTopLevel: true, ExplicitMessageType: true, SplitUserContentParts: true,
		ImageDetail: "auto", IncludeEncryptedReasoning: true, ToolPolicyWithoutTools: true,
		ParallelToolCalls: &parallel, HistoryItemStatus: "completed", OutputSchemaName: "custom_name",
	}}
	encoded, _, err := encodeRequest("example-model", req, true)
	if err != nil {
		t.Fatal(err)
	}
	if encoded.Instructions != req.Instructions || len(encoded.Input) != 4 {
		t.Fatalf("input: %#v", encoded)
	}
	for _, item := range encoded.Input[:2] {
		if item.Type != "message" || item.Role != "user" || len(item.Content) != 1 {
			t.Fatalf("message: %#v", item)
		}
	}
	if image := encoded.Input[1].Content[0].(inputImageURLPart); image.Detail != "auto" {
		t.Fatal(image)
	}
	if item := encoded.Input[2]; item.ID != "fc_original" || item.CallID != "call_1" || item.Status != "completed" {
		t.Fatal(item)
	}
	if encoded.Input[3].Status != "completed" || encoded.Text.Format.Name != "custom_name" ||
		encoded.ToolChoice != "auto" ||
		encoded.ParallelToolCalls == nil ||
		*encoded.ParallelToolCalls ||
		len(encoded.Include) != 1 ||
		encoded.Include[0] != "reasoning.encrypted_content" ||
		encoded.Store != nil {
		t.Fatalf("options: %#v", encoded)
	}
}

func TestResponsesSSEErrorVariants(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, code := range []string{"server_error", "invalid_request_error"} {
			for _, capture := range []bool{false, true} {
				raw := `{"type":"error","code":"` + code + `","message":"failure"}`
				if nested {
					raw = `{"type":"error","error":{"code":"` + code + `","message":"failure"}}`
				}
				ctx := context.Background()
				if capture {
					ctx = transport.WithErrorBodyCapture(ctx)
				}
				events := make(chan aikit.StreamEvent, 4)
				err := decodeResponsesSSEStreamWithRequestID(
					ctx,
					transport.NewSSEReader(strings.NewReader("data: "+raw+"\n\n")),
					events,
					"req_1",
				)
				if err != nil {
					t.Fatal(err)
				}
				event := <-events
				var apiErr *aikit.APIError
				if event.Type != aikit.StreamEventError || !errors.As(event.Error, &apiErr) {
					t.Fatalf("error: %#v", event)
				}
				if apiErr.StatusCode != 0 || apiErr.Code != code || apiErr.Message != "failure" ||
					apiErr.RequestID != "req_1" ||
					apiErr.Retryable() != (code == "server_error") {
					t.Fatalf("api error: %#v", apiErr)
				}
				if capture && string(apiErr.RawBody) != raw || !capture && len(apiErr.RawBody) != 0 {
					t.Fatalf("capture: %q", apiErr.RawBody)
				}
			}
		}
	}
}

func TestResponsesNativeMetadataAndRoute(t *testing.T) {
	raw := `{"id":"resp_1","model":"actual-model","status":"completed","extra":{"preserved":true},"output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"{}"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/custom/responses" || r.Header.Get("X-Example") != "value" {
			t.Errorf("request: %s %v", r.URL.Path, r.Header)
		}
		var body struct {
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.Header().Set("x-request-id", "req_1")
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":"+raw+"}\n\n")
		} else {
			_, _ = io.WriteString(w, raw)
		}
	}))
	defer server.Close()
	model := NewLanguageModel(
		"example-model",
		Config{
			BaseURL:       server.URL,
			ResponsesPath: "custom/responses",
			Headers:       map[string]string{"X-Example": "value"},
		},
	)
	completed, err := model.Complete(context.Background(), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	native := completed.RawResponse.(*ResponsesResponse)
	if string(native.Raw) != raw || native.RequestID != "req_1" || completed.Message.Content[0].ItemID != "fc_1" {
		t.Fatalf("completion: %#v", completed)
	}
	events, err := model.Stream(context.Background(), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	var finish *aikit.StreamEvent
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == aikit.StreamEventFinish {
			copy := event
			finish = &copy
		}
	}
	if finish == nil {
		t.Fatal("missing finish")
	}
	metadata := finish.ProviderMetadata["openai"].(map[string]any)
	if metadata["requestID"] != "req_1" || string(metadata["rawResponse"].(json.RawMessage)) != raw {
		t.Fatalf("metadata: %#v", metadata)
	}
}
