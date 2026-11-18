package openai

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

// A semantic user turn can alternate user parts and tool results. The facade
// splits it at role boundaries without regrouping parts: user text/images go
// to RoleUser, tool results to RoleTool. SplitUserContentParts then gives each
// ordinary user part its own wire message, while nested tool-result content
// stays inside its one function_call_output in exactly its supplied order.
func TestResponsesOrderedMultimodalSemanticMessageRecipe(t *testing.T) {
	tool := aikit.ToolResultPart("call_mixed", "lookup", "must not override structured content")
	tool.ToolResultContent = []aikit.ToolResultContent{
		aikit.ImageToolResultContent([]byte{1, 2, 3}, "image/png"),
		aikit.TextToolResultContent("before JSON"),
		aikit.JSONToolResultContent(json.RawMessage(`{"answer":42}`)),
		aikit.ImageToolResultContent([]byte{4, 5, 6}, "image/jpeg"),
		aikit.TextToolResultContent("after image"),
	}
	req := llm.Request{
		Messages: []aikit.Message{
			{Role: aikit.RoleUser, Content: []aikit.ContentPart{
				aikit.TextPart("before tool"), aikit.ImageURLPart("https://example.org/before.png"),
			}},
			{Role: aikit.RoleTool, Content: []aikit.ContentPart{tool}},
			{Role: aikit.RoleUser, Content: []aikit.ContentPart{
				aikit.ImageFileIDPart("file_after"), aikit.TextPart("after tool"),
			}},
			// A singleton text/JSON result uses the existing literal string output
			// constructor to match endpoints that compact one input_text to a string.
			{Role: aikit.RoleTool, Content: []aikit.ContentPart{
				aikit.ToolResultPart("call_text", "lookup", "literal text"),
				aikit.ToolResultPart("call_json", "lookup", `{"count":2}`),
			}},
		},
		ProviderOptions: map[string]any{"openai": ProviderOptions{
			ExplicitMessageType: true, SplitUserContentParts: true,
			ImageDetail: "auto", HistoryItemStatus: "completed",
		}},
	}
	encoded, warnings, err := encodeRequest("example-model", req, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("content was dropped: %#v", warnings)
	}
	want := `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"before tool"}]},
		{"type":"message","role":"user","content":[{"type":"input_image","image_url":"https://example.org/before.png","detail":"auto"}]},
		{"type":"function_call_output","call_id":"call_mixed","status":"completed","output":[
			{"type":"input_image","image_url":"data:image/png;base64,AQID","detail":"auto"},
			{"type":"input_text","text":"before JSON"},
			{"type":"input_text","text":"{\"answer\":42}"},
			{"type":"input_image","image_url":"data:image/jpeg;base64,BAUG","detail":"auto"},
			{"type":"input_text","text":"after image"}
		]},
		{"type":"message","role":"user","content":[{"type":"input_image","file_id":"file_after","detail":"auto"}]},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"after tool"}]},
		{"type":"function_call_output","call_id":"call_text","status":"completed","output":"literal text"},
		{"type":"function_call_output","call_id":"call_json","status":"completed","output":"{\"count\":2}"}
	]`
	assertResponsesInputJSON(t, encoded.Input, want)
	// Encoding options apply only to wire parts, not the caller's semantic data.
	if tool.ToolResultContent[0].Type != aikit.ToolResultContentTypeImage || len(req.Messages[0].Content) != 2 {
		t.Fatal("caller-owned content changed")
	}
}

func TestResponsesOrderedContentAllRoleBoundaryPermutations(t *testing.T) {
	parts := []aikit.ContentPart{
		aikit.TextPart("text"),
		aikit.ImageURLPart("https://example.org/image.png"),
		aikit.ToolResultPart("call_1", "lookup", "result"),
	}
	for _, order := range [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		var messages []aikit.Message
		for _, index := range order {
			role := aikit.RoleUser
			if parts[index].Type == aikit.ContentPartTypeToolResult {
				role = aikit.RoleTool
			}
			// Consecutive ordinary user parts may share one semantic message.
			if len(messages) != 0 && role == aikit.RoleUser && messages[len(messages)-1].Role == role {
				messages[len(messages)-1].Content = append(messages[len(messages)-1].Content, parts[index])
			} else {
				messages = append(messages, aikit.Message{Role: role, Content: []aikit.ContentPart{parts[index]}})
			}
		}
		encoded, warnings, err := encodeRequest("example-model", llm.Request{
			Messages:        messages,
			ProviderOptions: map[string]any{"openai": ProviderOptions{SplitUserContentParts: true}},
		}, false)
		if err != nil || len(warnings) != 0 || len(encoded.Input) != 3 {
			t.Fatalf("order %v: %#v, %#v, %v", order, encoded, warnings, err)
		}
		for position, index := range order {
			item := encoded.Input[position]
			switch index {
			case 0:
				if item.Role != "user" || len(item.Content) != 1 || item.Content[0].(inputTextPart).Text != "text" {
					t.Fatalf("order %v position %d: %#v", order, position, item)
				}
			case 1:
				if item.Role != "user" || len(item.Content) != 1 ||
					item.Content[0].(inputImageURLPart).ImageURL != "https://example.org/image.png" {
					t.Fatalf("order %v position %d: %#v", order, position, item)
				}
			case 2:
				if item.Type != "function_call_output" || item.CallID != "call_1" || item.Output != "result" {
					t.Fatalf("order %v position %d: %#v", order, position, item)
				}
			}
		}
	}
}

func TestResponsesToolResultImageDetailDefaultsUnchanged(t *testing.T) {
	part := aikit.ToolResultPart("call_1", "lookup", "")
	part.ToolResultContent = []aikit.ToolResultContent{
		aikit.TextToolResultContent(`{"literal":true}`),
		aikit.JSONToolResultContent(json.RawMessage(`{"json":true}`)),
		aikit.ImageToolResultContent([]byte{1}, ""),
	}
	req := llm.Request{Messages: []aikit.Message{{Role: aikit.RoleTool, Content: []aikit.ContentPart{part}}}}
	encoded, _, err := encodeRequest("example-model", req, false)
	if err != nil {
		t.Fatal(err)
	}
	assertResponsesInputJSON(
		t,
		encoded.Input,
		`[{"type":"function_call_output","call_id":"call_1","output":[{"type":"input_text","text":"{\"literal\":true}"},{"type":"input_text","text":"{\"json\":true}"},{"type":"input_image","image_url":"data:image/png;base64,AQ=="}]}]`,
	)
	for _, detail := range []string{"auto", "low", "high"} {
		req.ProviderOptions = map[string]any{"openai": ProviderOptions{ImageDetail: detail}}
		encoded, _, err = encodeRequest("example-model", req, false)
		if err != nil {
			t.Fatal(err)
		}
		output := encoded.Input[0].Output.([]inputPart)
		if output[2].(inputImageURLPart).Detail != detail {
			t.Fatalf("tool image detail: %#v", output)
		}
	}
}

func assertResponsesInputJSON(t *testing.T, input []inputItem, want string) {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal(raw, &gotValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("input wire mismatch:\ngot  %s\nwant %s", raw, want)
	}
}
