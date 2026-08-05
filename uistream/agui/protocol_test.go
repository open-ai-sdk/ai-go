package agui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"iter"
	"os"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream"
)

func TestProtocolProducesOrderedRunLifecycle(t *testing.T) {
	events := sequence(
		aikit.StepEvent{Type: aikit.StepEventStepStart, StepNumber: 1},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, StepNumber: 1, TextDelta: "hello"},
		aikit.StepEvent{
			Type:  aikit.StepEventUsage,
			Usage: &aikit.Usage{InputTokens: 2, OutputTokens: 3, TotalTokens: 5},
		},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, StepNumber: 1},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)
	stream := runProtocol(t, events)
	want := []string{
		"RUN_STARTED", "TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT",
		"TEXT_MESSAGE_END", "RUN_FINISHED",
	}
	if got := eventTypes(t, stream); !equalStrings(got, want) {
		t.Fatalf("event order = %v, want %v\n%s", got, want, stream)
	}
	if !strings.Contains(stream, `"threadId":"thread_1"`) ||
		!strings.Contains(stream, `"role":"assistant"`) ||
		!strings.Contains(stream, `"totalTokens":5`) {
		t.Fatalf("stream is missing required AG-UI fields: %s", stream)
	}
}

func TestProtocolPropagatesParentRunIDToRunStarted(t *testing.T) {
	var output bytes.Buffer
	if err := uistream.Pipe(
		context.Background(),
		&output,
		sequence(aikit.StepEvent{Type: aikit.StepEventDone}),
		Protocol(WithRunID(func() string { return "run_2" })),
		uistream.Options{Extra: map[string]any{"threadId": "thread_1", "parentRunId": "run_1"}},
	); err != nil {
		t.Fatal(err)
	}
	started := firstEvent(t, output.String())
	if started["parentRunId"] != "run_1" {
		t.Fatalf("RUN_STARTED parentRunId = %#v, want run_1", started["parentRunId"])
	}
}

func TestProtocolClosesTextAndToolsBeforeTerminalError(t *testing.T) {
	wantErr := errors.New("provider failure")
	events := iter.Seq2[aikit.StepEvent, error](func(yield func(aikit.StepEvent, error) bool) {
		if !yield(aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "hello"}, nil) {
			return
		}
		if !yield(
			aikit.StepEvent{Type: aikit.StepEventToolCallStart, ToolCallID: "call_1", ToolCallName: "lookup"},
			nil,
		) {
			return
		}
		yield(aikit.StepEvent{}, wantErr)
	})
	var output bytes.Buffer
	err := uistream.Pipe(
		context.Background(),
		&output,
		events,
		Protocol(WithRunID(func() string { return "run_1" })),
		uistream.Options{Extra: map[string]any{"threadId": "thread_1"}},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	want := []string{
		"RUN_STARTED",
		"TEXT_MESSAGE_START",
		"TEXT_MESSAGE_CONTENT",
		"TOOL_CALL_START",
		"TEXT_MESSAGE_END",
		"TOOL_CALL_END",
		"RUN_ERROR",
	}
	if got := eventTypes(t, output.String()); !equalStrings(got, want) {
		t.Fatalf("event order = %v, want %v\n%s", got, want, output.String())
	}
}

func TestProtocolSuspendsRunOnToolApproval(t *testing.T) {
	events := sequence(
		aikit.StepEvent{Type: aikit.StepEventStepStart, StepNumber: 1},
		aikit.StepEvent{
			Type: aikit.StepEventToolCallStart, ToolCallID: "call_1",
			ToolCallName: "send_email", ToolCallArgsDelta: `{"to":"a@b.test"}`,
		},
		aikit.StepEvent{Type: aikit.StepEventToolCallReady, ToolCallID: "call_1"},
		aikit.StepEvent{
			Type: aikit.StepEventToolApprovalRequest, ApprovalID: "approval_1",
			ToolCallID: "call_1", ToolCallName: "send_email",
			ToolCallArgsDelta: `{"to":"a@b.test"}`, ApprovalSignature: "sig-1",
		},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, StepNumber: 1, FinishReason: aikit.FinishReasonToolCalls},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)
	stream := runProtocol(t, events)

	want := []string{
		"RUN_STARTED", "TOOL_CALL_START", "TOOL_CALL_ARGS",
		"TOOL_CALL_END", "MESSAGES_SNAPSHOT", "RUN_FINISHED",
	}
	if got := eventTypes(t, stream); !equalStrings(got, want) {
		t.Fatalf("event order = %v, want %v\n%s", got, want, stream)
	}

	finished := lastEvent(t, stream)
	outcome, ok := finished["outcome"].(map[string]any)
	if !ok || outcome["type"] != "interrupt" {
		t.Fatalf("RUN_FINISHED outcome = %#v", finished["outcome"])
	}
	interrupts, ok := outcome["interrupts"].([]any)
	if !ok || len(interrupts) != 1 {
		t.Fatalf("interrupts = %#v", outcome["interrupts"])
	}
	interrupt, _ := interrupts[0].(map[string]any)
	if interrupt["id"] != "approval_1" || interrupt["reason"] != "tool_call" ||
		interrupt["toolCallId"] != "call_1" {
		t.Fatalf("interrupt = %#v", interrupt)
	}
	metadata, _ := interrupt["metadata"].(map[string]any)
	binding, _ := metadata["tanstack:interruptBinding"].(map[string]any)
	if binding["kind"] != "tool-approval" || binding["toolCallId"] != "call_1" ||
		binding["signature"] != "sig-1" {
		t.Fatalf("interrupt binding = %#v", binding)
	}
}

func TestProtocolEmitsReasoningBeforeText(t *testing.T) {
	events := sequence(
		aikit.StepEvent{Type: aikit.StepEventStepStart, StepNumber: 1},
		aikit.StepEvent{Type: aikit.StepEventReasoningDelta, ReasoningDelta: "thinking"},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, StepNumber: 1, TextDelta: "answer"},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, StepNumber: 1, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)
	stream := runProtocol(t, events)
	want := []string{
		"RUN_STARTED",
		"REASONING_START", "REASONING_MESSAGE_START", "REASONING_MESSAGE_CONTENT",
		"REASONING_MESSAGE_END", "REASONING_END",
		"TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT", "TEXT_MESSAGE_END",
		"RUN_FINISHED",
	}
	if got := eventTypes(t, stream); !equalStrings(got, want) {
		t.Fatalf("event order = %v, want %v\n%s", got, want, stream)
	}
	// THINKING_* is deprecated in @ag-ui/core and absent from TanStack's union.
	if strings.Contains(stream, "THINKING_") {
		t.Fatalf("stream emitted deprecated THINKING_* events: %s", stream)
	}
}

func TestProtocolEmitsCustomEventsForSourcesFilesAndStructuredOutput(t *testing.T) {
	events := sequence(
		aikit.StepEvent{Type: aikit.StepEventStepStart, StepNumber: 1},
		aikit.StepEvent{
			Type:   aikit.StepEventSource,
			Source: &aikit.Source{SourceType: "url", ID: "s1", URL: "https://example.test", Title: "Example"},
		},
		aikit.StepEvent{
			Type: aikit.StepEventFileDelta, FileData: []byte("binary"), FileMediaType: "image/png",
		},
		aikit.StepEvent{Type: aikit.StepEventStructuredOutput, StructuredOutput: []byte(`{"ok":true}`)},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, StepNumber: 1, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)
	stream := runProtocol(t, events)
	want := []string{
		"RUN_STARTED", "CUSTOM", "CUSTOM", "CUSTOM", "RUN_FINISHED",
	}
	if got := eventTypes(t, stream); !equalStrings(got, want) {
		t.Fatalf("event order = %v, want %v\n%s", got, want, stream)
	}
	for _, name := range []string{`"name":"source"`, `"name":"file"`, `"name":"structured-output.complete"`} {
		if !strings.Contains(stream, name) {
			t.Errorf("stream is missing custom event %s: %s", name, stream)
		}
	}
	if !strings.Contains(stream, "data:image/png;base64,YmluYXJ5") {
		t.Errorf("file custom event is not a data URL: %s", stream)
	}
}

// TanStack AI overloads STEP_* as its reasoning transport: handleStepFinished
// builds a thinking part even when the event carries no content, so bare step
// markers render an empty "thinking" block for every step.
func TestProtocolOmitsStepEventsByDefault(t *testing.T) {
	events := sequence(
		aikit.StepEvent{Type: aikit.StepEventStepStart, StepNumber: 1},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, StepNumber: 1, TextDelta: "hi"},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, StepNumber: 1, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)
	stream := runProtocol(t, events)
	if strings.Contains(stream, "STEP_STARTED") || strings.Contains(stream, "STEP_FINISHED") {
		t.Fatalf("default stream emitted step markers: %s", stream)
	}
}

func TestProtocolEmitsStepEventsWhenEnabled(t *testing.T) {
	events := sequence(
		aikit.StepEvent{Type: aikit.StepEventStepStart, StepNumber: 1},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, StepNumber: 1, TextDelta: "hi"},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, StepNumber: 1, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)
	var output bytes.Buffer
	err := uistream.Pipe(
		context.Background(),
		&output,
		events,
		Protocol(WithRunID(func() string { return "run_1" }), WithStepEvents()),
		uistream.Options{Extra: map[string]any{"runId": "run_1", "threadId": "thread_1"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"RUN_STARTED", "STEP_STARTED", "TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT",
		"TEXT_MESSAGE_END", "STEP_FINISHED", "RUN_FINISHED",
	}
	if got := eventTypes(t, output.String()); !equalStrings(got, want) {
		t.Fatalf("event order = %v, want %v\n%s", got, want, output.String())
	}
}

// Usage folding and per-step tool resets must survive the step markers being
// suppressed, since both hang off the same step events.
func TestProtocolFoldsUsageAcrossStepsWithoutStepEvents(t *testing.T) {
	events := sequence(
		aikit.StepEvent{Type: aikit.StepEventStepStart, StepNumber: 1},
		aikit.StepEvent{
			Type:  aikit.StepEventUsage,
			Usage: &aikit.Usage{InputTokens: 2, OutputTokens: 3, TotalTokens: 5},
		},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, StepNumber: 1},
		aikit.StepEvent{Type: aikit.StepEventStepStart, StepNumber: 2},
		aikit.StepEvent{
			Type:  aikit.StepEventUsage,
			Usage: &aikit.Usage{InputTokens: 4, OutputTokens: 6, TotalTokens: 10},
		},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, StepNumber: 2, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)
	finished := lastEvent(t, runProtocol(t, events))
	usage, ok := finished["usage"].(map[string]any)
	if !ok {
		t.Fatalf("RUN_FINISHED carries no usage: %#v", finished)
	}
	if usage["promptTokens"] != float64(6) || usage["completionTokens"] != float64(9) ||
		usage["totalTokens"] != float64(15) {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestProtocolRedactsTerminalErrorAndScopesItToTheRun(t *testing.T) {
	events := iter.Seq2[aikit.StepEvent, error](func(yield func(aikit.StepEvent, error) bool) {
		yield(aikit.StepEvent{}, &aikit.APIError{StatusCode: 503})
	})
	var output bytes.Buffer
	_ = uistream.Pipe(
		context.Background(),
		&output,
		events,
		Protocol(WithRunID(func() string { return "run_1" })),
		uistream.Options{Extra: map[string]any{"runId": "run_1", "threadId": "thread_1"}},
	)
	runError := lastEvent(t, output.String())
	if runError["type"] != "RUN_ERROR" {
		t.Fatalf("terminal event = %#v", runError)
	}
	// The status code survives; the provider message does not.
	if runError["message"] != "provider error (status 503)" {
		t.Errorf("message = %#v", runError["message"])
	}
	// A run-less RUN_ERROR makes the client clear every active run.
	if runError["runId"] != "run_1" || runError["threadId"] != "thread_1" {
		t.Errorf("RUN_ERROR is not scoped to the run: %#v", runError)
	}
}

func TestProtocolNeverEmitsDoneSentinel(t *testing.T) {
	events := sequence(
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "hi"},
		aikit.StepEvent{Type: aikit.StepEventDone},
	)
	// RUN_FINISHED terminates an AG-UI stream; TanStack's SSE parser warns that
	// a [DONE] sentinel is deprecated.
	if stream := runProtocol(t, events); strings.Contains(stream, "[DONE]") {
		t.Fatalf("AG-UI stream emitted a [DONE] sentinel: %s", stream)
	}
}

func TestDecoderAcceptsTanStackFanOutRoles(t *testing.T) {
	// uiMessagesToWire fans an assistant turn out into reasoning and tool rows
	// alongside the anchor message.
	request, err := (decoder{}).Decode(strings.NewReader(`{
		"threadId":"t1","runId":"r1","state":{},"tools":[],"context":[],
		"messages":[
			{"id":"m1","role":"user","parts":[{"type":"text","text":"hi"}],"content":"hi"},
			{"id":"m2-reasoning-p1","role":"reasoning","content":"let me think"},
			{"id":"m2","role":"assistant","content":"hello"},
			{"id":"a1","role":"activity","content":"typing"}
		]}`))
	if err != nil {
		t.Fatalf("Decode failed on a real TanStack body: %v", err)
	}
	// Reasoning and activity rows carry no engine history of their own.
	if len(request.Messages) != 2 {
		t.Fatalf("messages = %#v", request.Messages)
	}
	if request.Messages[0].Role != aikit.RoleUser || request.Messages[1].Role != aikit.RoleAssistant {
		t.Fatalf("roles = %#v", request.Messages)
	}
}

func TestDecoderReadsResumeEntries(t *testing.T) {
	request, err := (decoder{}).Decode(strings.NewReader(`{
		"threadId":"t1","runId":"r1","messages":[{"id":"m1","role":"user","content":"hi"}],
		"resume":[{"interruptId":"approval_1","status":"resolved","payload":{"approved":true}}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	entries, ok := request.Extra["resume"].([]ResumeEntry)
	if !ok || len(entries) != 1 {
		t.Fatalf("resume = %#v", request.Extra["resume"])
	}
	if entries[0].InterruptID != "approval_1" || entries[0].Status != "resolved" {
		t.Fatalf("resume entry = %#v", entries[0])
	}
}

func TestDecoderReadsRunAgentInput(t *testing.T) {
	request, err := (decoder{}).Decode(strings.NewReader(`{
		"threadId":"thread_1","runId":"run_1","state":{"page":2},
		"messages":[
			{"id":"m1","role":"developer","content":"be concise"},
			{"id":"m2","role":"user","content":[{"type":"text","text":"hello"},{"type":"image","source":{"type":"url","value":"https://example.test/a.png","mimeType":"image/png"}}]},
			{"id":"m3","role":"assistant","toolCalls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"x\"}"}}]},
			{"id":"m4","role":"tool","toolCallId":"call_1","content":"done"}
		],"tools":[],"context":[],"forwardedProps":{"trace":"x"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if request.ID != "thread_1" || len(request.Messages) != 4 {
		t.Fatalf("request = %#v", request)
	}
	if request.Messages[0].Role != aikit.RoleSystem || request.Messages[1].ID != "" ||
		request.Messages[1].Content[1].Type != aikit.ContentPartTypeImage ||
		request.Messages[2].Content[0].ToolCallName != "lookup" ||
		request.Messages[3].Content[0].ToolResultName != "lookup" {
		t.Fatalf("messages = %#v", request.Messages)
	}
	if request.Extra["runId"] != "run_1" || request.Extra["threadId"] != "thread_1" {
		t.Fatalf("extra = %#v", request.Extra)
	}
}

func TestDecoderRejectsInvalidEnvelopeBoundaries(t *testing.T) {
	for name, body := range map[string]string{
		"missing IDs":   `{"messages":[]}`,
		"trailing JSON": `{"threadId":"t","runId":"r","messages":[]} {}`,
		"unknown role":  `{"threadId":"t","runId":"r","messages":[{"id":"m","role":"sorcerer","content":"x"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := (decoder{}).Decode(strings.NewReader(body)); err == nil {
				t.Fatal("Decode succeeded, want error")
			}
		})
	}
}

func TestStepEventMappingIsExhaustive(t *testing.T) {
	classified := map[aikit.StepEventType]string{
		aikit.StepEventTextDelta: "mapped", aikit.StepEventReasoningDelta: "mapped",
		aikit.StepEventToolCallStart: "mapped", aikit.StepEventToolCallDelta: "mapped",
		aikit.StepEventToolCallReady: "mapped", aikit.StepEventToolResult: "mapped",
		aikit.StepEventToolApprovalRequest: "interrupt", aikit.StepEventToolOutputDenied: "mapped",
		aikit.StepEventUsage: "folded", aikit.StepEventStepStart: "mapped",
		aikit.StepEventStepEnd: "mapped", aikit.StepEventToolCallInvalid: "mapped",
		aikit.StepEventStructuredOutput: "mapped", aikit.StepEventDone: "swallowed",
		aikit.StepEventError: "normalized", aikit.StepEventSource: "mapped",
		aikit.StepEventFileDelta: "mapped", aikit.StepEventClientToolRequest: "interrupt",
		aikit.StepEventStateSnapshot: "mapped", aikit.StepEventStateDelta: "mapped",
	}
	// The upper bound is read from the source enum rather than hardcoded here:
	// pinning it to a named constant would leave this test green for exactly
	// the drift it exists to catch — a newly appended event type.
	last := lastDeclaredStepEventType(t)
	for eventType := aikit.StepEventTextDelta; eventType <= last; eventType++ {
		if classified[eventType] == "" {
			t.Errorf("StepEventType %d is not classified", eventType)
		}
	}
	if len(classified) != int(last)+1 {
		t.Fatalf("classified %d events, want %d", len(classified), int(last)+1)
	}
}

// lastDeclaredStepEventType counts the constants in aikit's iota block, so
// appending one fails the exhaustiveness check above until it is classified.
func lastDeclaredStepEventType(t *testing.T) aikit.StepEventType {
	t.Helper()
	source, err := os.ReadFile("../../aikit/step_event.go")
	if err != nil {
		t.Fatal(err)
	}
	block := string(source)
	start := strings.Index(block, "StepEventTextDelta StepEventType = iota")
	if start < 0 {
		t.Fatal("could not find the StepEventType iota block")
	}
	end := strings.Index(block[start:], "\n)")
	if end < 0 {
		t.Fatal("could not find the end of the StepEventType iota block")
	}
	count := 0
	for _, line := range strings.Split(block[start:start+end], "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "StepEvent") {
			count++
		}
	}
	if count == 0 {
		t.Fatal("found no StepEventType constants")
	}
	return aikit.StepEventType(count - 1)
}

func runProtocol(t *testing.T, events iter.Seq2[aikit.StepEvent, error]) string {
	t.Helper()
	var output bytes.Buffer
	protocol := Protocol(WithRunID(func() string { return "fallback" }))
	options := uistream.Options{Extra: map[string]any{"runId": "run_1", "threadId": "thread_1"}}
	if err := uistream.Pipe(context.Background(), &output, events, protocol, options); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func sequence(events ...aikit.StepEvent) iter.Seq2[aikit.StepEvent, error] {
	return func(yield func(aikit.StepEvent, error) bool) {
		for _, event := range events {
			if !yield(event, nil) {
				return
			}
		}
	}
}

func eventTypes(t *testing.T, stream string) []string {
	t.Helper()
	var types []string
	for _, line := range strings.Split(stream, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatalf("decode event %q: %v", line, err)
		}
		types = append(types, event.Type)
	}
	return types
}

// lastEvent decodes the final SSE frame, which is always the terminal AG-UI
// event for a completed pipe.
func lastEvent(t *testing.T, stream string) map[string]any {
	t.Helper()
	var last map[string]any
	for _, line := range strings.Split(stream, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		event := map[string]any{}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatalf("decode event %q: %v", line, err)
		}
		last = event
	}
	if last == nil {
		t.Fatalf("stream has no events: %s", stream)
	}
	return last
}

func firstEvent(t *testing.T, stream string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(stream, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		event := map[string]any{}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatalf("decode event %q: %v", line, err)
		}
		return event
	}
	t.Fatalf("stream has no events: %s", stream)
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
