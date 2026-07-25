package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/open-ai-sdk/ai-go/internal/safego"
)

// FallbackModel tries models in order, falling back on transient errors.
type FallbackModel struct {
	models []LanguageModel
}

// WithFallback creates a LanguageModel that tries each model in order.
// On transient errors (429, 500+, timeout), the next model is tried.
// Non-transient errors (400, 401, 403) are returned immediately.
// Panics if no models are provided.
func WithFallback(models ...LanguageModel) LanguageModel {
	if len(models) == 0 {
		panic("ai.WithFallback: at least one model is required")
	}
	if len(models) == 1 {
		return models[0]
	}
	return &FallbackModel{models: models}
}

// ModelID returns a composite ID of all fallback models.
func (f *FallbackModel) ModelID() string {
	ids := make([]string, len(f.models))
	for i, m := range f.models {
		ids[i] = m.ModelID()
	}
	return "fallback(" + strings.Join(ids, ",") + ")"
}

// Stream tries each model in order until one succeeds or all fail.
// It handles both synchronous errors (from model.Stream) and asynchronous
// stream errors (StreamEventError on the channel) for the first event only.
func (f *FallbackModel) Stream(ctx context.Context, req LanguageModelRequest) (<-chan StreamEvent, error) {
	var lastErr error
	for i, model := range f.models {
		ch, err := model.Stream(ctx, req)
		if err != nil {
			lastErr = err
			if !isFallbackRetryable(err) {
				return nil, fmt.Errorf(
					"fallback model %d (%s): %w",
					i, model.ModelID(), err,
				)
			}
			continue
		}

		// Peek at the first event to detect immediate stream errors.
		// If the first event is an error and retryable, try the next model.
		// Honour caller cancellation while waiting: a primary that stalls after
		// opening the stream must not pin the caller past its own deadline.
		var firstEvent StreamEvent
		var ok bool
		select {
		case firstEvent, ok = <-ch:
		case <-ctx.Done():
			go func() {
				defer safego.Recover(nil, nil)
				for range ch {
				}
			}()
			return nil, ctx.Err()
		}
		if !ok {
			// Channel closed immediately — empty stream, return it.
			out := make(chan StreamEvent)
			close(out)
			return out, nil
		}
		if firstEvent.Type == StreamEventError && firstEvent.Error != nil {
			if isFallbackRetryable(firstEvent.Error) {
				lastErr = firstEvent.Error
				// Drain remaining events to prevent goroutine leak
				go func() {
					defer safego.Recover(nil, nil)
					for range ch {
					}
				}()
				continue
			}
		}

		// Re-emit the first event followed by the rest of the stream. Sends are
		// guarded on ctx so an abandoning consumer cannot park this goroutine;
		// on early exit the upstream is drained so its body is released.
		out := make(chan StreamEvent, 64)
		go func() {
			defer close(out)
			// A panic while re-emitting surfaces as an error event (ctx-guarded)
			// before close rather than crashing the process.
			defer safego.Recover(nil, func(err error) {
				select {
				case out <- StreamEvent{Type: StreamEventError, Error: err}:
				case <-ctx.Done():
				}
			})
			send := func(ev StreamEvent) bool {
				select {
				case out <- ev:
					return true
				case <-ctx.Done():
					go func() {
						defer safego.Recover(nil, nil)
						for range ch {
						}
					}()
					return false
				}
			}
			if !send(firstEvent) {
				return
			}
			for ev := range ch {
				if !send(ev) {
					return
				}
			}
		}()
		return out, nil
	}
	return nil, fmt.Errorf(
		"all fallback models failed, last error: %w",
		lastErr,
	)
}

// isFallbackRetryable reports whether err warrants trying the next model. It
// shares isRetryable's typed classification (status code + network error type),
// so falling over to the next provider is never triggered by attacker-echoed
// message text in a 400 body.
func isFallbackRetryable(err error) bool {
	return isRetryable(err)
}
