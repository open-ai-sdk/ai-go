package aisdkhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

const sensitiveError = "secret provider request body"

func TestHandlerPreStreamErrorUsesHTTPStatusAndRedacts(t *testing.T) {
	run := func(context.Context, []aikit.Message) (<-chan aikit.StepEvent, error) {
		return nil, errors.New(sensitiveError)
	}
	recorder := httptest.NewRecorder()
	Handler(run).ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"messages":[]}`)),
	)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	body := recorder.Body.String()
	if strings.Contains(body, sensitiveError) ||
		!strings.Contains(body, streamErrorMessage) {
		t.Fatalf("unexpected error body %q", body)
	}
	if got := recorder.Header().Get(contentTypeHeader); got == "text/event-stream" {
		t.Fatal("pre-stream error incorrectly committed SSE headers")
	}
}

func TestHandlerMidStreamErrorUsesRedactedChunk(t *testing.T) {
	run := func(context.Context, []aikit.Message) (<-chan aikit.StepEvent, error) {
		events := make(chan aikit.StepEvent, 3)
		events <- aikit.StepEvent{Type: aikit.StepEventStepStart}
		events <- aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "partial"}
		events <- aikit.StepEvent{Type: aikit.StepEventError, Error: errors.New(sensitiveError)}
		close(events)
		return events, nil
	}
	recorder := httptest.NewRecorder()
	Handler(run).ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"messages":[]}`)),
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	body := recorder.Body.String()
	if strings.Contains(body, sensitiveError) ||
		!strings.Contains(body, `"type":"error"`) ||
		!strings.Contains(body, `"errorText":"stream error"`) {
		t.Fatalf("unexpected SSE error body %q", body)
	}
	if !strings.HasSuffix(body, "data: [DONE]\n\n") {
		t.Fatalf("error stream did not terminate: %q", body)
	}
}

func TestHandlerRejectsInvalidRequestsBeforeRunning(t *testing.T) {
	called := false
	run := func(context.Context, []aikit.Message) (<-chan aikit.StepEvent, error) {
		called = true
		return nil, nil
	}
	for name, request := range map[string]struct {
		request *http.Request
		status  int
	}{
		"method": {httptest.NewRequest(http.MethodGet, "/chat", nil), http.StatusMethodNotAllowed},
		"json":   {httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader("not-json")), http.StatusBadRequest},
		"null":   {httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader("null")), http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			Handler(run).ServeHTTP(recorder, request.request)
			if recorder.Code != request.status {
				t.Fatalf("status = %d, want %d", recorder.Code, request.status)
			}
		})
	}
	if called {
		t.Fatal("run was called for an invalid request")
	}
}
