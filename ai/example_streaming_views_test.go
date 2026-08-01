package ai_test

import (
	"context"
	"fmt"

	"github.com/open-ai-sdk/ai-go/ai"
)

// ExampleStreamResult_Events shows the iter.Seq2 view over a stream: ranging
// over Events() yields each event alongside a nil error, or a terminal
// (StepEvent{}, err) pair when the stream ends in an error — including a
// mid-stream provider failure, not just a pre-flight validation error.
func ExampleStreamResult_Events() {
	model := &stubLanguageModel{events: []ai.StreamEvent{
		{Type: ai.StreamEventTextDelta, TextDelta: "Hello, "},
		{Type: ai.StreamEventTextDelta, TextDelta: "world!"},
		{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonStop},
	}}

	sr := ai.StreamText(context.Background(), ai.GenerateTextRequest{
		Model:    model,
		Messages: []ai.Message{ai.UserMessage("Hi")},
	})

	for ev, err := range sr.Events() {
		if err != nil {
			fmt.Println("error:", err)
			break
		}
		if ev.Type == ai.StepEventTextDelta {
			fmt.Print(ev.TextDelta)
		}
	}
	fmt.Println()

	// Output:
	// Hello, world!
}
