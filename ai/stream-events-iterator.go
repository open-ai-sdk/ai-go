package ai

import "iter"

// Events yields stream events as an iter.Seq2, one more view alongside Stream,
// TextStream, and Consume. Each range gets its own branch of the fan-out.
//
// The final value is (zero, err) when the stream ends in an error — including
// ErrStreamConsumed when Events is called after the source is exhausted. A
// normal end simply stops yielding.
//
// Breaking out of the range unregisters only this branch; it does NOT cancel the
// run, because other views may still be consuming it. To cancel the whole run,
// cancel the context passed to StreamText — phase-3 made that reliable.
func (sr *StreamResult) Events() iter.Seq2[StepEvent, error] {
	return func(yield func(StepEvent, error) bool) {
		b, err := sr.register()
		if err != nil {
			yield(StepEvent{}, err)
			return
		}
		sr.ensureStarted()
		defer b.unregister(sr)
		for ev := range b.ch {
			if ev.Type == StepEventError {
				yield(StepEvent{}, ev.Error)
				return
			}
			if !yield(ev, nil) {
				return
			}
		}
	}
}
