package ainode

import (
	"iter"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// eventChannel adapts the public pull-based event stream to the channel-based
// chunk producer. Iterator errors become terminal error events so the UI wire
// format keeps its existing error behavior.
func eventChannel(events iter.Seq2[aikit.StepEvent, error]) <-chan aikit.StepEvent {
	out := make(chan aikit.StepEvent, 64)
	go func() {
		defer close(out)
		defer recoverPanic(recoverToEvent(out))
		if events == nil {
			return
		}
		for event, err := range events {
			if err != nil {
				out <- aikit.StepEvent{Type: aikit.StepEventError, Error: err}
				return
			}
			out <- event
		}
	}()
	return out
}
