package openai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/transport"
)

const (
	responsesEOFCompleted = "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_test\",\"status\":\"completed\",\"model\":\"example-model\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0,\"total_tokens\":1}}}\n\n"
	responsesEOFText      = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"
)

func TestResponsesMissingTerminalEvent(t *testing.T) {
	for _, test := range []struct {
		name, body string
		terminal   bool
	}{
		{name: "empty"},
		{name: "reasoning", body: "data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"thought\"}\n\n"},
		{name: "partial_text", body: responsesEOFText},
		{name: "done_is_not_response_terminal", body: responsesEOFText + "data: [DONE]\n\n"},
		{name: "completed_without_text", body: responsesEOFCompleted, terminal: true},
		{name: "completed_with_done", body: responsesEOFCompleted + "data: [DONE]\n\n", terminal: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			model := NewLanguageModel("example-model", Config{BaseURL: server.URL})
			for _, allow := range []bool{false, true} {
				events, err := model.Stream(
					context.Background(),
					llm.Request{
						ProviderOptions: map[string]any{"openai": ProviderOptions{AllowMissingTerminalEvent: allow}},
					},
				)
				if err != nil {
					t.Fatal(err)
				}
				var failure error
				for event := range events {
					if event.Type == aikit.StreamEventError {
						failure = event.Error
					}
				}
				var missing *aikit.MissingTerminalEventError
				if !allow && !test.terminal {
					if !errors.As(failure, &missing) || !errors.Is(failure, io.ErrUnexpectedEOF) ||
						missing.Provider != "openai" {
						t.Fatalf("missing terminal: %v", failure)
					}
				} else if failure != nil {
					t.Fatalf("valid terminal or explicit compatibility: %v", failure)
				}
			}
		})
	}
}

func TestResponsesMissingTerminalDoesNotReplaceProviderErrors(t *testing.T) {
	for _, body := range []string{
		"data: {invalid\n\n",
		"data: {\"type\":\"error\",\"code\":\"invalid_request_error\",\"message\":\"Synthetic rejection.\"}\n\n",
		"data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"Synthetic failure.\"}}}\n\n",
	} {
		reader := strings.NewReader(body)
		events := make(chan aikit.StreamEvent, 16)
		err := decodeResponsesSSEStreamWithRequestID(
			context.Background(),
			transport.NewSSEReader(reader),
			events,
			"test",
		)
		if err == nil && len(events) > 0 {
			event := <-events
			err = event.Error
		}
		var missing *aikit.MissingTerminalEventError
		if err == nil || errors.As(err, &missing) {
			t.Fatalf("provider error was replaced: %v", err)
		}
	}
}

func TestResponsesEOFUsesExistingPreContentRetryBudget(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_text", true: "after_text"}[partial], func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if calls.Add(1) == 1 {
					if partial {
						_, _ = io.WriteString(w, responsesEOFText)
					}
					return
				}
				_, _ = io.WriteString(w, responsesEOFCompleted)
			}))
			defer server.Close()
			model := llm.WithRetry(
				NewLanguageModel("example-model", Config{BaseURL: server.URL}),
				llm.RetryOptions{MaxRetries: 1},
			)
			events, err := model.Stream(context.Background(), llm.Request{})
			if err != nil {
				t.Fatal(err)
			}
			var failure error
			for event := range events {
				if event.Type == aikit.StreamEventError {
					failure = event.Error
				}
			}
			if partial {
				var missing *aikit.MissingTerminalEventError
				if calls.Load() != 1 || !errors.As(failure, &missing) {
					t.Fatalf("partial text retried or lost failure: calls=%d error=%v", calls.Load(), failure)
				}
			} else if calls.Load() != 2 || failure != nil {
				t.Fatalf("empty EOF retry: calls=%d error=%v", calls.Load(), failure)
			}
		})
	}
}
