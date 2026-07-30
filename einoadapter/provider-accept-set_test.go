package einoadapter

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

// Guards the conversion against what a provider will actually accept per role.
//
// The plan's criterion was "asserted by round-tripping a two-tool turn through
// agenticclaude's convertor without error". That cannot be written as stated:
// `toAnthropicMessages` and both role converters are UNEXPORTED
// (eino-ext/components/model/agenticclaude/convertor.go:29,173,211), so no external
// package can call them. Importing the provider would also add 78 transitive modules
// (grpc, protobuf, the Anthropic SDK) to a library whose selling point is that `aisdk`
// has no dependencies.
//
// What is asserted instead: the accept-sets transcribed from those switch statements,
// which is the thing the round-trip would have been checking. Each is a closed switch
// with a `default: fmt.Errorf("invalid content block type %q with <role> role")`, so a
// block outside the set is a hard provider error, not a degraded render — and it surfaces
// on turn 2 of a tool conversation, which is a confusing place to find it.
//
// The gap this leaves: if a provider CHANGES its accept-set, this test still passes. The
// honest way to close that is a separate module (the `compat-test` pattern) that
// constructs the model with a dummy key and calls Generate — conversion failures return
// before any network I/O, so a well-formed message fails with an auth error while a
// malformed one fails with the conversion error. Recorded in the phase report as the
// follow-up rather than done here, because it needs its own go.mod.

// agenticclaude/convertor.go:211-230 — assistant role.
var assistantAcceptSet = map[schema.ContentBlockType]bool{
	schema.ContentBlockTypeAssistantGenText: true,
	schema.ContentBlockTypeReasoning:        true,
	schema.ContentBlockTypeFunctionToolCall: true,
	schema.ContentBlockTypeServerToolCall:   true,
	schema.ContentBlockTypeServerToolResult: true,
}

// agenticclaude/convertor.go:173-192 — user role.
var userAcceptSet = map[schema.ContentBlockType]bool{
	schema.ContentBlockTypeUserInputText:      true,
	schema.ContentBlockTypeUserInputImage:     true,
	schema.ContentBlockTypeUserInputFile:      true,
	schema.ContentBlockTypeFunctionToolResult: true,
	schema.ContentBlockTypeToolSearchResult:   true,
}

// agenticclaude/convertor.go:154-171 — system role accepts text only.
var systemAcceptSet = map[schema.ContentBlockType]bool{
	schema.ContentBlockTypeUserInputText: true,
}

// assertProviderWouldAccept checks every block against its message's role.
func assertProviderWouldAccept(t *testing.T, msgs []*schema.AgenticMessage) {
	t.Helper()
	for i, m := range msgs {
		var set map[schema.ContentBlockType]bool
		switch m.Role {
		case schema.AgenticRoleTypeAssistant:
			set = assistantAcceptSet
		case schema.AgenticRoleTypeUser:
			set = userAcceptSet
		case schema.AgenticRoleTypeSystem:
			set = systemAcceptSet
		default:
			t.Errorf("message %d has role %q, which is not one of system|user|assistant",
				i, m.Role)
			continue
		}
		if len(m.ContentBlocks) == 0 {
			t.Errorf("message %d (%s) has no content blocks; providers reject empty messages",
				i, m.Role)
		}
		for j, b := range m.ContentBlocks {
			if !set[b.Type] {
				t.Errorf("message %d block %d: %q is not accepted with %s role — the "+
					"provider returns \"invalid content block type %%q with %s role\"",
					i, j, b.Type, m.Role, m.Role)
			}
		}
	}
}

// The two-tool turn the plan names, checked against both role accept-sets.
func TestConversion_TwoToolTurnMatchesProviderAcceptSets(t *testing.T) {
	msgs := convert(t, `{"messages":[
	  {"id":"m0","role":"system","parts":[{"type":"text","text":"be terse"}]},
	  {"id":"m1","role":"user","parts":[
	    {"type":"text","text":"weather in Hanoi and Tokyo?"},
	    {"type":"file","url":"https://e.com/a.png","mediaType":"image/png"}
	  ]},
	  {"id":"m2","role":"assistant","parts":[
	    {"type":"step-start"},
	    {"type":"reasoning","text":"two lookups","providerMetadata":{"signature":"s1"}},
	    {"type":"text","text":"Checking both."},
	    {"type":"tool-getWeather","toolCallId":"c1","state":"output-available",
	     "input":{"city":"Hanoi"},"output":"31C"},
	    {"type":"tool-getWeather","toolCallId":"c2","state":"output-available",
	     "input":{"city":"Tokyo"},"output":"18C"}
	  ]}
	]}`)

	assertProviderWouldAccept(t, msgs)

	// And specifically: exactly two user-role result messages, one per tool.
	var resultMsgs int
	for _, m := range msgs {
		if m.Role != schema.AgenticRoleTypeUser {
			continue
		}
		for _, b := range m.ContentBlocks {
			if b.Type == schema.ContentBlockTypeFunctionToolResult {
				resultMsgs++
			}
		}
	}
	if resultMsgs != 2 {
		t.Errorf("got %d tool results, want 2 (one message each)", resultMsgs)
	}
}

// Every shape the conversion can produce must satisfy the accept-sets, not just the happy
// path — the denial and provider-executed paths build different blocks.
func TestConversion_AllShapesMatchProviderAcceptSets(t *testing.T) {
	cases := map[string]string{
		"denied approval": `{"messages":[{"id":"m","role":"assistant","parts":[
		  {"type":"tool-x","toolCallId":"c1","state":"approval-responded","input":{},
		   "approval":{"id":"a1","approved":false,"reason":"no"}}]}]}`,

		"output denied": `{"messages":[{"id":"m","role":"assistant","parts":[
		  {"type":"tool-x","toolCallId":"c1","state":"output-denied","input":{},
		   "approval":{"id":"a1","approved":false}}]}]}`,

		"output error": `{"messages":[{"id":"m","role":"assistant","parts":[
		  {"type":"tool-x","toolCallId":"c1","state":"output-error",
		   "rawInput":"{","errorText":"boom"}]}]}`,

		"provider executed": `{"messages":[{"id":"m","role":"assistant","parts":[
		  {"type":"tool-web_search","toolCallId":"c1","state":"output-available",
		   "providerExecuted":true,"input":{"q":"x"},"output":"hits"}]}]}`,

		"approval requested": `{"messages":[{"id":"m","role":"assistant","parts":[
		  {"type":"tool-x","toolCallId":"c1","state":"approval-requested","input":{},
		   "approval":{"id":"a1","signature":"sig"}}]}]}`,

		"multi step": `{"messages":[{"id":"m","role":"assistant","parts":[
		  {"type":"step-start"},{"type":"text","text":"a"},
		  {"type":"step-start"},
		  {"type":"tool-x","toolCallId":"c1","state":"output-available","input":{},"output":"o"}]}]}`,

		"user with file and image": `{"messages":[{"id":"m","role":"user","parts":[
		  {"type":"text","text":"t"},
		  {"type":"file","url":"u","mediaType":"image/png"},
		  {"type":"file","url":"u2","mediaType":"application/pdf"}]}]}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			assertProviderWouldAccept(t, convert(t, body))
		})
	}
}

// The specific regression the accept-set exists to catch: a FunctionToolResult under
// assistant role. This asserts the guard itself works, so the tests above are meaningful.
func TestProviderAcceptSet_RejectsToolResultUnderAssistantRole(t *testing.T) {
	bad := []*schema.AgenticMessage{{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.FunctionToolResult{CallID: "c1", Name: "x"}),
		},
	}}

	fake := &testing.T{}
	assertProviderWouldAccept(fake, bad)
	if !fake.Failed() {
		t.Error("the accept-set guard did not reject a FunctionToolResult under " +
			"assistant role, so it would not catch the bug it exists for")
	}
}

// And that an unrepresentable role is caught.
func TestProviderAcceptSet_RejectsToolRole(t *testing.T) {
	bad := []*schema.AgenticMessage{{
		Role:          schema.AgenticRoleType("tool"),
		ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.UserInputText{Text: "x"})},
	}}
	fake := &testing.T{}
	assertProviderWouldAccept(fake, bad)
	if !fake.Failed() {
		t.Error("a `tool` role was accepted; AgenticRoleType has no such value")
	}
}

// Nothing the converter emits may be an empty message.
func TestConversion_NeverEmitsEmptyMessages(t *testing.T) {
	msgs := convert(t, `{"messages":[
	  {"id":"m1","role":"user","parts":[{"type":"source-url","sourceId":"s","url":"u"}]},
	  {"id":"m2","role":"assistant","parts":[{"type":"step-start"},{"type":"step-start"}]}
	]}`)
	for i, m := range msgs {
		if len(m.ContentBlocks) == 0 {
			t.Errorf("message %d is empty; a provider rejects content-less messages", i)
		}
	}
}
