package aisdk

import (
	"encoding/json"
	"testing"
)

// TestUIMessage_ParsesRealClientBody decodes a body shaped the way useChat POSTs it:
// a full history with a text part, a completed tool call, and an approval the user
// answered. This is the inbound contract the adapter's conversion depends on.
func TestUIMessage_ParsesRealClientBody(t *testing.T) {
	body := []byte(`{
	  "id": "chat-1",
	  "messages": [
	    {"id":"m1","role":"user","parts":[{"type":"text","text":"delete it"}]},
	    {"id":"m2","role":"assistant","parts":[
	      {"type":"step-start"},
	      {"type":"reasoning","text":"considering","state":"done"},
	      {"type":"text","text":"I will delete it.","state":"done"},
	      {"type":"tool-deleteFile","toolCallId":"c1","state":"output-available",
	       "input":{"path":"/tmp/x"},"output":"deleted","providerExecuted":false},
	      {"type":"tool-sendEmail","toolCallId":"c2","state":"approval-responded",
	       "input":{"to":"a@b.c"},
	       "approval":{"id":"a1","approved":true,"signature":"sig-xyz"}},
	      {"type":"tool-wipeDisk","toolCallId":"c3","state":"output-denied",
	       "input":{},"approval":{"id":"a2","approved":false,"reason":"nope"}},
	      {"type":"dynamic-tool","toolName":"lookup","toolCallId":"c4",
	       "state":"input-available","input":{"q":"x"}},
	      {"type":"data-plan","id":"d1","data":{"steps":2}}
	    ]}
	  ]
	}`)

	var env struct {
		ID       string      `json:"id"`
		Messages []UIMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.ID != "chat-1" || len(env.Messages) != 2 {
		t.Fatalf("envelope: id=%q messages=%d", env.ID, len(env.Messages))
	}

	user := env.Messages[0]
	if user.Role != UIRoleUser || user.Parts[0].Text != "delete it" {
		t.Errorf("user message: %+v", user)
	}

	asst := env.Messages[1]
	if asst.Role != UIRoleAssistant {
		t.Fatalf("assistant role = %q", asst.Role)
	}
	if len(asst.Parts) != 8 {
		t.Fatalf("assistant parts = %d, want 8", len(asst.Parts))
	}

	// The state key is shared between text/reasoning and tool parts. A non-tool part
	// must not report a tool state, or a "done" text part would look like a tool.
	reasoning := asst.Parts[1]
	if reasoning.State != "done" {
		t.Errorf("reasoning state = %q, want done", reasoning.State)
	}
	if got := reasoning.ToolStateOf(); got != "" {
		t.Errorf("reasoning part reported tool state %q; want empty", got)
	}

	// Typed tool part: name comes from the type prefix.
	del := asst.Parts[3]
	if !del.IsToolPart() {
		t.Error("tool-deleteFile not recognised as a tool part")
	}
	if got := del.ToolNameOf(); got != "deleteFile" {
		t.Errorf("tool name = %q, want deleteFile", got)
	}
	if got := del.ToolStateOf(); got != UIToolOutputAvailable {
		t.Errorf("tool state = %q, want output-available", got)
	}
	if del.ProviderExecuted == nil || *del.ProviderExecuted {
		t.Errorf("providerExecuted = %v, want explicit false", del.ProviderExecuted)
	}
	if string(del.Output) != `"deleted"` {
		t.Errorf("output = %s", del.Output)
	}

	// Approved approval.
	email := asst.Parts[4]
	approved, answered := email.ApprovalDecision()
	if !answered || !approved {
		t.Errorf("sendEmail: approved=%v answered=%v, want true/true", approved, answered)
	}
	if email.Approval.Signature != "sig-xyz" {
		t.Errorf("signature = %q", email.Approval.Signature)
	}

	// Denied approval.
	wipe := asst.Parts[5]
	approved, answered = wipe.ApprovalDecision()
	if !answered || approved {
		t.Errorf("wipeDisk: approved=%v answered=%v, want false/true", approved, answered)
	}
	if got := wipe.ToolStateOf(); got != UIToolOutputDenied {
		t.Errorf("wipeDisk state = %q, want output-denied", got)
	}

	// Dynamic tool: name comes from ToolName, not the type.
	dyn := asst.Parts[6]
	if !dyn.IsToolPart() || dyn.ToolNameOf() != "lookup" {
		t.Errorf("dynamic tool: isTool=%v name=%q", dyn.IsToolPart(), dyn.ToolNameOf())
	}

	// Data part.
	data := asst.Parts[7]
	if !data.IsDataPart() || data.DataNameOf() != "plan" {
		t.Errorf("data part: isData=%v name=%q", data.IsDataPart(), data.DataNameOf())
	}
}

// TestUIToolApproval_UnansweredIsDistinctFromDenied is why Approved is a pointer.
// A pending request and a denial must not look the same, or the gate would execute a
// tool nobody approved — or refuse one nobody denied.
func TestUIToolApproval_UnansweredIsDistinctFromDenied(t *testing.T) {
	var pending, denied UIMessagePart
	if err := json.Unmarshal([]byte(
		`{"type":"tool-x","toolCallId":"c1","state":"approval-requested",
		  "approval":{"id":"a1"}}`), &pending); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(
		`{"type":"tool-x","toolCallId":"c1","state":"output-denied",
		  "approval":{"id":"a1","approved":false}}`), &denied); err != nil {
		t.Fatal(err)
	}

	if _, answered := pending.ApprovalDecision(); answered {
		t.Error("a pending approval reported as answered")
	}
	approved, answered := denied.ApprovalDecision()
	if !answered || approved {
		t.Errorf("denial: approved=%v answered=%v, want false/true", approved, answered)
	}
}

// TestUIMessagePart_StateTagIsSingle guards the JSON-tag collision directly: two Go
// fields tagged `json:"state"` make encoding/json drop both silently, so a tool state
// would arrive empty and every tool part would look unstarted.
func TestUIMessagePart_StateTagIsSingle(t *testing.T) {
	var p UIMessagePart
	if err := json.Unmarshal([]byte(
		`{"type":"tool-x","toolCallId":"c1","state":"input-available"}`), &p); err != nil {
		t.Fatal(err)
	}
	if p.State != "input-available" {
		t.Fatalf("state did not decode (got %q) — check for duplicate json:\"state\" tags", p.State)
	}
	if p.ToolStateOf() != UIToolInputAvailable {
		t.Errorf("ToolStateOf = %q, want input-available", p.ToolStateOf())
	}
}

// TestUIMessage_RoundTrip — re-encoding must not invent fields. An absent optional that
// comes back as null or false changes meaning on the next turn.
func TestUIMessage_RoundTrip(t *testing.T) {
	in := []byte(`{"id":"m1","role":"user","parts":[{"type":"text","text":"hi"}]}`)
	var msg UIMessage
	if err := json.Unmarshal(in, &msg); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 { // id, role, parts — metadata absent
		t.Errorf("round trip changed the key set: %v", got)
	}
	parts, _ := got["parts"].([]any)
	if len(parts) != 1 {
		t.Fatalf("parts = %v", parts)
	}
	part, _ := parts[0].(map[string]any)
	if len(part) != 2 { // type, text only
		t.Errorf("part gained fields on re-encode: %v", part)
	}
}
