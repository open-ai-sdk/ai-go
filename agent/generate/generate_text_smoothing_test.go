package generate

import (
	"context"
	"testing"
	"time"
)

// textStreamModel streams a fixed set of text deltas then finishes.
type textStreamModel struct{ deltas []string }

func (textStreamModel) ModelID() string { return "textstream" }

func (m textStreamModel) Stream(context.Context, LanguageModelRequest) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, len(m.deltas)+1)
	for _, d := range m.deltas {
		ch <- StreamEvent{Type: StreamEventTextDelta, TextDelta: d}
	}
	ch <- StreamEvent{Type: StreamEventFinish, FinishReason: FinishReasonStop}
	close(ch)
	return ch, nil
}

// TestGenerateText_DisablesSmoothing proves GenerateText does not pay the
// per-chunk SmoothStream delay: with a 500ms/chunk smoother set, five deltas
// still aggregate almost instantly because GenerateText nils smoothing.
func TestGenerateText_DisablesSmoothing(t *testing.T) {
	model := textStreamModel{deltas: []string{"one ", "two ", "three ", "four ", "five "}}
	start := time.Now()
	res, err := GenerateText(context.Background(), GenerateTextRequest{
		Model:        model,
		Messages:     []Message{UserMessage("hi")},
		MaxSteps:     1,
		SmoothStream: NewSmoothStream(WithDelayMs(500)),
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("GenerateText: %v", err)
	}
	if res.Text != "one two three four five " {
		t.Errorf("text = %q", res.Text)
	}
	if elapsed > 400*time.Millisecond {
		t.Errorf("GenerateText took %v — SmoothStream delay was not disabled", elapsed)
	}
}
