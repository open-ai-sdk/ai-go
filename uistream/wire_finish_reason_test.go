package uistream

import (
	"testing"

	"github.com/open-ai-sdk/ai-go/aitypes"
)

func TestWireFinishReason(t *testing.T) {
	tests := []struct {
		name   string
		reason aitypes.FinishReason
		want   string
	}{
		{name: "stop", reason: aitypes.FinishReasonStop, want: "stop"},
		{name: "tool calls", reason: aitypes.FinishReasonToolCalls, want: "tool-calls"},
		{name: "length", reason: aitypes.FinishReasonLength, want: "length"},
		{name: "content filter", reason: aitypes.FinishReasonContentFilter, want: "content-filter"},
		{name: "error", reason: aitypes.FinishReasonError, want: "error"},
		{name: "unknown", reason: aitypes.FinishReasonUnknown, want: "other"},
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
	if got, ok := wireFinishReason(aitypes.FinishReason("new-provider-reason")); ok {
		t.Fatalf("wireFinishReason accepted an unmapped value as %q", got)
	}
}
