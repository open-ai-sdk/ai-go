package einoadapter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"github.com/open-ai-sdk/ai-go/aisdk"
)

// decodeHistory parses a client-shaped body into messages, so these tests exercise the
// same path a real request takes rather than hand-built structs.
func decodeHistory(t *testing.T, body string) []aisdk.UIMessage {
	t.Helper()
	var env struct {
		Messages []aisdk.UIMessage `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return env.Messages
}

func convert(t *testing.T, body string, opts ...ConvertOption) []*schema.AgenticMessage {
	t.Helper()
	msgs, err := ToAgenticMessages(decodeHistory(t, body), opts...)
	if err != nil {
		t.Fatalf("ToAgenticMessages: %v", err)
	}
	return msgs
}

func roles(msgs []*schema.AgenticMessage) []schema.AgenticRoleType {
	out := make([]schema.AgenticRoleType, len(msgs))
	for i, m := range msgs {
		out[i] = m.Role
	}
	return out
}

func blockTypes(m *schema.AgenticMessage) []schema.ContentBlockType {
	out := make([]schema.ContentBlockType, len(m.ContentBlocks))
	for i, b := range m.ContentBlocks {
		out[i] = b.Type
	}
	return out
}

// The single most important structural property: a tool result is a USER-role message.
// Under assistant role, agenticclaude's convertor returns
// "invalid content block type %q with assistant role" and the turn hard-fails inside the
// provider — on turn 2 of any tool conversation, which is a confusing place to discover it.
func TestToAgenticMessages_ToolResultsAreUserRole(t *testing.T) {
	msgs := convert(t, `{"messages":[
	  {"id":"m1","role":"user","parts":[{"type":"text","text":"add 2+2"}]},
	  {"id":"m2","role":"assistant","parts":[
	    {"type":"step-start"},
	    {"type":"tool-add","toolCallId":"c1","state":"output-available",
	     "input":{"a":2,"b":2},"output":"4"}
	  ]}
	]}`)

	if got := roles(msgs); len(got) != 3 ||
		got[0] != schema.AgenticRoleTypeUser ||
		got[1] != schema.AgenticRoleTypeAssistant ||
		got[2] != schema.AgenticRoleTypeUser {
		t.Fatalf("roles = %v, want [user assistant user]", got)
	}

	// And no assistant message may carry a FunctionToolResult.
	for i, m := range msgs {
		if m.Role != schema.AgenticRoleTypeAssistant {
			continue
		}
		for _, b := range m.ContentBlocks {
			if b.Type == schema.ContentBlockTypeFunctionToolResult {
				t.Errorf("message %d puts a FunctionToolResult under assistant role, "+
					"which the provider rejects", i)
			}
		}
	}

	result := msgs[2]
	if got := blockTypes(result); len(got) != 1 ||
		got[0] != schema.ContentBlockTypeFunctionToolResult {
		t.Fatalf("result blocks = %v", got)
	}
	ftr := result.ContentBlocks[0].FunctionToolResult
	if ftr.CallID != "c1" || ftr.Name != "add" {
		t.Errorf("result identity = %q/%q", ftr.CallID, ftr.Name)
	}
	if got := ftr.Content[0].Text.Text; got != "4" {
		t.Errorf("result text = %q, want 4", got)
	}
}

// One message per result, not one message with N results — agenticclaude merges adjacent
// tool results itself (convertor.go:88-105), so pre-merging would double the work and
// diverge from what its own tools node produces.
func TestToAgenticMessages_OneUserMessagePerToolResult(t *testing.T) {
	msgs := convert(t, `{"messages":[
	  {"id":"m1","role":"assistant","parts":[
	    {"type":"step-start"},
	    {"type":"tool-a","toolCallId":"c1","state":"output-available","input":{},"output":"1"},
	    {"type":"tool-b","toolCallId":"c2","state":"output-available","input":{},"output":"2"}
	  ]}
	]}`)

	if got := roles(msgs); len(got) != 3 {
		t.Fatalf("got %d messages %v, want 3 (assistant + 2 results)", len(got), got)
	}
	for _, m := range msgs[1:] {
		if len(m.ContentBlocks) != 1 {
			t.Errorf("a result message carries %d blocks; results must not be pre-merged",
				len(m.ContentBlocks))
		}
	}
}

// step-start groups parts into steps: one assistant message per step.
func TestToAgenticMessages_StepStartGroupingThreeSteps(t *testing.T) {
	msgs := convert(t, `{"messages":[
	  {"id":"m1","role":"assistant","parts":[
	    {"type":"step-start"},
	    {"type":"text","text":"one"},
	    {"type":"step-start"},
	    {"type":"text","text":"two"},
	    {"type":"step-start"},
	    {"type":"text","text":"three"}
	  ]}
	]}`)

	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3 — one per step", len(msgs))
	}
	for i, want := range []string{"one", "two", "three"} {
		if msgs[i].Role != schema.AgenticRoleTypeAssistant {
			t.Errorf("message %d role = %q", i, msgs[i].Role)
		}
		if got := msgs[i].ContentBlocks[0].AssistantGenText.Text; got != want {
			t.Errorf("step %d text = %q, want %q", i, got, want)
		}
	}
}

// A denied approval produces a synthetic execution-denied result. Without it the model
// sees an unanswered tool call and most providers hard-error on the next turn.
func TestToAgenticMessages_DeniedApprovalProducesExecutionDenied(t *testing.T) {
	msgs := convert(t, `{"messages":[
	  {"id":"m1","role":"assistant","parts":[
	    {"type":"step-start"},
	    {"type":"tool-sendEmail","toolCallId":"c1","state":"approval-responded",
	     "input":{"to":"a@b.c"},
	     "approval":{"id":"a1","approved":false,"reason":"user declined"}}
	  ]}
	]}`)

	if len(msgs) != 2 {
		t.Fatalf("got %d messages %v, want assistant + denial result", len(msgs), roles(msgs))
	}
	text := msgs[1].ContentBlocks[0].FunctionToolResult.Content[0].Text.Text
	decoded, ok := aisdk.DecodeToolResult(text)
	if !ok {
		t.Fatalf("denial result is not the documented encoding: %q", text)
	}
	if decoded.Type != aisdk.ToolResultExecutionDenied {
		t.Errorf("kind = %q, want execution-denied", decoded.Type)
	}
	if decoded.Reason != "user declined" {
		t.Errorf("reason = %q", decoded.Reason)
	}
}

// The other denial path: output-denied is a decision already carried out, replayed as
// history. It must render as error-text, not execution-denied.
func TestToAgenticMessages_OutputDeniedProducesErrorText(t *testing.T) {
	msgs := convert(t, `{"messages":[
	  {"id":"m1","role":"assistant","parts":[
	    {"type":"step-start"},
	    {"type":"tool-wipeDisk","toolCallId":"c1","state":"output-denied",
	     "input":{},"approval":{"id":"a1","approved":false,"reason":"nope"}}
	  ]}
	]}`)

	text := msgs[len(msgs)-1].ContentBlocks[0].FunctionToolResult.Content[0].Text.Text
	decoded, ok := aisdk.DecodeToolResult(text)
	if !ok {
		t.Fatalf("not the documented encoding: %q", text)
	}
	if decoded.Type != aisdk.ToolResultErrorText {
		t.Errorf("kind = %q, want error-text — the two denial paths must stay distinct",
			decoded.Type)
	}
	if decoded.Value != "nope" {
		t.Errorf("value = %q", decoded.Value)
	}
}

func TestToAgenticMessages_OutputDeniedWithoutReasonUsesDefault(t *testing.T) {
	msgs := convert(t, `{"messages":[
	  {"id":"m1","role":"assistant","parts":[
	    {"type":"tool-x","toolCallId":"c1","state":"output-denied",
	     "input":{},"approval":{"id":"a1","approved":false}}
	  ]}
	]}`)
	text := msgs[len(msgs)-1].ContentBlocks[0].FunctionToolResult.Content[0].Text.Text
	decoded, _ := aisdk.DecodeToolResult(text)
	if decoded.Value != aisdk.DefaultDenialText {
		t.Errorf("value = %q, want the documented default", decoded.Value)
	}
}

// A provider-executed tool's result belongs in the assistant content. Emitting it again as
// a tool message creates an orphaned output the provider cannot pair.
func TestToAgenticMessages_ProviderExecutedResultIsNotDuplicated(t *testing.T) {
	msgs := convert(t, `{"messages":[
	  {"id":"m1","role":"assistant","parts":[
	    {"type":"step-start"},
	    {"type":"tool-web_search","toolCallId":"c1","state":"output-available",
	     "providerExecuted":true,"input":{"q":"x"},"output":"3 results"}
	  ]}
	]}`)

	if len(msgs) != 1 {
		t.Fatalf("got %d messages %v, want 1 — the result is already in the assistant "+
			"content and must not be repeated", len(msgs), roles(msgs))
	}
	want := []schema.ContentBlockType{
		schema.ContentBlockTypeServerToolCall,
		schema.ContentBlockTypeServerToolResult,
	}
	got := blockTypes(msgs[0])
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("blocks = %v, want %v", got, want)
	}
}

// The exception: a provider-executed tool the user answered still needs its decision seen.
func TestToAgenticMessages_ProviderExecutedWithApprovalStillEmitsResult(t *testing.T) {
	msgs := convert(t, `{"messages":[
	  {"id":"m1","role":"assistant","parts":[
	    {"type":"tool-web_search","toolCallId":"c1","state":"approval-responded",
	     "providerExecuted":true,"input":{"q":"x"},
	     "approval":{"id":"a1","approved":false,"reason":"no"}}
	  ]}
	]}`)

	if len(msgs) != 2 {
		t.Fatalf("got %d messages %v, want assistant + denial", len(msgs), roles(msgs))
	}
}

func TestToAgenticMessages_UserPartsMapToInputBlocks(t *testing.T) {
	msgs := convert(t, `{"messages":[
	  {"id":"m1","role":"user","parts":[
	    {"type":"text","text":"look at these"},
	    {"type":"file","url":"https://e.com/a.png","mediaType":"image/png"},
	    {"type":"file","url":"https://e.com/b.pdf","mediaType":"application/pdf","filename":"b.pdf"},
	    {"type":"source-url","sourceId":"s1","url":"https://e.com"},
	    {"type":"data-plan","data":{"a":1}}
	  ]}
	]}`)

	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	// An image gets the image block; anything else is a file block. source-* and data-*
	// have no Eino equivalent and are dropped.
	want := []schema.ContentBlockType{
		schema.ContentBlockTypeUserInputText,
		schema.ContentBlockTypeUserInputImage,
		schema.ContentBlockTypeUserInputFile,
	}
	got := blockTypes(msgs[0])
	if len(got) != len(want) {
		t.Fatalf("blocks = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("block %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestToAgenticMessages_ReasoningCarriesSignature(t *testing.T) {
	msgs := convert(t, `{"messages":[
	  {"id":"m1","role":"assistant","parts":[
	    {"type":"reasoning","text":"thinking","state":"done",
	     "providerMetadata":{"signature":"sig-abc"}}
	  ]}
	]}`)

	r := msgs[0].ContentBlocks[0].Reasoning
	if r == nil {
		t.Fatalf("no reasoning block: %v", blockTypes(msgs[0]))
	}
	if r.Text != "thinking" || r.Signature != "sig-abc" {
		t.Errorf("reasoning = %+v; the signature must survive the round trip because "+
			"some models require it echoed back", r)
	}
}

// output-error falls back to rawInput, which is where the client stores an input it could
// not parse — the processor moves it there rather than keeping it under `input`.
func TestToAgenticMessages_OutputErrorFallsBackToRawInput(t *testing.T) {
	msgs := convert(t, `{"messages":[
	  {"id":"m1","role":"assistant","parts":[
	    {"type":"tool-getWeather","toolCallId":"c1","state":"output-error",
	     "rawInput":"{\"city\":","errorText":"bad input"}
	  ]}
	]}`)

	call := msgs[0].ContentBlocks[0].FunctionToolCall
	if call == nil {
		t.Fatalf("no tool call block: %v", blockTypes(msgs[0]))
	}
	if !strings.Contains(call.Arguments, `"city"`) {
		t.Errorf("arguments = %q, want the rawInput fallback", call.Arguments)
	}

	text := msgs[len(msgs)-1].ContentBlocks[0].FunctionToolResult.Content[0].Text.Text
	decoded, ok := aisdk.DecodeToolResult(text)
	if !ok || decoded.Value != "bad input" {
		t.Errorf("error result = %q", text)
	}
}

// input-streaming is a partial call the client rendered; the model never issued it in that
// form, so replaying it would invent a call that never happened.
func TestToAgenticMessages_InputStreamingIsDropped(t *testing.T) {
	msgs := convert(t, `{"messages":[
	  {"id":"m1","role":"assistant","parts":[
	    {"type":"text","text":"calling"},
	    {"type":"tool-x","toolCallId":"c1","state":"input-streaming","input":{"partial":true}}
	  ]}
	]}`)

	for _, m := range msgs {
		for _, b := range m.ContentBlocks {
			if b.Type == schema.ContentBlockTypeFunctionToolCall {
				t.Error("an input-streaming part produced a tool call")
			}
		}
	}
}

func TestToAgenticMessages_DataPartsDroppedUnlessConverterSupplied(t *testing.T) {
	body := `{"messages":[
	  {"id":"m1","role":"assistant","parts":[
	    {"type":"step-start"},
	    {"type":"data-plan","data":{"steps":2}}
	  ]}
	]}`

	if msgs := convert(t, body); len(msgs) != 0 {
		t.Errorf("data part produced %d messages without a converter", len(msgs))
	}

	msgs := convert(t, body, WithDataPartConverter(
		func(name string, data json.RawMessage) *schema.ContentBlock {
			return schema.NewContentBlock(&schema.AssistantGenText{Text: name})
		}))
	if len(msgs) != 1 || msgs[0].ContentBlocks[0].AssistantGenText.Text != "plan" {
		t.Errorf("supplied converter was not used: %v", msgs)
	}
}

func TestToAgenticMessages_SystemMessages(t *testing.T) {
	msgs := convert(t, `{"messages":[
	  {"id":"m0","role":"system","parts":[{"type":"text","text":"be terse"}]}
	]}`)
	if len(msgs) != 1 || msgs[0].Role != schema.AgenticRoleTypeSystem {
		t.Fatalf("roles = %v", roles(msgs))
	}
}

func TestToAgenticMessages_EmptyAndTextOnly(t *testing.T) {
	if msgs := convert(t, `{"messages":[]}`); len(msgs) != 0 {
		t.Errorf("empty history produced %d messages", len(msgs))
	}
	// A message whose parts produce no blocks must not become an empty message — a
	// provider rejects content-less messages.
	if msgs := convert(t, `{"messages":[{"id":"m1","role":"user","parts":[]}]}`); len(msgs) != 0 {
		t.Errorf("part-less message produced %d messages", len(msgs))
	}
}

// The counts the Phase 01 test used to assert, re-asserted against the new target as that
// note promised: a user message with text + file → 2 blocks; an assistant tool call with a
// result → tool-call + text in the assistant message, plus one result message.
func TestToAgenticMessages_FanOutCountsFromTheRetargetedEnvelopeTest(t *testing.T) {
	msgs := convert(t, `{"messages":[
	  {"id":"m1","role":"user","parts":[
	    {"type":"text","text":"What is 2+2?"},
	    {"type":"file","url":"https://e.com/img.png","mediaType":"image/png"}
	  ]},
	  {"id":"m2","role":"assistant","parts":[
	    {"type":"step-start"},
	    {"type":"tool-add","toolCallId":"tc-rt-1","state":"output-available",
	     "input":{"a":2,"b":2},"output":"4"},
	    {"type":"text","text":"The answer is 4."}
	  ]}
	]}`)

	if len(msgs[0].ContentBlocks) != 2 {
		t.Errorf("user message blocks = %d, want 2", len(msgs[0].ContentBlocks))
	}
	if len(msgs[1].ContentBlocks) != 2 {
		t.Errorf("assistant message blocks = %d, want 2 (tool-call + text)",
			len(msgs[1].ContentBlocks))
	}
	if len(msgs) != 3 {
		t.Errorf("total messages = %d, want 3", len(msgs))
	}
}
