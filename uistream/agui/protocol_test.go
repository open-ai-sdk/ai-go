package agui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"iter"
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
		"RUN_STARTED", "STEP_STARTED", "TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT",
		"TEXT_MESSAGE_END", "STEP_FINISHED", "RUN_FINISHED",
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

func TestProtocolRejectsApprovalWithNamedError(t *testing.T) {
	events := sequence(aikit.StepEvent{Type: aikit.StepEventToolApprovalRequest, ToolCallName: "send_email"})
	var output bytes.Buffer
	err := uistream.Pipe(
		context.Background(),
		&output,
		events,
		Protocol(WithRunID(func() string { return "run_1" })),
		uistream.Options{Extra: map[string]any{"threadId": "thread_1"}},
	)
	if !errors.Is(err, ErrToolApprovalUnsupported) {
		t.Fatalf("error = %v, want ErrToolApprovalUnsupported", err)
	}
	if got := eventTypes(t, output.String()); !equalStrings(got, []string{"RUN_STARTED", "RUN_ERROR"}) {
		t.Fatalf("events = %v", got)
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
		"unknown role":  `{"threadId":"t","runId":"r","messages":[{"id":"m","role":"activity","content":"x"}]}`,
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
		aikit.StepEventTextDelta: "mapped", aikit.StepEventReasoningDelta: "dropped",
		aikit.StepEventToolCallStart: "mapped", aikit.StepEventToolCallDelta: "mapped",
		aikit.StepEventToolCallReady: "mapped", aikit.StepEventToolResult: "mapped",
		aikit.StepEventToolApprovalRequest: "rejected", aikit.StepEventToolOutputDenied: "mapped",
		aikit.StepEventUsage: "folded", aikit.StepEventStepStart: "mapped",
		aikit.StepEventStepEnd: "mapped", aikit.StepEventToolCallInvalid: "mapped",
		aikit.StepEventStructuredOutput: "dropped", aikit.StepEventDone: "swallowed",
		aikit.StepEventError: "normalized", aikit.StepEventSource: "dropped",
		aikit.StepEventFileDelta: "dropped",
	}
	for eventType := aikit.StepEventTextDelta; eventType <= aikit.StepEventFileDelta; eventType++ {
		if classified[eventType] == "" {
			t.Errorf("StepEventType %d is not classified", eventType)
		}
	}
	if len(classified) != int(aikit.StepEventFileDelta)+1 {
		t.Fatalf("classified %d events, want %d", len(classified), int(aikit.StepEventFileDelta)+1)
	}
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
