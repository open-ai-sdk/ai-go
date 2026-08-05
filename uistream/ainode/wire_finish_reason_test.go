package ainode

import (
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func TestWireFinishReason(t *testing.T) {
	tests := []struct {
		name   string
		reason aikit.FinishReason
		want   string
	}{
		{name: "stop", reason: aikit.FinishReasonStop, want: "stop"},
		{name: "tool calls", reason: aikit.FinishReasonToolCalls, want: "tool-calls"},
		{name: "length", reason: aikit.FinishReasonLength, want: "length"},
		{name: "content filter", reason: aikit.FinishReasonContentFilter, want: "content-filter"},
		{name: "error", reason: aikit.FinishReasonError, want: "error"},
		{name: "unknown", reason: aikit.FinishReasonUnknown, want: "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := wireFinishReason(tt.reason)
			if !ok {
				t.Fatalf("wireFinishReason(%q) was not mapped", tt.reason)
			}
			if got != tt.want {
				t.Errorf("wireFinishReason(%q) = %q, want %q", tt.reason, got, tt.want)
			}
		})
	}
}

func TestWireFinishReasonRejectsUnmappedValue(t *testing.T) {
	if got, ok := wireFinishReason(aikit.FinishReason("new-provider-reason")); ok {
		t.Fatalf("wireFinishReason accepted an unmapped value as %q", got)
	}
}
