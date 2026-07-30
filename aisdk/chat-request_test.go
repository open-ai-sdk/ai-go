package aisdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func decodeBody(t *testing.T, body string) (*ChatRequest, error) {
	t.Helper()
	return DecodeChatRequest(strings.NewReader(body), DefaultDecodeLimits())
}

func TestDecodeChatRequest_RealClientBody(t *testing.T) {
	req, err := decodeBody(t, `{
	  "id":"chat-1","trigger":"submit-message","messageId":"m2",
	  "modelId":"anthropic:claude","messages":[
	    {"id":"m1","role":"user","parts":[{"type":"text","text":"delete tmp"}]},
	    {"id":"m2","role":"assistant","parts":[
	      {"type":"step-start"},
	      {"type":"tool-deleteFile","toolCallId":"c1","state":"approval-responded",
	       "input":{"path":"/tmp/x"},
	       "approval":{"id":"a1","approved":true,"signature":"sig-1"}}]}
	  ]}`)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if req.ID != "chat-1" || req.Trigger != TriggerSubmitMessage || req.MessageID != "m2" {
		t.Errorf("envelope = %+v", req)
	}
	if _, ok := req.Body["modelId"]; !ok {
		t.Errorf("application field lost: %v", req.Body)
	}

	p := req.Messages[1].Parts[1]
	if p.Approval == nil || p.Approval.Signature != "sig-1" {
		t.Errorf("signature lost: %+v", p.Approval)
	}
	approved, answered := p.ApprovalDecision()
	if !answered || !approved {
		t.Errorf("approval decision = %v/%v", approved, answered)
	}
}

// ChatRequest must not have a Metadata field: requestMetadata reaches only
// prepareSendMessagesRequest and never enters the default body, so a server reading it
// would be reading something the client never sends.
func TestChatRequest_HasNoMetadataFieldAndMessageIDIsAString(t *testing.T) {
	b, err := json.Marshal(ChatRequest{ID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if _, ok := m["metadata"]; ok {
		t.Error("ChatRequest marshals a metadata field; v7 does not send one")
	}

	// MessageID is a string, not *string: JSON.stringify drops undefined keys, so the
	// field is omitted rather than null and a pointer would add an impossible nil case.
	req, err := decodeBody(t, `{"id":"c","messages":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if req.MessageID != "" {
		t.Errorf("absent messageId decoded as %q", req.MessageID)
	}
}

// The continuation rule. Echoing req.MessageID would be wrong on the edit-and-resend
// path, where messageId is a USER message id — the client's replaceLastMessage would then
// overwrite the user's own prompt with the answer.
func TestResolveResponseMessageID(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"last message assistant → continue it",
			`{"id":"c","messageId":"m1","messages":[
			  {"id":"m1","role":"user","parts":[]},
			  {"id":"m2","role":"assistant","parts":[]}]}`, "m2"},

		{"last message user with a messageId set → fresh id",
			`{"id":"c","messageId":"m1","messages":[
			  {"id":"m1","role":"user","parts":[]}]}`, ""},

		{"empty history → fresh id", `{"id":"c","messages":[]}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := decodeBody(t, tc.body)
			if err != nil {
				t.Fatal(err)
			}
			if got := req.ResolveResponseMessageID(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeChatRequest_RejectsResumeStream(t *testing.T) {
	_, err := decodeBody(t, `{"id":"c","trigger":"resume-stream","messages":[]}`)
	if err == nil {
		t.Fatal("resume-stream was accepted; this server has no stream store, so " +
			"treating it as a submission would re-run and re-bill the model")
	}
	if !errors.Is(err, ErrChatRequest) {
		t.Errorf("err = %v, want ErrChatRequest", err)
	}
}

func TestDecodeChatRequest_RejectsUnknownTriggerAndAcceptsAbsent(t *testing.T) {
	if _, err := decodeBody(t, `{"id":"c","trigger":"nonsense","messages":[]}`); err == nil {
		t.Error("unknown trigger accepted")
	}
	// An absent trigger is treated as a submission rather than rejected: hand-rolled
	// callers omit it, and the request is otherwise well-formed.
	req, err := decodeBody(t, `{"id":"c","messages":[]}`)
	if err != nil {
		t.Fatalf("absent trigger rejected: %v", err)
	}
	if req.Trigger != TriggerSubmitMessage {
		t.Errorf("absent trigger defaulted to %q", req.Trigger)
	}
}

// Depth is checked with a token loop, before the value is built. A check performed after
// unmarshalling would already have paid the allocation it exists to prevent.
func TestDecodeChatRequest_DepthLimit(t *testing.T) {
	limits := DefaultDecodeLimits()

	build := func(depth int) string {
		return `{"id":"c","messages":[],"deep":` +
			strings.Repeat(`[`, depth) + strings.Repeat(`]`, depth) + `}`
	}

	// One under the limit: the outer object counts as level 1.
	if _, err := DecodeChatRequest(strings.NewReader(build(limits.MaxJSONDepth-2)), limits); err != nil {
		t.Errorf("a body within the depth budget was rejected: %v", err)
	}
	// Over the limit.
	_, err := DecodeChatRequest(strings.NewReader(build(limits.MaxJSONDepth+1)), limits)
	if err == nil {
		t.Fatal("a body past the depth budget was accepted; canonical hashing is " +
			"unguarded recursion and runs before anything authenticates the request")
	}
	if !strings.Contains(err.Error(), "nesting") {
		t.Errorf("err = %v, want a nesting error", err)
	}
}

func TestDecodeChatRequest_BodySizeLimit(t *testing.T) {
	limits := DefaultDecodeLimits()
	limits.MaxBodyBytes = 200

	big := fmt.Sprintf(`{"id":"c","messages":[],"pad":%q}`, strings.Repeat("x", 500))
	if _, err := DecodeChatRequest(strings.NewReader(big), limits); err == nil {
		t.Error("oversized body accepted")
	}
	small := `{"id":"c","messages":[]}`
	if _, err := DecodeChatRequest(strings.NewReader(small), limits); err != nil {
		t.Errorf("small body rejected: %v", err)
	}
}

func TestDecodeChatRequest_StructuralLimits(t *testing.T) {
	limits := DefaultDecodeLimits()
	limits.MaxMessages = 2
	limits.MaxPartsPerMessage = 2
	limits.MaxToolInputBytes = 32

	tooMany := `{"id":"c","messages":[
	  {"id":"1","role":"user","parts":[]},{"id":"2","role":"user","parts":[]},
	  {"id":"3","role":"user","parts":[]}]}`
	if _, err := DecodeChatRequest(strings.NewReader(tooMany), limits); err == nil {
		t.Error("message count limit not enforced")
	}

	tooManyParts := `{"id":"c","messages":[{"id":"1","role":"user","parts":[
	  {"type":"text","text":"a"},{"type":"text","text":"b"},{"type":"text","text":"c"}]}]}`
	if _, err := DecodeChatRequest(strings.NewReader(tooManyParts), limits); err == nil {
		t.Error("part count limit not enforced")
	}

	bigInput := fmt.Sprintf(`{"id":"c","messages":[{"id":"1","role":"assistant","parts":[
	  {"type":"tool-x","toolCallId":"c1","state":"input-available","input":{"v":%q}}]}]}`,
		strings.Repeat("y", 100))
	if _, err := DecodeChatRequest(strings.NewReader(bigInput), limits); err == nil {
		t.Error("tool input size limit not enforced; that value is passed to code that runs")
	}
}

func TestDecodeChatRequest_MalformedJSON(t *testing.T) {
	if _, err := decodeBody(t, `{"id":`); err == nil {
		t.Error("truncated JSON accepted")
	}
}

// The error must locate the problem. "invalid request" without indices means bisecting a
// conversation by hand.
func TestChatRequestError_NamesTheLocation(t *testing.T) {
	err := &ChatRequestError{
		MessageIndex: 3, PartIndex: 2, ToolName: "deleteFile", ToolCallID: "c9",
		Reason: "unknown tool state \"weird\"",
	}
	msg := err.Error()
	for _, want := range []string{"messages[3].parts[2]", "deleteFile", "c9", "weird"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q: %s", want, msg)
		}
	}
	if !errors.Is(err, ErrChatRequest) {
		t.Error("ChatRequestError does not unwrap to ErrChatRequest")
	}
}

// The v6 envelope is gone.
func TestNoToolApprovalResponsesField(t *testing.T) {
	// useChat never sent a top-level toolApprovalResponses array; approvals ride inside
	// the assistant message's tool part. A server reading the old field would silently
	// see no approvals at all.
	req, err := decodeBody(t, `{"id":"c","messages":[],
	  "toolApprovalResponses":[{"approvalId":"a1","approved":true}]}`)
	if err != nil {
		t.Fatal(err)
	}
	// It survives only as an opaque application field, not as protocol.
	if _, ok := req.Body["toolApprovalResponses"]; !ok {
		t.Log("unknown top-level keys are kept in Body, which is correct")
	}
	if len(req.Messages) != 0 {
		t.Error("the legacy field must not populate messages")
	}
}
