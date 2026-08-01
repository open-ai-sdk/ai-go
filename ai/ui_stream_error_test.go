package uistream

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
)

// erroringAgent is a custom ai.Agent whose Stream fails before any bytes are
// written — the case ToolLoopAgent never hits (its failures ride the stream).
type erroringAgent struct{}

func (erroringAgent) ID() string         { return "err" }
func (erroringAgent) Tools() *ai.ToolSet { return nil }
func (erroringAgent) Generate(context.Context, ...ai.Option) (*ai.GenerateTextResult, error) {
	return nil, errors.New("boom")
}

func (erroringAgent) Stream(context.Context, ...ai.Option) (*ai.StreamResult, error) {
	return nil, errors.New("boom")
}

// TestAgentHandler_PreStreamErrorReturns500 verifies a pre-stream Stream failure
// yields a 500, not a bare 200 with an empty body.
func TestAgentHandler_PreStreamErrorReturns500(t *testing.T) {
	h := AgentHandler(erroringAgent{})
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"messages":[]}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
