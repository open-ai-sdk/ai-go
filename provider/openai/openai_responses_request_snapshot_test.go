package openai

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
	"github.com/open-ai-sdk/ai-go/llm"
)

const updateSnapshotsEnv = "AIGO_UPDATE_SNAPSHOTS"

func boolPtr(b bool) *bool { return &b }

func wireBody(t *testing.T, req llm.Request) (map[string]any, []ai.Warning) {
	t.Helper()
	r, warnings, err := encodeRequest("gpt-test", req, true)
	if err != nil {
		t.Fatalf("encodeRequest: %v", err)
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out, warnings
}

func fullRequest() llm.Request {
	temp, topP := float32(0.25), float32(0.5)
	return llm.Request{
		Instructions: "be brief",
		Messages:     []ai.Message{ai.UserMessage("find the weather")},
		Tools: []ai.ToolDefinition{{
			Name:        "lookup",
			Description: "look something up",
			InputSchema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"q": map[string]any{"type": "string"}},
				"required":             []any{"q"},
				"additionalProperties": false,
			},
			Strict: true,
		}},
		ToolChoice: &ai.ToolChoice{Type: "tool", ToolName: "lookup"},
		Output: &llm.OutputSchema{Type: "object", Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"answer": map[string]any{"type": "string"}},
		}},
		Settings: llm.CallSettings{Temperature: &temp, TopP: &topP, MaxTokens: 256},
		ProviderOptions: map[string]any{"openai": ProviderOptions{
			PreviousResponseID: "resp_prev",
			ReasoningEffort:    "low",
			ReasoningSummary:   "auto",
			EnableWebSearch:    true,
			WebSearchToolType:  "web_search",
			IncludeSources:     true,
			ParallelToolCalls:  boolPtr(false),
			Store:              boolPtr(false),
			User:               "user-1",
			Metadata:           map[string]string{"k": "v"},
			PromptCacheKey:     "cache-key",
		}},
	}
}

// TestEncodeRequest_FullSnapshot pins the complete wire body for a request
// that sets every llm.Request field. Regenerate with AIGO_UPDATE_SNAPSHOTS=1.
func TestEncodeRequest_FullSnapshot(t *testing.T) {
	body, warnings := wireBody(t, fullRequest())
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
	got, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	const path = "testdata/responses_request_full.golden.json"
	if os.Getenv(updateSnapshotsEnv) != "" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("wire body mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestEncodeRequest_ToolChoiceWire(t *testing.T) {
	tools := []ai.ToolDefinition{{Name: "lookup", InputSchema: map[string]any{"type": "object"}}}
	tests := []struct {
		name   string
		choice *ai.ToolChoice
		want   any
	}{
		{"nil omitted", nil, nil},
		{"auto", &ai.ToolChoice{Type: "auto"}, "auto"},
		{"none", &ai.ToolChoice{Type: "none"}, "none"},
		{"required", &ai.ToolChoice{Type: "required"}, "required"},
		{
			"named", &ai.ToolChoice{Type: "tool", ToolName: "lookup"},
			map[string]any{"type": "function", "name": "lookup"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := wireBody(t, llm.Request{
				Messages: []ai.Message{ai.UserMessage("x")}, Tools: tools, ToolChoice: tt.choice,
			})
			got, present := body["tool_choice"]
			if tt.want == nil {
				if present {
					t.Fatalf("tool_choice = %v, want omitted", got)
				}
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("tool_choice = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestEncodeRequest_ToolChoiceErrors(t *testing.T) {
	tools := []ai.ToolDefinition{{Name: "lookup"}}
	for name, choice := range map[string]*ai.ToolChoice{
		"unknown type":   {Type: "sometimes"},
		"missing name":   {Type: "tool"},
		"undeclared one": {Type: "tool", ToolName: "other"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := encodeRequest("gpt-test", llm.Request{
				Messages: []ai.Message{ai.UserMessage("x")}, Tools: tools, ToolChoice: choice,
			}, false)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestEncodeRequest_ToolChoiceOmittedWithoutTools(t *testing.T) {
	body, _ := wireBody(t, llm.Request{
		Messages:        []ai.Message{ai.UserMessage("x")},
		ToolChoice:      &ai.ToolChoice{Type: "required"},
		ProviderOptions: map[string]any{"openai": ProviderOptions{ParallelToolCalls: boolPtr(true)}},
	})
	for _, k := range []string{"tool_choice", "parallel_tool_calls", "tools"} {
		if _, ok := body[k]; ok {
			t.Fatalf("%s present without tools", k)
		}
	}
}

func TestEncodeRequest_ParallelToolCallsWire(t *testing.T) {
	for _, v := range []bool{true, false} {
		body, _ := wireBody(t, llm.Request{
			Messages:        []ai.Message{ai.UserMessage("x")},
			Tools:           []ai.ToolDefinition{{Name: "lookup"}},
			ProviderOptions: map[string]any{"openai": ProviderOptions{ParallelToolCalls: boolPtr(v)}},
		})
		if got, ok := body["parallel_tool_calls"].(bool); !ok || got != v {
			t.Fatalf("parallel_tool_calls = %#v, want %v", body["parallel_tool_calls"], v)
		}
	}
	body, _ := wireBody(t, llm.Request{
		Messages: []ai.Message{ai.UserMessage("x")}, Tools: []ai.ToolDefinition{{Name: "lookup"}},
	})
	if _, ok := body["parallel_tool_calls"]; ok {
		t.Fatal("parallel_tool_calls must be omitted when unset")
	}
}

func TestEncodeRequest_StrictFunctionTools(t *testing.T) {
	body, _ := wireBody(t, llm.Request{
		Messages: []ai.Message{ai.UserMessage("x")},
		Tools: []ai.ToolDefinition{
			{Name: "strict_tool", Strict: true},
			{Name: "loose_tool"},
		},
	})
	tools := body["tools"].([]any)
	if tools[0].(map[string]any)["strict"] != true {
		t.Fatalf("strict tool wire = %#v", tools[0])
	}
	if _, ok := tools[1].(map[string]any)["strict"]; ok {
		t.Fatalf("non-strict tool must omit strict: %#v", tools[1])
	}
}

func TestEncodeRequest_WebSearchToolType(t *testing.T) {
	for _, tt := range []struct{ configured, want string }{
		{"", "web_search_preview"},
		{"web_search", "web_search"},
	} {
		body, _ := wireBody(t, llm.Request{
			Messages: []ai.Message{ai.UserMessage("x")},
			ProviderOptions: map[string]any{"openai": ProviderOptions{
				EnableWebSearch: true, WebSearchToolType: tt.configured,
			}},
		})
		tools := body["tools"].([]any)
		if got := tools[0].(map[string]any)["type"]; got != tt.want {
			t.Fatalf("web search type = %v, want %s", got, tt.want)
		}
	}
}

// TestEncodeRequest_EveryRequestFieldAccountedFor fails when llm.Request or
// llm.CallSettings gain a field that is neither encoded nor listed here, which
// is how silent request drops (such as the old missing tool_choice) start.
func TestEncodeRequest_EveryRequestFieldAccountedFor(t *testing.T) {
	covered := map[string]bool{
		"Instructions": true, "Messages": true, "Tools": true, "ToolChoice": true,
		"Output": true, "Settings": true, "ProviderOptions": true,
		// Runtime-only contexts never leave the process.
		"ToolsContext": true, "RuntimeContext": true,
	}
	rt := reflect.TypeOf(llm.Request{})
	for i := 0; i < rt.NumField(); i++ {
		if !covered[rt.Field(i).Name] {
			t.Errorf("llm.Request.%s is not covered by the Responses encoder snapshot", rt.Field(i).Name)
		}
	}
	settings := map[string]bool{
		"Temperature": true, "MaxTokens": true, "TopP": true,
		"TopK": true, "Seed": true, "StopSequences": true, // last three warn
	}
	st := reflect.TypeOf(llm.CallSettings{})
	for i := 0; i < st.NumField(); i++ {
		if !settings[st.Field(i).Name] {
			t.Errorf("llm.CallSettings.%s is not covered by the Responses encoder snapshot", st.Field(i).Name)
		}
	}
}

func TestEncodeRequest_SettingsReachWireOrWarn(t *testing.T) {
	temp, topP, topK, seed := float32(0.1), float32(0.9), 40, 7
	body, warnings := wireBody(t, llm.Request{
		Messages: []ai.Message{ai.UserMessage("x")},
		Settings: llm.CallSettings{
			Temperature: &temp, TopP: &topP, MaxTokens: 99,
			TopK: &topK, Seed: &seed, StopSequences: []string{"END"},
		},
	})
	if body["max_output_tokens"] != float64(99) || body["top_p"] == nil || body["temperature"] == nil {
		t.Fatalf("settings missing from wire: %#v", body)
	}
	var settings []string
	for _, w := range warnings {
		if w.Type == "unsupported-setting" {
			settings = append(settings, w.Setting)
		}
	}
	for _, want := range []string{"topK", "seed", "stopSequences"} {
		if !strings.Contains(strings.Join(settings, ","), want) {
			t.Errorf("missing unsupported-setting warning for %s (got %v)", want, settings)
		}
	}
}

func TestOpenAISSE_AnsweringModelInMetadata(t *testing.T) {
	sse := `data: {"type":"response.created","response":{"id":"resp_m","model":"gpt-test-2026-01-01","status":"in_progress"}}
data: {"type":"response.completed","response":{"id":"resp_m","model":"gpt-test-2026-01-01","status":"completed"}}
`
	for _, e := range collectEvents(streamFromString(sse)) {
		if e.Type != ai.StreamEventFinish {
			continue
		}
		meta := e.ProviderMetadata["openai"].(map[string]any)
		if meta["responseId"] != "resp_m" || meta["model"] != "gpt-test-2026-01-01" {
			t.Fatalf("metadata = %#v", meta)
		}
		return
	}
	t.Fatal("no finish event")
}

func TestChatRewrite_ParallelToolCalls(t *testing.T) {
	b := chatBackend{}
	opts := map[string]any{"openai": ChatProviderOptions{ParallelToolCalls: boolPtr(false)}}
	with, err := b.RewriteRequest(llm.Request{ProviderOptions: opts},
		map[string]any{"tools": []any{map[string]any{"type": "function"}}})
	if err != nil || with["parallel_tool_calls"] != false {
		t.Fatalf("with tools: %#v err=%v", with, err)
	}
	without, err := b.RewriteRequest(llm.Request{ProviderOptions: opts}, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := without["parallel_tool_calls"]; ok {
		t.Fatal("parallel_tool_calls must be omitted without tools")
	}
}
