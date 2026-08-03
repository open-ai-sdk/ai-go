package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// erroringAgent is a custom Agent whose Stream fails before any bytes are
// written — the case ToolLoopAgent never hits (its failures ride the stream).
type erroringAgent struct{}

func (erroringAgent) ID() string      { return "err" }
func (erroringAgent) Tools() *ToolSet { return nil }
func (erroringAgent) Prompt(context.Context, string) (string, error) {
	return "", errors.New("boom")
}

func (erroringAgent) Chat(context.Context, string, ...Message) (string, error) {
	return "", errors.New("boom")
}

func (erroringAgent) Generate(context.Context, ...Option) (*GenerateTextResult, error) {
	return nil, errors.New("boom")
}

func (erroringAgent) Stream(context.Context, ...Option) (*StreamResult, error) {
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
	if strings.Contains(rec.Body.String(), "boom") || !strings.Contains(rec.Body.String(), "stream error") {
		t.Fatalf("unredacted HTTP error body: %q", rec.Body.String())
	}
}
