package openrouter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

type captured struct {
	body    map[string]any
	headers http.Header
	path    string
}

func serve(t *testing.T, sse []byte) (*httptest.Server, *captured) {
	t.Helper()
	got := &captured{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got.path = r.URL.Path
		got.headers = r.Header.Clone()
		got.body = nil
		if err := json.Unmarshal(raw, &got.body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(sse)
	}))
	t.Cleanup(server.Close)
	return server, got
}

func run(t *testing.T, cfg Config, req llm.Request) []aikit.StreamEvent {
	t.Helper()
	events, err := NewLanguageModel("example/model", cfg).Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var out []aikit.StreamEvent
	for e := range events {
		out = append(out, e)
	}
	return out
}

func userReq(text string, opts ...any) llm.Request {
	req := llm.Request{Messages: []aikit.Message{aikit.UserMessage(text)}}
	if len(opts) > 0 {
		req.ProviderOptions = map[string]any{"openrouter": opts[0]}
	}
	return req
}

func TestStreamTextFixtureWithKeepaliveAndCost(t *testing.T) {
	server, got := serve(t, fixture(t, "text_stream.sse"))
	events := run(
		t,
		Config{APIKey: "k", BaseURL: server.URL + "/api/v1", Referer: "https://app.example", Title: "Example"},
		userReq("hi"),
	)

	if got.path != "/api/v1/chat/completions" {
		t.Fatalf("path = %s", got.path)
	}
	if got.headers.Get("Authorization") != "Bearer k" ||
		got.headers.Get("HTTP-Referer") != "https://app.example" ||
		got.headers.Get("X-Title") != "Example" {
		t.Fatalf("headers = %v", got.headers)
	}

	var text, reasoning strings.Builder
	var usage *aikit.Usage
	var finish aikit.StreamEvent
	for _, e := range events {
		switch e.Type {
		case aikit.StreamEventTextDelta:
			text.WriteString(e.TextDelta)
		case aikit.StreamEventReasoningDelta:
			reasoning.WriteString(e.TextDelta)
		case aikit.StreamEventUsage:
			usage = e.Usage
		case aikit.StreamEventFinish:
			finish = e
		case aikit.StreamEventError:
			t.Fatalf("unexpected error event: %v", e.Error)
		}
	}
	if text.String() != "Hello world" || reasoning.String() != "thinking about it" {
		t.Fatalf("text=%q reasoning=%q", text.String(), reasoning.String())
	}
	if usage == nil {
		t.Fatal("no usage event")
	}
	cost, ok := Cost(*usage)
	if !ok || cost != 0.000273 {
		t.Fatalf("cost = %v ok=%v", cost, ok)
	}
	if usage.InputTokenDetails.CacheReadTokens != 100 || usage.OutputTokenDetails.ReasoningTokens != 12 {
		t.Fatalf("usage details = %+v", usage)
	}
	if _, ok := usage.Raw["cost_details"].(map[string]any); !ok {
		t.Fatalf("raw usage lost cost_details: %#v", usage.Raw)
	}
	if finish.FinishReason != aikit.FinishReasonStop {
		t.Fatalf("finish = %+v", finish)
	}
	meta, _ := finish.ProviderMetadata["openrouter"].(map[string]any)
	if meta["responseId"] != "gen-test-1" || meta["model"] != "example/answering-model" {
		t.Fatalf("metadata = %#v", finish.ProviderMetadata)
	}
}

func TestStreamToolCallFixture(t *testing.T) {
	server, _ := serve(t, fixture(t, "tool_call_stream.sse"))
	events := run(t, Config{BaseURL: server.URL}, userReq("hi"))

	type call struct{ id, name, args string }
	calls := map[int]*call{}
	var finish aikit.FinishReason
	for _, e := range events {
		switch e.Type {
		case aikit.StreamEventToolCallDelta:
			c := calls[e.ToolCallIndex]
			if c == nil {
				c = &call{}
				calls[e.ToolCallIndex] = c
			}
			if e.ToolCallID != "" {
				c.id = e.ToolCallID
			}
			if e.ToolCallName != "" {
				c.name = e.ToolCallName
			}
			c.args += e.ToolCallArgsDelta
		case aikit.StreamEventFinish:
			finish = e.FinishReason
		case aikit.StreamEventError:
			t.Fatalf("unexpected error event: %v", e.Error)
		}
	}
	if finish != aikit.FinishReasonToolCalls || len(calls) != 2 {
		t.Fatalf("finish=%v calls=%d", finish, len(calls))
	}
	if *calls[0] != (call{"call_a", "lookup", `{"q":"widgets"}`}) || *calls[1] != (call{"call_b", "count", "{}"}) {
		t.Fatalf("calls = %+v %+v", *calls[0], *calls[1])
	}
}

func TestStreamMidStreamErrorSurfaces(t *testing.T) {
	server, _ := serve(t, fixture(t, "midstream_error.sse"))
	events := run(t, Config{BaseURL: server.URL}, userReq("hi"))
	var sawText bool
	var errEvent error
	for _, e := range events {
		if e.Type == aikit.StreamEventTextDelta {
			sawText = true
		}
		if e.Type == aikit.StreamEventError {
			errEvent = e.Error
		}
	}
	if !sawText || errEvent == nil || !strings.Contains(errEvent.Error(), "Provider disconnected unexpectedly") ||
		!strings.Contains(errEvent.Error(), "server_error") {
		t.Fatalf("sawText=%v err=%v", sawText, errEvent)
	}
}

func TestRequestBodyOptions(t *testing.T) {
	server, got := serve(t, fixture(t, "text_stream.sse"))
	yes, no := true, false
	run(t, Config{BaseURL: server.URL}, llm.Request{
		Messages: []aikit.Message{aikit.UserMessage("hi")},
		Tools:    []aikit.ToolDefinition{{Name: "lookup", InputSchema: map[string]any{"type": "object"}}},
		Output: &llm.OutputSchema{Type: "object", Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"a": map[string]any{"type": "string"}},
		}},
		ProviderOptions: map[string]any{"openrouter": ProviderOptions{
			Reasoning: &Reasoning{Effort: "high", Exclude: true},
			Provider: &Routing{
				Order:             []string{"ExampleHost"},
				AllowFallbacks:    &no,
				RequireParameters: &yes,
				DataCollection:    "deny",
			},
			Models:            []string{"example/fallback"},
			Transforms:        []string{"middle-out"},
			ParallelToolCalls: &no,
			User:              "u-1",
		}},
	})
	b := got.body
	if b["usage"].(map[string]any)["include"] != true {
		t.Fatalf("usage = %v", b["usage"])
	}
	reasoning := b["reasoning"].(map[string]any)
	if reasoning["effort"] != "high" || reasoning["exclude"] != true {
		t.Fatalf("reasoning = %v", reasoning)
	}
	routing := b["provider"].(map[string]any)
	if routing["allow_fallbacks"] != false || routing["require_parameters"] != true ||
		routing["data_collection"] != "deny" || routing["order"].([]any)[0] != "ExampleHost" {
		t.Fatalf("provider = %v", routing)
	}
	if b["models"].([]any)[0] != "example/fallback" || b["transforms"].([]any)[0] != "middle-out" ||
		b["parallel_tool_calls"] != false || b["user"] != "u-1" {
		t.Fatalf("body = %v", b)
	}
	format := b["response_format"].(map[string]any)
	schema := format["json_schema"].(map[string]any)
	if format["type"] != "json_schema" || schema["strict"] != true {
		t.Fatalf("response_format = %v", format)
	}
}

func TestUsageIncludeDefaultsAndOptOut(t *testing.T) {
	server, got := serve(t, fixture(t, "text_stream.sse"))
	run(t, Config{BaseURL: server.URL}, userReq("hi"))
	if _, ok := got.body["usage"]; !ok {
		t.Fatal("usage.include must default on")
	}
	for _, k := range []string{"reasoning", "provider", "models", "transforms", "parallel_tool_calls"} {
		if _, ok := got.body[k]; ok {
			t.Fatalf("%s present without options", k)
		}
	}
	off := false
	run(t, Config{BaseURL: server.URL}, userReq("hi", ProviderOptions{IncludeUsage: &off}))
	if _, ok := got.body["usage"]; ok {
		t.Fatal("usage present after opt-out")
	}
}

func TestProviderOptionsFromMapAndErrors(t *testing.T) {
	server, got := serve(t, fixture(t, "text_stream.sse"))
	run(t, Config{BaseURL: server.URL}, userReq("hi", map[string]any{
		"reasoning": map[string]any{"effort": "low"},
	}))
	if got.body["reasoning"].(map[string]any)["effort"] != "low" {
		t.Fatalf("body = %v", got.body)
	}
	if _, err := NewLanguageModel("m", Config{BaseURL: server.URL}).Stream(
		context.Background(), userReq("hi", 42),
	); err == nil {
		t.Fatal("expected type error")
	}
	var nilOpts *ProviderOptions
	if _, err := NewLanguageModel("m", Config{BaseURL: server.URL}).Stream(
		context.Background(), userReq("hi", nilOpts),
	); err == nil {
		t.Fatal("expected nil pointer error")
	}
	ptr := &ProviderOptions{User: "p"}
	run(t, Config{BaseURL: server.URL}, userReq("hi", ptr))
	if got.body["user"] != "p" {
		t.Fatalf("pointer options not applied: %v", got.body)
	}
}

func TestHTTPRejectionIsTypedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":{"code":402,"message":"insufficient credits"}}`))
	}))
	defer server.Close()
	_, err := NewLanguageModel("m", Config{BaseURL: server.URL}).Stream(context.Background(), userReq("hi"))
	var apiErr *aikit.APIError
	if err == nil || !asAPIError(err, &apiErr) || apiErr.StatusCode != http.StatusPaymentRequired {
		t.Fatalf("err = %v", err)
	}
}

func TestProviderRegistryAndCost(t *testing.T) {
	p := NewProvider(Config{APIKey: "k"})
	if p.Name() != "openrouter" || p.LanguageModel("a/b").ModelID() != "a/b" {
		t.Fatal("provider wiring")
	}
	t.Setenv("OPENROUTER_API_KEY", "env")
	if NewFromEnv().config.APIKey != "env" {
		t.Fatal("env key")
	}
	if _, ok := Cost(aikit.Usage{}); ok {
		t.Fatal("cost without raw")
	}
	if c, ok := Cost(aikit.Usage{Raw: map[string]any{"cost": 3}}); !ok || c != 3 {
		t.Fatal("int cost")
	}
	if _, ok := Cost(aikit.Usage{Raw: map[string]any{"cost": "x"}}); ok {
		t.Fatal("string cost")
	}
}
