package openaicompat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

func TestSSE_UsageKeepsFullRawObject(t *testing.T) {
	body := sseBody(
		`data: {"id":"gen-1","model":"m-1","choices":[{"delta":{"content":"hi"},"finish_reason":"stop"}]}`,
		`data: {"id":"gen-1","model":"m-1","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14,`+
			`"cost":0.5,"cost_details":{"upstream_inference_cost":0.4},`+
			`"prompt_tokens_details":{"cached_tokens":3,"cache_write_tokens":2},`+
			`"completion_tokens_details":{"reasoning_tokens":1}}}`,
		`data: [DONE]`,
	)
	events := collectEvents(decodeTestStream(context.Background(), body, SSEDecodeParams{ProviderName: "p"}))
	var usage *aikit.Usage
	for _, e := range events {
		if e.Type == aikit.StreamEventUsage {
			usage = e.Usage
		}
	}
	if usage == nil {
		t.Fatal("no usage event")
	}
	if usage.Raw["cost"] != 0.5 {
		t.Fatalf("cost lost: %#v", usage.Raw)
	}
	if _, ok := usage.Raw["cost_details"].(map[string]any); !ok {
		t.Fatalf("cost_details lost: %#v", usage.Raw)
	}
	if usage.Raw["cached_tokens"] != 3 || usage.Raw["reasoning_tokens"] != 1 {
		t.Fatalf("normalized counters missing: %#v", usage.Raw)
	}
	if usage.InputTokenDetails.NoCacheTokens != 5 || usage.InputTokenDetails.CacheWriteTokens != 2 {
		t.Fatalf("details = %+v", usage.InputTokenDetails)
	}
}

func TestSSE_FinishCarriesResponseIDAndModel(t *testing.T) {
	body := sseBody(
		`data: {"id":"gen-9","model":"answering-model","choices":[{"delta":{"content":"a"}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	)
	events := collectEvents(decodeTestStream(context.Background(), body, SSEDecodeParams{ProviderName: "p"}))
	for _, e := range events {
		if e.Type != aikit.StreamEventFinish {
			continue
		}
		meta, _ := e.ProviderMetadata["p"].(map[string]any)
		if meta["responseId"] != "gen-9" || meta["model"] != "answering-model" {
			t.Fatalf("metadata = %#v", e.ProviderMetadata)
		}
		return
	}
	t.Fatal("no finish event")
}

func TestSSE_DoneWithoutFinishCarriesIdentity(t *testing.T) {
	body := sseBody(`data: {"id":"g","model":"m","choices":[{"delta":{"content":"a"}}]}`, `data: [DONE]`)
	for _, e := range collectEvents(decodeTestStream(context.Background(), body, SSEDecodeParams{})) {
		if e.Type == aikit.StreamEventFinish {
			meta, _ := e.ProviderMetadata["openaichat"].(map[string]any)
			if meta["responseId"] != "g" {
				t.Fatalf("metadata = %#v", e.ProviderMetadata)
			}
			return
		}
	}
	t.Fatal("no finish event")
}

func TestSSE_ExtractorMetadataNotOverwritten(t *testing.T) {
	params := SSEDecodeParams{
		ProviderName: "p",
		MetadataExtractor: func(StreamChunk) map[string]any {
			return map[string]any{"p": map[string]any{"model": "extractor-model"}}
		},
	}
	body := sseBody(`data: {"id":"g","model":"stream-model","choices":[{"delta":{},"finish_reason":"stop"}]}`)
	for _, e := range collectEvents(decodeTestStream(context.Background(), body, params)) {
		if e.Type == aikit.StreamEventFinish {
			meta := e.ProviderMetadata["p"].(map[string]any)
			if meta["model"] != "extractor-model" || meta["responseId"] != "g" {
				t.Fatalf("metadata = %#v", meta)
			}
			return
		}
	}
	t.Fatal("no finish event")
}

func TestSSE_ErrorChunkEndsStreamWithError(t *testing.T) {
	cases := map[string]struct{ line, want string }{
		"message and code": {`data: {"error":{"message":"boom","code":502}}`, "boom (code 502)"},
		"message only":     {`data: {"error":{"message":"just text"}}`, "just text"},
		"code only":        {`data: {"error":{"code":"x"}}`, "code x"},
		"empty":            {`data: {"error":{}}`, "unspecified stream error"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			body := sseBody(`data: {"choices":[{"delta":{"content":"a"}}]}`, tc.line)
			var gotErr error
			for _, e := range collectEvents(decodeTestStream(context.Background(), body, SSEDecodeParams{ProviderName: "p"})) {
				if e.Type == aikit.StreamEventError {
					gotErr = e.Error
				}
			}
			if gotErr == nil || !strings.Contains(gotErr.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", gotErr, tc.want)
			}
		})
	}
}

func TestStreamUsageUnmarshalRejectsNonObject(t *testing.T) {
	var chunk StreamChunk
	if err := json.Unmarshal([]byte(`{"usage":"nope"}`), &chunk); err == nil {
		t.Fatal("expected error")
	}
}

func TestEncodeToolChoiceVariants(t *testing.T) {
	cases := []struct {
		in   *aikit.ToolChoice
		want string
	}{
		{nil, `"auto"`},
		{&aikit.ToolChoice{Type: "none"}, `"none"`},
		{&aikit.ToolChoice{Type: "required"}, `"required"`},
		{&aikit.ToolChoice{Type: "weird"}, `"auto"`},
		{&aikit.ToolChoice{Type: "tool", ToolName: "t"}, `{"function":{"name":"t"},"type":"function"}`},
	}
	for _, tc := range cases {
		raw, _ := json.Marshal(encodeToolChoice(tc.in))
		if string(raw) != tc.want {
			t.Errorf("encodeToolChoice(%v) = %s, want %s", tc.in, raw, tc.want)
		}
	}
}

func TestEncodeRequest_ToolsStrictAndChoiceAndSchema(t *testing.T) {
	temp := float32(0.3)
	req := llm.Request{
		Instructions: "sys",
		Messages: []aikit.Message{
			aikit.UserMessage("q"),
			{Role: aikit.RoleTool, Content: []aikit.ContentPart{{
				Type: aikit.ContentPartTypeToolResult, ToolResultID: "c1", ToolResultOutput: "result",
			}}},
		},
		Tools: []aikit.ToolDefinition{
			{Name: "a", Strict: true, InputSchema: map[string]any{"type": "object"}},
			{Name: "b", InputSchema: map[string]any{"type": "object"}},
		},
		ToolChoice: &aikit.ToolChoice{Type: "required"},
		Output:     &llm.OutputSchema{Type: "object", Schema: map[string]any{"properties": map[string]any{}}},
		Settings:   llm.CallSettings{Temperature: &temp, MaxTokens: 5},
	}
	cr, _, err := EncodeRequest(EncodeRequestParams{ModelID: "m", SupportsStructuredOutput: true}, req, false)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(cr)
	var wire map[string]any
	_ = json.Unmarshal(raw, &wire)
	tools := wire["tools"].([]any)
	fnA := tools[0].(map[string]any)["function"].(map[string]any)
	fnB := tools[1].(map[string]any)["function"].(map[string]any)
	if fnA["strict"] != true {
		t.Fatalf("strict missing on a: %v", fnA)
	}
	if _, ok := fnB["strict"]; ok {
		t.Fatalf("strict must be omitted on b: %v", fnB)
	}
	if wire["tool_choice"] != "required" || wire["max_tokens"] != float64(5) {
		t.Fatalf("wire = %v", wire)
	}
	format := wire["response_format"].(map[string]any)
	if format["type"] != "json_schema" {
		t.Fatalf("format = %v", format)
	}
	if schema := format["json_schema"].(map[string]any)["schema"].(map[string]any); schema["type"] != "object" {
		t.Fatalf("schema not wrapped: %v", schema)
	}
	msgs := wire["messages"].([]any)
	if last := msgs[len(msgs)-1].(map[string]any); last["role"] != "tool" || last["tool_call_id"] != "c1" {
		t.Fatalf("tool message = %v", last)
	}
}

func TestEncodeOutputSchemaFallbacks(t *testing.T) {
	if f := encodeOutputSchema(&llm.OutputSchema{Type: "object"}, false); f.Type != "json_object" {
		t.Fatalf("unsupported structured output must fall back: %+v", f)
	}
	if f := encodeOutputSchema(&llm.OutputSchema{Type: "json_object"}, true); f.Type != "json_object" {
		t.Fatalf("json_object = %+v", f)
	}
}

func TestMapFinishReason(t *testing.T) {
	for in, want := range map[string]aikit.FinishReason{
		"stop": aikit.FinishReasonStop, "TOOL_CALLS": aikit.FinishReasonToolCalls,
		"length": aikit.FinishReasonLength, "content_filter": aikit.FinishReasonContentFilter,
		"error": aikit.FinishReasonUnknown,
	} {
		if got := MapFinishReason(in); got != want {
			t.Errorf("MapFinishReason(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestModelAccessorsAndMissingProvider(t *testing.T) {
	m := NewModel(Config{ModelID: "x"})
	if m.ModelID() != "x" {
		t.Fatal("ModelID")
	}
	if _, err := m.Stream(
		context.Background(),
		llm.Request{Messages: []aikit.Message{aikit.UserMessage("q")}},
	); err == nil {
		t.Fatal("expected missing-provider error")
	}
	if providerName(nil) != "openai-compatible" || providerCapabilities(nil) != (CapabilityFlags{}) {
		t.Fatal("defaults")
	}
	if m.NativeSchemaSupport() != llm.NativeSchemaFull {
		t.Fatal("native schema default")
	}
	if _, err := structToMap(make(chan int)); err == nil {
		t.Fatal("structToMap should fail on unmarshalable input")
	}
}
