package agui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// clientToolStream is the wire shape a client-executed tool produces: the call
// is streamed in full, then the run suspends.
func clientToolStream(t *testing.T, args string) (string, map[string]any) {
	t.Helper()
	stream := runProtocol(t, sequence(
		aikit.StepEvent{
			Type: aikit.StepEventToolCallStart, ToolCallID: "call_1",
			ToolCallName: "renderChart", ToolCallArgsDelta: args,
		},
		aikit.StepEvent{Type: aikit.StepEventToolCallReady, ToolCallID: "call_1"},
		aikit.StepEvent{
			Type: aikit.StepEventClientToolRequest, ToolCallID: "call_1",
			ToolCallName: "renderChart", ToolCallArgsDelta: args,
		},
		aikit.StepEvent{
			Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonToolCalls,
		},
		aikit.StepEvent{Type: aikit.StepEventDone},
	))
	return stream, lastEventPayload(t, stream)
}

func lastEventPayload(t *testing.T, stream string) map[string]any {
	t.Helper()
	var last map[string]any
	for _, line := range strings.Split(stream, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := map[string]any{}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &payload); err != nil {
			t.Fatalf("frame is not JSON: %v", err)
		}
		last = payload
	}
	if last == nil {
		t.Fatal("stream carried no frames")
	}
	return last
}

func soleClientInterrupt(t *testing.T, finished map[string]any) map[string]any {
	t.Helper()
	outcome, ok := finished["outcome"].(map[string]any)
	if !ok {
		t.Fatalf("RUN_FINISHED carries no outcome: %#v", finished)
	}
	if outcome["type"] != "interrupt" {
		t.Fatalf("outcome type = %v, want interrupt", outcome["type"])
	}
	interrupts, ok := outcome["interrupts"].([]any)
	if !ok || len(interrupts) != 1 {
		t.Fatalf("interrupts = %#v, want exactly one", outcome["interrupts"])
	}
	entry, ok := interrupts[0].(map[string]any)
	if !ok {
		t.Fatalf("interrupt is not an object: %#v", interrupts[0])
	}
	return entry
}

// The client routes a frontend tool only through its pre-binding metadata path,
// so these exact spellings are load-bearing.
func TestClientToolInterruptUsesLegacyMetadataShape(t *testing.T) {
	_, finished := clientToolStream(t, `{"series":[1,2]}`)
	if finished["type"] != "RUN_FINISHED" {
		t.Fatalf("last frame = %v, want RUN_FINISHED", finished["type"])
	}

	entry := soleClientInterrupt(t, finished)
	if entry["reason"] != "tanstack:client_tool_execution" {
		t.Errorf("reason = %v", entry["reason"])
	}
	if entry["toolCallId"] != "call_1" {
		t.Errorf("toolCallId = %v", entry["toolCallId"])
	}
	if entry["id"] != "client_tool_call_1" {
		t.Errorf("id = %v", entry["id"])
	}

	metadata, ok := entry["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata = %#v", entry["metadata"])
	}
	if metadata["kind"] != "client_tool" {
		t.Errorf("metadata.kind = %v, want the underscore spelling", metadata["kind"])
	}
	if metadata["toolName"] != "renderChart" {
		t.Errorf("metadata.toolName = %v", metadata["toolName"])
	}
	if _, present := metadata["input"]; !present {
		t.Error("metadata.input key must be present; the client tests for presence")
	}
	// A binding is honored only with schema digests this side cannot compute,
	// and a failing one routes to this same path — so none is emitted.
	if _, found := metadata[interruptBindingKey]; found {
		t.Error("a client tool must carry no interrupt binding")
	}
}

// Unparseable arguments must still yield the key, as JSON null.
func TestClientToolInterruptKeepsInputKeyOnInvalidArgs(t *testing.T) {
	_, finished := clientToolStream(t, `{"series":`)
	metadata := soleClientInterrupt(t, finished)["metadata"].(map[string]any)
	value, present := metadata["input"]
	if !present {
		t.Fatal("metadata.input must exist even when arguments do not parse")
	}
	if value != nil {
		t.Errorf("metadata.input = %#v, want null", value)
	}
}

// The call must be fully streamed before the snapshot the client rebuilds from.
func TestClientToolInterruptOrdering(t *testing.T) {
	stream, _ := clientToolStream(t, `{}`)
	types := eventTypes(t, stream)

	indexOf := func(want string) int {
		for i, name := range types {
			if name == want {
				return i
			}
		}
		t.Fatalf("%s missing from %v", want, types)
		return -1
	}

	if indexOf("TOOL_CALL_END") > indexOf("MESSAGES_SNAPSHOT") {
		t.Errorf("TOOL_CALL_END must precede MESSAGES_SNAPSHOT: %v", types)
	}
	if indexOf("MESSAGES_SNAPSHOT") > indexOf("RUN_FINISHED") {
		t.Errorf("MESSAGES_SNAPSHOT must precede RUN_FINISHED: %v", types)
	}
	for _, name := range types {
		if name == "RUN_ERROR" {
			t.Fatalf("a suspended run must not emit RUN_ERROR: %v", types)
		}
	}
}

// A client tool and an approval in one turn both reach the client.
func TestClientToolAndApprovalInterruptsCoexist(t *testing.T) {
	stream := runProtocol(t, sequence(
		aikit.StepEvent{
			Type: aikit.StepEventToolCallStart, ToolCallID: "call_1",
			ToolCallName: "renderChart", ToolCallArgsDelta: `{}`,
		},
		aikit.StepEvent{Type: aikit.StepEventToolCallReady, ToolCallID: "call_1"},
		aikit.StepEvent{
			Type: aikit.StepEventClientToolRequest, ToolCallID: "call_1",
			ToolCallName: "renderChart", ToolCallArgsDelta: `{}`,
		},
		aikit.StepEvent{
			Type: aikit.StepEventToolApprovalRequest, ToolCallID: "call_2",
			ToolCallName: "sendEmail", ApprovalID: "approval_1", ToolCallArgsDelta: `{}`,
		},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonToolCalls},
		aikit.StepEvent{Type: aikit.StepEventDone},
	))

	outcome := lastEventPayload(t, stream)["outcome"].(map[string]any)
	interrupts, ok := outcome["interrupts"].([]any)
	if !ok || len(interrupts) != 2 {
		t.Fatalf("interrupts = %#v, want two", outcome["interrupts"])
	}
	reasons := map[string]bool{}
	for _, raw := range interrupts {
		reasons[raw.(map[string]any)["reason"].(string)] = true
	}
	if !reasons["tanstack:client_tool_execution"] || !reasons[interruptReasonToolCall] {
		t.Errorf("reasons = %#v, want one of each kind", reasons)
	}
}
