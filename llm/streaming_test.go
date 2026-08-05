package llm_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

// drainHelper is the shape a generic consumer over the streaming interfaces
// takes. It exists to prove the two-parameter interfaces can be written
// against, and to pin the partial-inference cost its callers pay.
func drainHelper[E any, S aikit.Stream[E], P aikit.StreamingPrompt[E, S]](
	ctx context.Context, p P, prompt string,
) (S, error) {
	stream, err := p.StreamPrompt(ctx, prompt)
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

func TestStreamPromptDrainedEqualsPrompt(t *testing.T) {
	text, err := llm.Prompt(context.Background(), &countingStreamModel{events: richEvents()}, "hello")
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}

	stream, err := llm.StreamPrompt(context.Background(), &countingStreamModel{events: richEvents()}, "hello")
	if err != nil {
		t.Fatalf("StreamPrompt() error = %v", err)
	}
	drain(t, stream)
	response, err := stream.Response()
	if err != nil {
		t.Fatalf("Response() error = %v", err)
	}
	if response.Text != text {
		t.Errorf("streamed text = %q, want %q", response.Text, text)
	}
}

func TestStreamChatBuildsTheSameRequestAsChat(t *testing.T) {
	history := []aikit.Message{{
		Role:    aikit.RoleUser,
		Content: []aikit.ContentPart{{Type: aikit.ContentPartTypeText, Text: "earlier"}},
	}}

	chatModel := &completionModel{events: richEvents()}
	if _, err := llm.Chat(context.Background(), chatModel, "now", history...); err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	streamModel := &completionModel{events: richEvents()}
	stream, err := llm.StreamChat(context.Background(), streamModel, "now", history...)
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	drain(t, stream)

	if !reflect.DeepEqual(streamModel.request, chatModel.request) {
		t.Fatalf("StreamChat request = %#v, want Chat's %#v", streamModel.request, chatModel.request)
	}
}

func TestStreamChatDrainedEqualsChat(t *testing.T) {
	history := []aikit.Message{{
		Role:    aikit.RoleUser,
		Content: []aikit.ContentPart{{Type: aikit.ContentPartTypeText, Text: "earlier"}},
	}}
	text, err := llm.Chat(context.Background(), &countingStreamModel{events: richEvents()}, "now", history...)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	stream, err := llm.StreamChat(
		context.Background(), &countingStreamModel{events: richEvents()}, "now", history...,
	)
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	drain(t, stream)
	response, err := stream.Response()
	if err != nil {
		t.Fatalf("Response() error = %v", err)
	}
	if response.Text != text {
		t.Errorf("streamed text = %q, want %q", response.Text, text)
	}
}

func TestStreamCompletionReturnsAShapeableBuilder(t *testing.T) {
	model := &completionModel{events: richEvents()}
	builder, err := llm.Streaming(model).StreamCompletion(context.Background(), "now")
	if err != nil {
		t.Fatalf("StreamCompletion() error = %v", err)
	}
	stream, err := builder.Temperature(0.25).MaxTokens(7).StreamSend(context.Background())
	if err != nil {
		t.Fatalf("StreamSend() error = %v", err)
	}
	drain(t, stream)

	if model.request.Settings.Temperature == nil || *model.request.Settings.Temperature != 0.25 {
		t.Errorf("Temperature = %v, want 0.25", model.request.Settings.Temperature)
	}
	if model.request.Settings.MaxTokens != 7 {
		t.Errorf("MaxTokens = %d, want 7", model.request.Settings.MaxTokens)
	}
}

// A forwarder that returned a nil pointer boxed in an interface would pass a
// `!= nil` check and panic on first use, so the concrete nil is the contract.
func TestStreamingEntrypointsReturnGenuineNilOnValidationError(t *testing.T) {
	t.Run("StreamPrompt", func(t *testing.T) {
		stream, err := llm.StreamPrompt(context.Background(), nil, "hello")
		if err == nil {
			t.Fatal("error = nil for a nil model")
		}
		if stream != nil {
			t.Fatalf("stream = %#v, want a nil *StreamingResponse", stream)
		}
	})
	t.Run("StreamChat", func(t *testing.T) {
		stream, err := llm.StreamChat(context.Background(), nil, "hello")
		if err == nil {
			t.Fatal("error = nil for a nil model")
		}
		if stream != nil {
			t.Fatalf("stream = %#v, want a nil *StreamingResponse", stream)
		}
	})
	t.Run("StreamCompletion", func(t *testing.T) {
		builder, err := llm.Streaming(nil).StreamCompletion(context.Background(), "hello")
		if err == nil {
			t.Fatal("error = nil for a nil model")
		}
		if !reflect.DeepEqual(builder, llm.CompletionRequestBuilder{}) {
			t.Fatalf("builder = %#v, want the zero builder", builder)
		}
		var completionErr *llm.CompletionError
		if !errors.As(err, &completionErr) || completionErr.Kind != llm.CompletionErrorKindRequest {
			t.Fatalf("error = %v, want a CompletionError of kind invalid_request", err)
		}
	})
}

// The interfaces exist so code can accept anything streamable. If this stops
// compiling, the generics are not carrying their weight.
func TestGenericConsumerAcceptsTheModelLayer(t *testing.T) {
	handle := llm.Streaming(&countingStreamModel{events: richEvents()})
	stream, err := drainHelper[aikit.StreamEvent, *llm.StreamingResponse](
		context.Background(), handle, "hello",
	)
	if err != nil {
		t.Fatalf("drainHelper() error = %v", err)
	}
	response, err := stream.Response()
	if err != nil {
		t.Fatalf("Response() error = %v", err)
	}
	if response.Text != "hello" {
		t.Errorf("Text = %q, want hello", response.Text)
	}
}

func TestStreamingExposesTheWrappedModel(t *testing.T) {
	model := &countingStreamModel{}
	if got := llm.Streaming(model).Model(); got != llm.Model(model) {
		t.Errorf("Model() = %#v, want the wrapped model", got)
	}
}
