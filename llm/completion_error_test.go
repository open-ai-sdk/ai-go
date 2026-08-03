package llm_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

func TestCompletionErrorPreservesAPIErrorAndSentinels(t *testing.T) {
	apiErr := aikit.NewAPIError("openai", 429, nil)
	apiErr.Code = "rate_limit"
	_, err := llm.NewCompletion(&completionModel{err: apiErr}, "secret prompt").Send(context.Background())

	var completionErr *llm.CompletionError
	var recoveredAPIError *aikit.APIError
	if !errors.As(err, &completionErr) || completionErr.Kind != llm.CompletionErrorKindProvider {
		t.Fatalf("error = %T %v, want provider CompletionError", err, err)
	}
	if !errors.As(err, &recoveredAPIError) || recoveredAPIError != apiErr {
		t.Fatalf("errors.As APIError = %#v, want original", recoveredAPIError)
	}
	if !errors.Is(err, aikit.ErrRateLimited) {
		t.Fatal("rate-limit sentinel did not survive CompletionError")
	}
	if strings.Contains(err.Error(), "secret prompt") {
		t.Fatalf("error leaked prompt: %v", err)
	}
}

func TestCompletionErrorKindMatchingAndPartialResponse(t *testing.T) {
	cause := errors.New("stream stopped")
	response, err := llm.NewCompletion(&completionModel{events: []aikit.StreamEvent{
		{Type: aikit.StreamEventTextDelta, TextDelta: "partial"},
		{Type: aikit.StreamEventError, Error: cause},
	}}, "prompt").Send(context.Background())

	if response == nil || response.Text != "partial" {
		t.Fatalf("response = %#v, want partial text", response)
	}
	if !errors.Is(err, cause) || !errors.Is(err, &llm.CompletionError{Kind: llm.CompletionErrorKindProvider}) {
		t.Fatalf("error = %v, want provider kind and original cause", err)
	}
}

func TestCompletionErrorInvalidRequestAndResponseAreDistinct(t *testing.T) {
	_, requestErr := llm.NewCompletion(nil, "prompt").Send(context.Background())
	if !errors.Is(requestErr, &llm.CompletionError{Kind: llm.CompletionErrorKindRequest}) {
		t.Fatalf("request error = %v", requestErr)
	}

	_, responseErr := llm.NewCompletion(nilStreamCompletionModel{}, "prompt").Send(context.Background())
	if !errors.Is(responseErr, &llm.CompletionError{Kind: llm.CompletionErrorKindResponse}) {
		t.Fatalf("response error = %v", responseErr)
	}
}

type nilStreamCompletionModel struct{}

func (nilStreamCompletionModel) ModelID() string { return "nil-stream" }
func (nilStreamCompletionModel) Stream(context.Context, llm.Request) (<-chan aikit.StreamEvent, error) {
	return nil, nil
}
