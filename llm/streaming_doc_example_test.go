package llm_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

// The docStream* functions below are the snippets docs/core/streaming.md and
// docs/core/completions.md publish, verbatim. They live here so the build fails
// when a snippet goes stale, rather than a reader discovering it.
// aikit/docs_snippet_sync_test.go asserts the published text still matches.

// docStreamOneModelCall is the "Direct model stream" snippet.
func docStreamOneModelCall(ctx context.Context, model llm.Model) error {
	stream, err := llm.NewCompletion(model, "Explain Go channels").
		Instructions("Answer concisely.").
		StreamSend(ctx)
	if err != nil {
		return err // synchronous request validation
	}

	for event, err := range stream.Events() {
		if err != nil {
			return err
		}
		if event.Type == aikit.StreamEventTextDelta {
			fmt.Print(event.TextDelta)
		}
	}

	response, err := stream.Response()
	if err != nil {
		return err
	}
	fmt.Println(response.Usage.TotalTokens)
	return nil
}

// docStreamPromptAndChat is the "Prompt and chat" snippet.
func docStreamPromptAndChat(ctx context.Context, model llm.Model, history []aikit.Message) error {
	stream, err := llm.StreamChat(ctx, model, "And in one sentence?", history...)
	if err != nil {
		return err
	}
	defer stream.Close()

	for event, err := range stream.Events() {
		if err != nil {
			return err
		}
		fmt.Print(event.TextDelta)
	}
	return nil
}

// docStreamShapedRequest is the "Shape the request first" snippet.
func docStreamShapedRequest(ctx context.Context, model llm.Model, history []aikit.Message) error {
	builder, err := llm.Streaming(model).StreamCompletion(ctx, "Summarize", history...)
	if err != nil {
		return err
	}
	stream, err := builder.Temperature(0.2).MaxTokens(256).StreamSend(ctx)
	if err != nil {
		return err
	}
	for _, err := range stream.Events() {
		if err != nil {
			return err
		}
	}
	response, err := stream.Response()
	if err != nil {
		return err
	}
	fmt.Println(response.Text)
	return nil
}

// docAcceptAnythingStreamable is the "Writing code over either layer" snippet.
func docAcceptAnythingStreamable[E any, S aikit.Stream[E], P aikit.StreamingPrompt[E, S]](
	ctx context.Context, source P, prompt string,
) (S, error) {
	stream, err := source.StreamPrompt(ctx, prompt)
	if err != nil {
		return stream, err
	}
	for _, err := range stream.Events() {
		if err != nil {
			return stream, err
		}
	}
	return stream, nil
}

// Compiling is the main gate, but running them catches a snippet that compiles
// and still misuses the API.
func TestDocumentedModelStreamSnippetsRun(t *testing.T) {
	ctx := context.Background()
	history := []aikit.Message{aikit.UserMessage("Explain Go channels")}

	if err := docStreamOneModelCall(ctx, &countingStreamModel{events: richEvents()}); err != nil {
		t.Errorf("docStreamOneModelCall() error = %v", err)
	}
	if err := docStreamPromptAndChat(ctx, &countingStreamModel{events: richEvents()}, history); err != nil {
		t.Errorf("docStreamPromptAndChat() error = %v", err)
	}
	if err := docStreamShapedRequest(ctx, &countingStreamModel{events: richEvents()}, history); err != nil {
		t.Errorf("docStreamShapedRequest() error = %v", err)
	}

	stream, err := docAcceptAnythingStreamable[aikit.StreamEvent, *llm.StreamingResponse](
		ctx, llm.Streaming(&countingStreamModel{events: richEvents()}), "Explain Go channels",
	)
	if err != nil {
		t.Fatalf("docAcceptAnythingStreamable() error = %v", err)
	}
	if _, err := stream.Response(); err != nil {
		t.Errorf("Response() error = %v", err)
	}
}
