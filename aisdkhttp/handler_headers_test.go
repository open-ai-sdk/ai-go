package aisdkhttp

import (
	"context"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream/agui"
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

func TestHandlerForAGUIUsesOnlyGenericSSEHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	HandlerFor(agui.Protocol(), successfulRun).ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/ag-ui", strings.NewReader(`{
			"threadId":"thread_1","runId":"run_1","state":{},"messages":[],
			"tools":[],"context":[],"forwardedProps":{}
		}`)),
	)
	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := recorder.Header().Get("x-vercel-ai-ui-message-stream"); got != "" {
		t.Fatalf("AI Node header leaked into AG-UI response: %q", got)
	}
	if !strings.Contains(recorder.Body.String(), `"type":"RUN_STARTED"`) ||
		!strings.Contains(recorder.Body.String(), `"type":"RUN_FINISHED"`) {
		t.Fatalf("body = %s", recorder.Body.String())
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

func successfulRun(context.Context, []aikit.Message) (iter.Seq2[aikit.StepEvent, error], error) {
	return eventSequence(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "hello"},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	), nil
}
