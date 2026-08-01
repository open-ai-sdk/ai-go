package llm_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

type completionModel struct {
	request llm.Request
	events  []aikit.StreamEvent
	err     error
}

func (*completionModel) ModelID() string { return "completion-test" }

func (m *completionModel) Stream(_ context.Context, request llm.Request) (<-chan aikit.StreamEvent, error) {
	m.request = request
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan aikit.StreamEvent, len(m.events))
	for _, event := range m.events {
		ch <- event
	}
	close(ch)
	return ch, nil
}

func TestCompletionBuilderSendsOneDirectModelRequest(t *testing.T) {
	model := &completionModel{events: []aikit.StreamEvent{
		{Type: aikit.StreamEventTextDelta, TextDelta: "hello"},
		{Type: aikit.StreamEventFinish, FinishReason: aikit.FinishReasonStop},
	}}
	response, err := llm.NewCompletion(model, "first").
		Instructions("concise").
		Prompt("second").
		Temperature(0.2).
		MaxTokens(12).
		Send(context.Background())
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if response.Text != "hello" || response.FinishReason != aikit.FinishReasonStop {
		t.Fatalf("response = %#v", response)
	}
	if model.request.Instructions != "concise" || len(model.request.Messages) != 2 ||
		model.request.Messages[1].Content[0].Text != "second" ||
		model.request.Settings.Temperature == nil || *model.request.Settings.Temperature != 0.2 ||
		model.request.Settings.MaxTokens != 12 {
		t.Fatalf("request = %#v", model.request)
	}
}

func TestCompletionAggregatesOrderedRichMessage(t *testing.T) {
	model := &completionModel{events: []aikit.StreamEvent{
		{Type: aikit.StreamEventTextDelta, TextDelta: "before "},
		{Type: aikit.StreamEventReasoningDelta, TextDelta: "think", ThoughtSignature: "sig-1"},
		{Type: aikit.StreamEventToolCallDelta, ToolCallIndex: 0, ToolCallID: "call-1", ToolCallName: "lookup", ToolCallArgsDelta: `{"city":`},
		{Type: aikit.StreamEventToolCallDelta, ToolCallIndex: 0, ToolCallID: "call-1", ToolCallArgsDelta: `"Hanoi"}`},
		{Type: aikit.StreamEventTextDelta, TextDelta: "after"},
		{Type: aikit.StreamEventUsage, Usage: &aikit.Usage{InputTokens: 3, InputTokenDetails: aikit.InputTokenDetails{CacheReadTokens: 1}}},
		{Type: aikit.StreamEventUsage, Usage: &aikit.Usage{OutputTokens: 5, Raw: map[string]any{"provider": "test"}}},
		{Type: aikit.StreamEventSource, Source: &aikit.Source{ID: "source-1"}},
		{Type: aikit.StreamEventFileDelta, FileData: []byte("file"), FileMediaType: "text/plain"},
		{Type: aikit.StreamEventFinish, FinishReason: aikit.FinishReasonToolCalls, RawFinishReason: "tool_calls", ProviderMetadata: map[string]any{"request": "r1"}, Warnings: []aikit.Warning{{Message: "warn"}}},
	}}
	response, err := llm.NewCompletion(model, "prompt").Send(context.Background())
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if response.Text != "before after" || response.Reasoning != "think" || response.Usage.InputTokens != 3 || response.Usage.InputTokenDetails.CacheReadTokens != 1 || response.Usage.OutputTokens != 5 || len(response.Sources) != 1 || len(response.Files) != 1 {
		t.Fatalf("response summary = %#v", response)
	}
	parts := response.Message.Content
	if len(parts) != 4 || parts[0].Text != "before " || parts[1].ReasoningText != "think" ||
		parts[1].ThoughtSignature != "sig-1" || parts[2].ToolCallID != "call-1" || parts[2].ToolCallName != "lookup" || string(parts[2].ToolCallArgs) != `{"city":"Hanoi"}` ||
		parts[3].Text != "after" {
		t.Fatalf("ordered message = %#v", response.Message)
	}
	response.Usage.Raw["provider"] = "changed"
	if got := model.events[6].Usage.Raw["provider"]; got != "test" {
		t.Fatalf("usage raw map was not copied: %v", got)
	}
}

func TestCompletionReturnsPartialResponseForStreamError(t *testing.T) {
	want := errors.New("provider failed")
	model := &completionModel{events: []aikit.StreamEvent{
		{Type: aikit.StreamEventTextDelta, TextDelta: "partial"},
		{Type: aikit.StreamEventError, Error: want},
	}}
	response, err := llm.NewCompletion(model, "prompt").Send(context.Background())
	if !errors.Is(err, want) || response == nil || response.Text != "partial" {
		t.Fatalf("response=%#v err=%v", response, err)
	}
}

func TestCompletionBuilderBuildCopiesOwnedValues(t *testing.T) {
	model := &completionModel{}
	first := llm.NewCompletion(model, "prompt").StopSequences("first").Build()
	second := llm.NewCompletion(model, "prompt").StopSequences("second").Build()
	first.Settings.StopSequences[0] = "changed"
	if !reflect.DeepEqual(second.Settings.StopSequences, []string{"second"}) {
		t.Fatalf("second = %#v", second)
	}
}

func TestCompletionBuilderBranchesDoNotSharePromptBackingStorage(t *testing.T) {
	builder := llm.NewCompletion(&completionModel{}, "first")
	left := builder.Prompt("left").Build()
	right := builder.Prompt("right").Build()
	if left.Messages[1].Content[0].Text != "left" || right.Messages[1].Content[0].Text != "right" {
		t.Fatalf("left=%#v right=%#v", left.Messages, right.Messages)
	}
}

func TestCompletionToolCallUsesIndexUntilIDArrives(t *testing.T) {
	model := &completionModel{events: []aikit.StreamEvent{
		{Type: aikit.StreamEventToolCallDelta, ToolCallIndex: 0, ToolCallName: "lookup", ToolCallArgsDelta: `{"city":`},
		{Type: aikit.StreamEventToolCallDelta, ToolCallIndex: 0, ToolCallID: "call-1", ToolCallArgsDelta: `"Hanoi"}`},
	}}
	response, err := llm.NewCompletion(model, "prompt").Send(context.Background())
	if err != nil || len(response.Message.Content) != 1 || response.Message.Content[0].ToolCallID != "call-1" || string(response.Message.Content[0].ToolCallArgs) != `{"city":"Hanoi"}` {
		t.Fatalf("response=%#v err=%v", response, err)
	}
}

func TestCompletionKeepsReasoningPartsWithDifferentSignaturesSeparate(t *testing.T) {
	model := &completionModel{events: []aikit.StreamEvent{
		{Type: aikit.StreamEventReasoningDelta, TextDelta: "first", ThoughtSignature: "sig-1"},
		{Type: aikit.StreamEventReasoningDelta, TextDelta: "second", ThoughtSignature: "sig-2"},
	}}
	response, err := llm.NewCompletion(model, "prompt").Send(context.Background())
	if err != nil || len(response.Message.Content) != 2 || response.Message.Content[0].ThoughtSignature != "sig-1" || response.Message.Content[1].ThoughtSignature != "sig-2" {
		t.Fatalf("response=%#v err=%v", response, err)
	}
}

func TestCompletionRequiresModel(t *testing.T) {
	_, err := llm.NewCompletion(nil, "prompt").Send(context.Background())
	if err == nil {
		t.Fatal("Send() error = nil")
	}
}

func TestPromptAndChatConveniences(t *testing.T) {
	model := &completionModel{events: []aikit.StreamEvent{{Type: aikit.StreamEventTextDelta, TextDelta: "answer"}}}
	text, err := llm.Prompt(context.Background(), model, "prompt")
	if err != nil || text != "answer" || len(model.request.Messages) != 1 {
		t.Fatalf("Prompt text=%q err=%v request=%#v", text, err, model.request)
	}

	text, err = llm.Chat(context.Background(), model, "next", aikit.Message{Role: aikit.RoleUser, Content: []aikit.ContentPart{{Type: aikit.ContentPartTypeText, Text: "history"}}})
	if err != nil || text != "answer" || len(model.request.Messages) != 2 || model.request.Messages[1].Content[0].Text != "next" {
		t.Fatalf("Chat text=%q err=%v request=%#v", text, err, model.request)
	}
}
