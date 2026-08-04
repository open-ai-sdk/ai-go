package ai_test

import (
	"context"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

type facadeStreamModel struct{}

func (facadeStreamModel) ModelID() string { return "facade-stream-test" }

func (facadeStreamModel) Stream(context.Context, llm.Request) (<-chan aikit.StreamEvent, error) {
	events := make(chan aikit.StreamEvent, 2)
	events <- aikit.StreamEvent{Type: aikit.StreamEventTextDelta, TextDelta: "hello"}
	events <- aikit.StreamEvent{Type: aikit.StreamEventFinish, FinishReason: aikit.FinishReasonStop}
	close(events)
	return events, nil
}

// The facade's streaming aliases must keep type identity with the leaf
// packages, or code cannot mix ai.* and llm.* declarations.
var (
	_ *llm.StreamingResponse                                          = (*ai.StreamingResponse)(nil)
	_ ai.Stream[ai.StreamEvent]                                       = (*llm.StreamingResponse)(nil)
	_ ai.StreamingPrompt[ai.StreamEvent, *ai.StreamingResponse]       = ai.ModelStream{}
	_ ai.StreamingChat[ai.StreamEvent, *ai.StreamingResponse]         = ai.ModelStream{}
	_ ai.StreamingCompletion[ai.CompletionRequestBuilder]             = ai.ModelStream{}
	_ aikit.StreamingPrompt[aikit.StreamEvent, *ai.StreamingResponse] = ai.ModelStream{}
)

func TestFacadeStreamPromptCarriesTheAggregate(t *testing.T) {
	stream, err := ai.StreamPrompt(context.Background(), facadeStreamModel{}, "hi")
	if err != nil {
		t.Fatalf("StreamPrompt() error = %v", err)
	}
	for _, err := range stream.Events() {
		if err != nil {
			t.Fatalf("Events() error = %v", err)
		}
	}
	if stream.State() != ai.StreamCompleted {
		t.Errorf("State() = %v, want StreamCompleted", stream.State())
	}
	response, err := stream.Response()
	if err != nil {
		t.Fatalf("Response() error = %v", err)
	}
	if response.Text != "hello" {
		t.Errorf("Text = %q, want hello", response.Text)
	}
}

func TestFacadeStreamChatForwardsHistory(t *testing.T) {
	history := []ai.Message{{
		Role:    ai.RoleUser,
		Content: []ai.ContentPart{{Type: ai.ContentPartTypeText, Text: "earlier"}},
	}}
	stream, err := ai.StreamChat(context.Background(), facadeStreamModel{}, "now", history...)
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	for _, err := range stream.Events() {
		if err != nil {
			t.Fatalf("Events() error = %v", err)
		}
	}
	response, err := stream.Response()
	if err != nil {
		t.Fatalf("Response() error = %v", err)
	}
	if response.Text != "hello" {
		t.Errorf("Text = %q, want hello", response.Text)
	}
}

func TestFacadeStreamingReturnsTheModelHandle(t *testing.T) {
	handle := ai.Streaming(facadeStreamModel{})
	stream, err := handle.StreamPrompt(context.Background(), "hi")
	if err != nil {
		t.Fatalf("StreamPrompt() error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
