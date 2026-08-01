package generate

import (
	"log/slog"
	"testing"
)

func TestWithLogger_SetsField(t *testing.T) {
	var req GenerateTextRequest
	logger := slog.New(slog.DiscardHandler)
	WithLogger(logger)(&req)
	if req.Logger != logger {
		t.Fatal("WithLogger did not set GenerateTextRequest.Logger")
	}
}

func TestWithLogger_DefaultIsNil(t *testing.T) {
	var req GenerateTextRequest
	if req.Logger != nil {
		t.Fatal("GenerateTextRequest.Logger must default to nil (no WithLogger call)")
	}
}

func TestWithTraceContent_SetsField(t *testing.T) {
	var req GenerateTextRequest
	WithTraceContent(true)(&req)
	if !req.TraceContent {
		t.Fatal("WithTraceContent(true) did not set GenerateTextRequest.TraceContent")
	}
	WithTraceContent(false)(&req)
	if req.TraceContent {
		t.Fatal("WithTraceContent(false) did not clear GenerateTextRequest.TraceContent")
	}
}

func TestWithTraceContent_DefaultIsFalse(t *testing.T) {
	var req GenerateTextRequest
	if req.TraceContent {
		t.Fatal("GenerateTextRequest.TraceContent must default to false (no WithTraceContent call)")
	}
}
