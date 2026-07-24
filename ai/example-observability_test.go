package ai_test

import (
	"context"
	"log/slog"
	"os"

	"github.com/open-ai-sdk/ai-go/ai"
)

// ExampleWithLogger wires a caller-supplied logger into a call so panics
// recovered at the tool-loop boundary and dropped provider error bodies
// reach it. Without WithLogger the SDK produces no log output at all — it
// never falls back to slog.Default().
func ExampleWithLogger() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	model := &stubLanguageModel{events: []ai.StreamEvent{
		{Type: ai.StreamEventTextDelta, TextDelta: "Hi!"},
		{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonStop},
	}}

	_, _ = ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model:    model,
		Messages: []ai.Message{ai.UserMessage("Hello!")},
		Logger:   logger,
	})
}

// ExampleWithTraceContent opts a single call into attaching prompt,
// completion, and tool-argument content to trace spans — off by default,
// since spans are commonly exported to a third-party backend that should
// not necessarily see conversation content.
func ExampleWithTraceContent() {
	model := &stubLanguageModel{events: []ai.StreamEvent{
		{Type: ai.StreamEventTextDelta, TextDelta: "Hi!"},
		{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonStop},
	}}

	_, _ = ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model:        model,
		Messages:     []ai.Message{ai.UserMessage("Hello!")},
		TraceContent: true,
	})
}
