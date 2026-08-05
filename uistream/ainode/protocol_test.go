package ainode

import (
	"bytes"
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream"
)

func TestProtocolMatchesChunkProducerBytes(t *testing.T) {
	events := []aikit.StepEvent{
		{Type: aikit.StepEventStepStart},
		{Type: aikit.StepEventTextDelta, TextDelta: "hello"},
		{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		{Type: aikit.StepEventDone},
	}

	channel := make(chan aikit.StepEvent, len(events))
	for _, event := range events {
		channel <- event
	}
	close(channel)
	var legacy bytes.Buffer
	if err := WriteSSEStream(&legacy, NewChunkProducer("msg_1").Produce(channel).Chunks); err != nil {
		t.Fatal(err)
	}

	sequence := iter.Seq2[aikit.StepEvent, error](func(yield func(aikit.StepEvent, error) bool) {
		for _, event := range events {
			if !yield(event, nil) {
				return
			}
		}
	})
	var protocol bytes.Buffer
	if err := uistream.Pipe(
		context.Background(),
		&protocol,
		sequence,
		Protocol(),
		uistream.Options{MessageID: "msg_1"},
	); err != nil {
		t.Fatal(err)
	}
	if protocol.String() != legacy.String() {
		t.Fatalf("protocol bytes differ\nprotocol: %q\nlegacy:   %q", protocol.String(), legacy.String())
	}
}

func TestProtocolErrorMatchesChunkProducerBytes(t *testing.T) {
	want := errors.New("provider failed")
	channel := make(chan aikit.StepEvent, 1)
	channel <- aikit.StepEvent{Type: aikit.StepEventError, Error: want}
	close(channel)
	var legacy bytes.Buffer
	if err := WriteSSEStream(&legacy, NewChunkProducer("msg_1").Produce(channel).Chunks); err != nil {
		t.Fatal(err)
	}

	sequence := iter.Seq2[aikit.StepEvent, error](func(yield func(aikit.StepEvent, error) bool) {
		yield(aikit.StepEvent{}, want)
	})
	var protocol bytes.Buffer
	if err := uistream.Pipe(
		context.Background(),
		&protocol,
		sequence,
		Protocol(),
		uistream.Options{MessageID: "msg_1"},
	); !errors.Is(
		err,
		want,
	) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if protocol.String() != legacy.String() {
		t.Fatalf("protocol error bytes differ\nprotocol: %q\nlegacy:   %q", protocol.String(), legacy.String())
	}
}

func TestDecoderPreservesMultipartAndApprovalSemantics(t *testing.T) {
	req, err := (decoder{}).Decode(
		strings.NewReader(
			`{"messages":[{"role":"user","parts":[{"type":"text","text":"describe"},{"type":"image","url":"https://example.test/a.png","mediaType":"image/png"},{"type":"tool-invocation","toolCallId":"call_1","toolName":"lookup","input":{"q":"x"},"state":"result"},{"type":"tool-invocation","toolCallId":"call_2","toolName":"approve","state":"approval-responded","approval":{"id":"approval_1","approved":true,"signature":"sig"}}]}]}`,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(req.Messages))
	}
	if got := len(req.Messages[0].Content); got != 5 {
		t.Fatalf("content parts = %d, want 5", got)
	}
	approval := req.Messages[1].Content[0]
	if approval.ToolApprovalID != "approval_1" || !approval.ToolApprovalApproved {
		t.Fatalf("approval = %#v", approval)
	}
}

func TestEncoderPreservesApprovalAndDenialEvents(t *testing.T) {
	e := &encoder{producer: NewChunkProducer("msg_1")}
	if _, err := e.Start(); err != nil {
		t.Fatal(err)
	}
	frames, err := e.Encode(
		aikit.StepEvent{
			Type:              aikit.StepEventToolApprovalRequest,
			ApprovalID:        "approval_1",
			ToolCallID:        "call_1",
			ToolCallName:      "lookup",
			ApprovalSignature: "sig",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	for _, f := range frames {
		b.Write(f.Data)
	}
	if !strings.Contains(b.String(), `"type":"tool-approval-request"`) ||
		!strings.Contains(b.String(), `"signature":"sig"`) {
		t.Fatalf("approval frames = %s", b.String())
	}
}

func TestEncoderMapsFinishReasonsToWireVocabulary(t *testing.T) {
	for _, tc := range []struct {
		reason aikit.FinishReason
		want   string
	}{
		{aikit.FinishReasonToolCalls, "tool-calls"}, {aikit.FinishReasonContentFilter, "content-filter"}, {aikit.FinishReasonUnknown, "other"},
	} {
		e := &encoder{producer: NewChunkProducer("msg_1")}
		_, _ = e.Start()
		if _, err := e.Encode(aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: tc.reason}); err != nil {
			t.Fatal(err)
		}
		frames, err := e.Encode(aikit.StepEvent{Type: aikit.StepEventDone})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(frames[0].Data), `"finishReason":"`+tc.want+`"`) {
			t.Errorf("%s: %s", tc.reason, frames[0].Data)
		}
	}
}

func TestDecoderSkipsUnknownToolStates(t *testing.T) {
	req, err := (decoder{}).Decode(
		strings.NewReader(
			`{"messages":[{"role":"assistant","parts":[{"type":"tool-invocation","toolCallId":"call_1","toolName":"lookup","state":"error"}]}]}`,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 0 {
		t.Fatalf("messages = %#v, want none", req.Messages)
	}
}
