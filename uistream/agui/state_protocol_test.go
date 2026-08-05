package agui

import (
	"bytes"
	"context"
	"encoding/json"
	"iter"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream"
)

func TestStateSnapshotEventPreservesBytes(t *testing.T) {
	document := `{"zebra":1,"alpha":{"nested":[1,2]},"count":1.50}`
	stream := runProtocol(t, sequence(
		aikit.StepEvent{Type: aikit.StepEventStateSnapshot, State: json.RawMessage(document)},
		aikit.StepEvent{Type: aikit.StepEventDone},
	))

	frame := frameOfType(t, stream, "STATE_SNAPSHOT")
	// Key order and the trailing zero survive only if the bytes are forwarded
	// rather than decoded and re-marshalled.
	if !strings.Contains(frame, `"snapshot":`+document) {
		t.Errorf("snapshot was reserialized: %s", frame)
	}
}

func TestStateDeltaEventPreservesBytes(t *testing.T) {
	patch := `[{"op":"replace","path":"/status","value":"done"},{"op":"remove","path":"/draft"}]`
	stream := runProtocol(t, sequence(
		aikit.StepEvent{Type: aikit.StepEventStateDelta, StatePatch: json.RawMessage(patch)},
		aikit.StepEvent{Type: aikit.StepEventDone},
	))

	frame := frameOfType(t, stream, "STATE_DELTA")
	if !strings.Contains(frame, `"delta":`+patch) {
		t.Errorf("delta was reserialized: %s", frame)
	}
}

func TestStateEventsKeepStreamOrder(t *testing.T) {
	stream := runProtocol(t, sequence(
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "before"},
		aikit.StepEvent{Type: aikit.StepEventStateSnapshot, State: json.RawMessage(`{"step":1}`)},
		aikit.StepEvent{
			Type:       aikit.StepEventStateDelta,
			StatePatch: json.RawMessage(`[{"op":"replace","path":"/step","value":2}]`),
		},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "after"},
		aikit.StepEvent{Type: aikit.StepEventDone},
	))

	types := eventTypes(t, stream)
	want := []string{"STATE_SNAPSHOT", "STATE_DELTA"}
	var seen []string
	for _, name := range types {
		if name == want[0] || name == want[1] {
			seen = append(seen, name)
		}
	}
	if len(seen) != 2 || seen[0] != want[0] || seen[1] != want[1] {
		t.Fatalf("state frames = %v, want %v in order (all: %v)", seen, want, types)
	}
	if indexOfType(types, "STATE_SNAPSHOT") > indexOfType(types, "TEXT_MESSAGE_END") {
		t.Errorf("state frames must interleave with the text they accompany: %v", types)
	}
}

func TestStateEventsNoOpWhenEmpty(t *testing.T) {
	stream := runProtocol(t, sequence(
		aikit.StepEvent{Type: aikit.StepEventStateSnapshot},
		aikit.StepEvent{Type: aikit.StepEventStateDelta},
		aikit.StepEvent{Type: aikit.StepEventDone},
	))
	for _, name := range eventTypes(t, stream) {
		if name == "STATE_SNAPSHOT" || name == "STATE_DELTA" {
			t.Errorf("an empty payload must write no frame, got %s", name)
		}
	}
}

// A malformed patch must fail loudly. A silent drop would leave the consumer
// unable to distinguish a rejected update from one that never arrived.
func TestStateDeltaRejectsMalformedPatch(t *testing.T) {
	cases := map[string]string{
		"not an array":      `{"op":"replace","path":"/a","value":1}`,
		"json null":         `null`,
		"element not obj":   `["replace"]`,
		"missing op":        `[{"path":"/a","value":1}]`,
		"unknown op":        `[{"op":"upsert","path":"/a","value":1}]`,
		"empty op":          `[{"op":"","path":"/a"}]`,
		"missing path":      `[{"op":"remove"}]`,
		"path not pointer":  `[{"op":"remove","path":"notapointer"}]`,
		"add without value": `[{"op":"add","path":"/a"}]`,
		"move without from": `[{"op":"move","path":"/a"}]`,
		"from not pointer":  `[{"op":"copy","from":"nope","path":"/a"}]`,
		"second op is bad":  `[{"op":"add","path":"/a","value":1},{"op":"nope","path":"/b"}]`,
	}
	for name, patch := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validatePatch(json.RawMessage(patch)); err == nil {
				t.Errorf("validatePatch(%s) accepted a malformed patch", patch)
			}
		})
	}
}

func TestStateDeltaAcceptsEveryRFC6902Op(t *testing.T) {
	patch := `[
		{"op":"add","path":"/a","value":1},
		{"op":"remove","path":"/b"},
		{"op":"replace","path":"/c","value":2},
		{"op":"move","from":"/d","path":"/e"},
		{"op":"copy","from":"/f","path":"/g"},
		{"op":"test","path":"/h","value":3}
	]`
	if err := validatePatch(json.RawMessage(patch)); err != nil {
		t.Fatalf("a well-formed patch was rejected: %v", err)
	}
	// An explicit null is a legitimate value, and "" is the whole-document
	// pointer — neither may be mistaken for an absent field.
	for _, patch := range []string{
		`[{"op":"replace","path":"/a","value":null}]`,
		`[{"op":"replace","path":"","value":{"whole":"doc"}}]`,
	} {
		if err := validatePatch(json.RawMessage(patch)); err != nil {
			t.Errorf("validatePatch(%s) = %v, want accepted", patch, err)
		}
	}
}

// A malformed patch terminates the run as an error rather than being dropped.
func TestStateDeltaFailureTerminatesRun(t *testing.T) {
	var output bytes.Buffer
	protocol := Protocol(WithRunID(func() string { return "run_1" }))
	events := sequence(
		aikit.StepEvent{Type: aikit.StepEventStateDelta, StatePatch: json.RawMessage(`{"op":"add"}`)},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)
	// Pipe normalizes the encoder error into the stream's terminal event.
	_ = uistream.Pipe(context.Background(), &output, events, protocol, uistream.Options{
		Extra: map[string]any{"runId": "run_1", "threadId": "thread_1"},
	})

	types := eventTypes(t, output.String())
	if indexOfType(types, "RUN_ERROR") < 0 {
		t.Fatalf("a malformed patch must surface as RUN_ERROR, got %v", types)
	}
	if indexOfType(types, "STATE_DELTA") >= 0 {
		t.Error("a rejected patch must never reach the wire")
	}
}

// The interrupt-boundary echo is a separate mechanism and must keep working.
func TestInterruptStateEchoUnchanged(t *testing.T) {
	var output bytes.Buffer
	protocol := Protocol(WithRunID(func() string { return "run_1" }))
	events := sequence(
		aikit.StepEvent{
			Type: aikit.StepEventToolApprovalRequest, ToolCallID: "call_1",
			ToolCallName: "sendEmail", ApprovalID: "approval_1", ToolCallArgsDelta: `{}`,
		},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)
	if err := uistream.Pipe(context.Background(), &output, events, protocol, uistream.Options{
		Extra: map[string]any{
			"runId": "run_1", "threadId": "thread_1",
			"state": json.RawMessage(`{"echoed":true}`),
		},
	}); err != nil {
		t.Fatal(err)
	}

	frame := frameOfType(t, output.String(), "STATE_SNAPSHOT")
	if !strings.Contains(frame, `"echoed":true`) {
		t.Errorf("the interrupt boundary must still echo request state: %s", frame)
	}
}

// A run that publishes no state must write exactly what it wrote before.
func TestNoStateEventsLeavesStreamUnchanged(t *testing.T) {
	plain := func() iter.Seq2[aikit.StepEvent, error] {
		return sequence(
			aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "hello"},
			aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
			aikit.StepEvent{Type: aikit.StepEventDone},
		)
	}
	want := []string{
		"RUN_STARTED", "TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT",
		"TEXT_MESSAGE_END", "RUN_FINISHED",
	}
	got := eventTypes(t, runProtocol(t, plain()))
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("frame list = %v, want %v", got, want)
	}
}

func TestStructuredOutputStartIsOffByDefault(t *testing.T) {
	stream := runProtocol(t, sequence(
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: `{"a":1}`},
		aikit.StepEvent{Type: aikit.StepEventDone},
	))
	if strings.Contains(stream, "structured-output.start") {
		t.Error("the announcement must be opt-in")
	}
}

func TestStructuredOutputStartPrecedesFirstText(t *testing.T) {
	var output bytes.Buffer
	protocol := Protocol(WithRunID(func() string { return "run_1" }), WithStructuredOutputStart())
	events := sequence(
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: `{"a":1}`},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)
	if err := uistream.Pipe(context.Background(), &output, events, protocol, uistream.Options{
		Extra: map[string]any{"runId": "run_1", "threadId": "thread_1"},
	}); err != nil {
		t.Fatal(err)
	}
	stream := output.String()

	if !strings.Contains(stream, `"name":"structured-output.start"`) {
		t.Fatalf("announcement missing: %s", stream)
	}
	types := eventTypes(t, stream)
	// CUSTOM sits between RUN_STARTED and the first text frame, which is what
	// makes the client route the deltas into a structured-output part.
	if indexOfType(types, "CUSTOM") != 1 {
		t.Errorf("announcement must follow RUN_STARTED immediately: %v", types)
	}
	if indexOfType(types, "CUSTOM") > indexOfType(types, "TEXT_MESSAGE_START") {
		t.Errorf("announcement must precede the first text frame: %v", types)
	}
}

func frameOfType(t *testing.T, stream, want string) string {
	t.Helper()
	for _, line := range strings.Split(stream, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
			t.Fatalf("frame is not JSON: %v", err)
		}
		if envelope.Type == want {
			return payload
		}
	}
	t.Fatalf("%s missing from stream: %s", want, stream)
	return ""
}

func indexOfType(types []string, want string) int {
	for index, name := range types {
		if name == want {
			return index
		}
	}
	return -1
}

// MESSAGES_SNAPSHOT means the whole conversation: its payload is typed
// Message[], and the client replaces its transcript with it. Publishing only
// the assistant turn deleted the user's message from the UI and left the
// resumed request with no user text.
func TestMessagesSnapshotCarriesRequestHistory(t *testing.T) {
	request, err := (decoder{}).Decode(strings.NewReader(`{
		"threadId":"t1","runId":"run_1","messages":[
			{"id":"m1","role":"user","content":"send the digest"},
			{"id":"m2","role":"assistant","content":"on it"}
		]}`))
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	events := sequence(
		aikit.StepEvent{
			Type: aikit.StepEventToolApprovalRequest, ToolCallID: "call_1",
			ToolCallName: "send_email", ApprovalID: "approval_1", ToolCallArgsDelta: `{}`,
		},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)
	if err := uistream.Pipe(context.Background(), &output, events,
		Protocol(WithRunID(func() string { return "run_1" })),
		uistream.Options{Extra: request.Extra}); err != nil {
		t.Fatal(err)
	}

	var snapshot struct {
		Messages []struct {
			ID      string `json:"id"`
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	frame := frameOfType(t, output.String(), "MESSAGES_SNAPSHOT")
	if err := json.Unmarshal([]byte(frame), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Messages) != 3 {
		t.Fatalf("snapshot carries %d messages, want the two sent plus the assistant turn",
			len(snapshot.Messages))
	}
	if snapshot.Messages[0].Role != "user" || snapshot.Messages[0].Content != "send the digest" {
		t.Errorf("first message = %#v, want the user's own", snapshot.Messages[0])
	}
	if last := snapshot.Messages[2]; last.Role != "assistant" || last.ID != "run_1_message_0" {
		t.Errorf("last message = %#v, want the assistant turn built this run", last)
	}
}

// Prior messages go out verbatim, so fields this package does not model —
// notably the `parts` passthrough carrying tool-call UI state — survive.
func TestMessagesSnapshotPreservesUnmodelledFields(t *testing.T) {
	request, err := (decoder{}).Decode(strings.NewReader(`{
		"threadId":"t1","runId":"run_1","messages":[
			{"id":"m1","role":"user","content":"hi","parts":[{"type":"text","content":"hi"}]}
		]}`))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := uistream.Pipe(context.Background(), &output, sequence(
		aikit.StepEvent{
			Type: aikit.StepEventClientToolRequest, ToolCallID: "call_1",
			ToolCallName: "get_location", ToolCallArgsDelta: `{}`,
		},
		aikit.StepEvent{Type: aikit.StepEventDone},
	), Protocol(WithRunID(func() string { return "run_1" })),
		uistream.Options{Extra: request.Extra}); err != nil {
		t.Fatal(err)
	}
	if frame := frameOfType(t, output.String(), "MESSAGES_SNAPSHOT"); !strings.Contains(
		frame, `"parts":[{"type":"text","content":"hi"}]`) {
		t.Errorf("unmodelled fields were dropped: %s", frame)
	}
}

// An absent "state" key must echo nothing. Raw JSON boxed in an any is never a
// nil interface, so a naive nil check would emit {"snapshot":null} and a
// conforming client would clobber its own state with it.
func TestNoStateKeyEchoesNoSnapshot(t *testing.T) {
	request, err := (decoder{}).Decode(strings.NewReader(
		`{"threadId":"t1","runId":"run_1","messages":[{"id":"m1","role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := uistream.Pipe(context.Background(), &output, sequence(
		aikit.StepEvent{
			Type: aikit.StepEventToolApprovalRequest, ToolCallID: "call_1",
			ToolCallName: "send_email", ApprovalID: "approval_1", ToolCallArgsDelta: `{}`,
		},
		aikit.StepEvent{Type: aikit.StepEventDone},
	), Protocol(WithRunID(func() string { return "run_1" })),
		uistream.Options{Extra: request.Extra}); err != nil {
		t.Fatal(err)
	}
	if indexOfType(eventTypes(t, output.String()), "STATE_SNAPSHOT") >= 0 {
		t.Errorf("a request with no state echoed one: %s", output.String())
	}
}
