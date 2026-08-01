package aisdkhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func TestHandlerSetsAllProtocolHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(successfulRun).ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"messages":[]}`)),
	)

	want := map[string]string{
		"Content-Type":                  "text/event-stream",
		"Cache-Control":                 "no-cache",
		"Connection":                    "keep-alive",
		"x-vercel-ai-ui-message-stream": "v1",
		"x-accel-buffering":             "no",
	}
	for name, value := range want {
		if got := recorder.Header().Get(name); got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
	}
}

func TestHandlerStartsNewMessagesAndReusesExplicitRegenerationID(t *testing.T) {
	for name, testCase := range map[string]struct {
		body       string
		wantID     string
		unwantedID string
	}{
		"submission": {
			body:       `{"trigger":"submit-message","messageId":"previous-assistant","messages":[{"id":"previous-assistant","role":"assistant","content":"old"}]}`,
			unwantedID: `"messageId":"previous-assistant"`,
		},
		"regeneration": {
			body:   `{"trigger":"regenerate-message","messageId":"regenerated-assistant","messages":[]}`,
			wantID: `"messageId":"regenerated-assistant"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			Handler(successfulRun).ServeHTTP(
				recorder,
				httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(testCase.body)),
			)
			stream := recorder.Body.String()
			if testCase.wantID != "" && !strings.Contains(stream, testCase.wantID) {
				t.Fatalf("stream %q does not contain %s", stream, testCase.wantID)
			}
			if testCase.unwantedID != "" && strings.Contains(stream, testCase.unwantedID) {
				t.Fatalf("stream unexpectedly continued previous assistant message: %q", stream)
			}
		})
	}
}

func successfulRun(context.Context, []aikit.Message) (<-chan aikit.StepEvent, error) {
	events := make(chan aikit.StepEvent, 4)
	events <- aikit.StepEvent{Type: aikit.StepEventStepStart}
	events <- aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "hello"}
	events <- aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop}
	events <- aikit.StepEvent{Type: aikit.StepEventDone}
	close(events)
	return events, nil
}
