package aisdkhttp

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream"
	"github.com/open-ai-sdk/ai-go/uistream/agui"
)

const aguiBody = `{
	"threadId":"thread_1","runId":"run_1","state":{"stage":"draft"},
	"messages":[{"id":"m1","role":"user","content":"hi"}],
	"tools":[{"name":"render_chart"}],"context":[],
	"forwardedProps":{"selection":"row-4"},
	"resume":[{"interruptId":"approval_1","status":"resolved","payload":{"approved":true}}]
}`

func postAGUI(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/ag-ui", strings.NewReader(aguiBody)))
	return recorder
}

// The whole point of the seam: extras the message-only form cannot reach.
func TestHandlerForRequestExposesProtocolExtras(t *testing.T) {
	var seen uistream.Request
	handler := HandlerForRequest(agui.Protocol(), func(
		_ context.Context, request uistream.Request,
	) (iter.Seq2[aikit.StepEvent, error], error) {
		seen = request
		return successfulRun(context.Background(), request.Messages)
	})
	postAGUI(t, handler)

	if len(seen.Messages) != 1 || seen.Messages[0].Role != aikit.RoleUser {
		t.Fatalf("messages = %#v", seen.Messages)
	}

	props, ok := seen.Extra["forwardedProps"].(map[string]any)
	if !ok || props["selection"] != "row-4" {
		t.Errorf("forwardedProps = %#v", seen.Extra["forwardedProps"])
	}

	// Consumable without a JSON round trip through a private mirror struct.
	entries, ok := seen.Extra["resume"].([]agui.ResumeEntry)
	if !ok || len(entries) != 1 {
		t.Fatalf("resume = %#v", seen.Extra["resume"])
	}
	if entries[0].InterruptID != "approval_1" || entries[0].Status != "resolved" {
		t.Errorf("resume entry = %#v", entries[0])
	}
	var decision struct {
		Approved bool `json:"approved"`
	}
	if err := json.Unmarshal(entries[0].Payload, &decision); err != nil || !decision.Approved {
		t.Errorf("payload = %s (err %v)", entries[0].Payload, err)
	}

	if seen.Extra["tools"] == nil {
		t.Error("client tool declarations must reach the run")
	}
	if seen.Extra["state"] == nil {
		t.Error("run state must reach the run")
	}
}

// HandlerFor keeps its exact signature and delivers the same messages.
func TestHandlerForStillReceivesMessagesOnly(t *testing.T) {
	var seen []aikit.Message
	handler := HandlerFor(agui.Protocol(), func(
		ctx context.Context, messages []aikit.Message,
	) (iter.Seq2[aikit.StepEvent, error], error) {
		seen = messages
		return successfulRun(ctx, messages)
	})
	recorder := postAGUI(t, handler)

	if len(seen) != 1 || seen[0].Role != aikit.RoleUser {
		t.Fatalf("messages = %#v", seen)
	}
	if !strings.Contains(recorder.Body.String(), "RUN_FINISHED") {
		t.Errorf("stream did not complete: %s", recorder.Body.String())
	}
}

func TestHandlerForRequestPanicsOnNilRun(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != "aisdkhttp: nil RunFunc" {
			t.Errorf("panic = %v, want the unchanged message", recovered)
		}
	}()
	HandlerForRequest(agui.Protocol(), nil)
	t.Error("a nil run must panic")
}

func TestHandlerForPanicsOnNilRun(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != "aisdkhttp: nil RunFunc" {
			t.Errorf("panic = %v, want the unchanged message", recovered)
		}
	}()
	HandlerFor(agui.Protocol(), nil)
	t.Error("a nil run must panic")
}

func TestHandlerForRequestPanicsOnIncompleteProtocol(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != "aisdkhttp: incomplete protocol" {
			t.Errorf("panic = %v, want the unchanged message", recovered)
		}
	}()
	HandlerForRequest(uistream.Protocol{}, func(
		context.Context, uistream.Request,
	) (iter.Seq2[aikit.StepEvent, error], error) {
		return nil, nil
	})
	t.Error("an incomplete protocol must panic")
}

func TestHandlerForRequestRejectsNonPost(t *testing.T) {
	recorder := httptest.NewRecorder()
	HandlerForRequest(agui.Protocol(), successfulRequestRun).ServeHTTP(
		recorder, httptest.NewRequest(http.MethodGet, "/ag-ui", nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", recorder.Code)
	}
	if got := recorder.Header().Get("Allow"); got != http.MethodPost {
		t.Errorf("Allow = %q, want POST", got)
	}
}

func TestHandlerForRequestRejectsUndecodableBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	HandlerForRequest(agui.Protocol(), successfulRequestRun).ServeHTTP(
		recorder, httptest.NewRequest(http.MethodPost, "/ag-ui", strings.NewReader("{")))

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), invalidRequestMessage) {
		t.Errorf("body = %q", recorder.Body.String())
	}
}

func TestHandlerForRequestReportsPreStreamFailure(t *testing.T) {
	recorder := postAGUI(t, HandlerForRequest(agui.Protocol(), func(
		context.Context, uistream.Request,
	) (iter.Seq2[aikit.StepEvent, error], error) {
		return nil, errors.New("model unavailable")
	}))

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", recorder.Code)
	}
	// The provider message must never reach the client verbatim.
	if body := recorder.Body.String(); !strings.Contains(body, streamErrorMessage) ||
		strings.Contains(body, "model unavailable") {
		t.Errorf("body = %q", body)
	}
}

func successfulRequestRun(
	ctx context.Context, request uistream.Request,
) (iter.Seq2[aikit.StepEvent, error], error) {
	return successfulRun(ctx, request.Messages)
}
