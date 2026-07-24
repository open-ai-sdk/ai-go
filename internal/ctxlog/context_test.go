package ctxlog

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestFromContext_ReturnsDiscardWhenUnset(t *testing.T) {
	l := FromContext(context.Background())
	if l == nil {
		t.Fatal("FromContext must never return nil")
	}
	// A discard-backed logger must not panic and must produce no output.
	var buf bytes.Buffer
	l.Handler().Handle(context.Background(), slog.Record{})
	if buf.Len() != 0 {
		t.Fatalf("expected discard logger to write nothing, got %q", buf.String())
	}
}

func TestFromContext_ReturnsDiscardWhenExplicitlyNil(t *testing.T) {
	ctx := WithLogger(context.Background(), nil)
	if l := FromContext(ctx); l == nil {
		t.Fatal("FromContext must never return nil even for an explicitly nil logger")
	}
}

func TestWithLogger_RoundTrips(t *testing.T) {
	var buf bytes.Buffer
	want := slog.New(slog.NewTextHandler(&buf, nil))

	ctx := WithLogger(context.Background(), want)
	got := FromContext(ctx)

	got.Info("hello")
	if !strings.Contains(buf.String(), "hello") {
		t.Fatalf("expected the attached logger to be returned; buf=%q", buf.String())
	}
}
