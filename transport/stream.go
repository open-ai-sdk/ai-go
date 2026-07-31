package transport

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/open-ai-sdk/ai-go/aikit"
)

const defaultStreamBuffer = 64

// StreamDecoder converts provider-specific SSE frames into normalized events.
// The decoder must not close events or the response body; [Stream] owns both.
type StreamDecoder func(
	context.Context,
	*SSEReader,
	chan<- aikit.StreamEvent,
) error

// Stream owns resp.Body and the returned channel for a provider stream. It
// closes the body on normal completion, decoder failure, panic, or context
// cancellation, and closes the returned channel exactly once.
func Stream(
	ctx context.Context,
	resp *http.Response,
	decode StreamDecoder,
) <-chan aikit.StreamEvent {
	out := make(chan aikit.StreamEvent, defaultStreamBuffer)
	raw := make(chan aikit.StreamEvent, defaultStreamBuffer)

	go runDecoder(ctx, resp, decode, raw)
	go relayStream(ctx, raw, out)
	return out
}

func runDecoder(
	ctx context.Context,
	resp *http.Response,
	decode StreamDecoder,
	raw chan<- aikit.StreamEvent,
) {
	defer close(raw)
	if resp == nil || resp.Body == nil {
		raw <- aikit.StreamEvent{
			Type:  aikit.StreamEventError,
			Error: fmt.Errorf("transport: stream response has no body"),
		}
		return
	}

	defer resp.Body.Close()
	defer CloseOnCancel(ctx, resp.Body)()
	defer func() {
		if recovered := recover(); recovered != nil {
			emitRaw(raw, aikit.StreamEvent{
				Type:  aikit.StreamEventError,
				Error: fmt.Errorf("transport: stream decoder panic: %v", recovered),
			})
		}
	}()

	err := decode(ctx, NewSSEReader(resp.Body), raw)
	if err == nil {
		return
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		err = ctxErr
	}
	emitRaw(raw, aikit.StreamEvent{Type: aikit.StreamEventError, Error: err})
}

func relayStream(
	ctx context.Context,
	raw <-chan aikit.StreamEvent,
	out chan<- aikit.StreamEvent,
) {
	defer close(out)
	draining := false
	for event := range raw {
		if event.Type == aikit.StreamEventError {
			if !draining {
				select {
				case out <- event:
				case <-ctx.Done():
					select {
					case out <- event:
					default:
					}
				}
				continue
			}
			select {
			case out <- event:
			default:
			}
			continue
		}
		if draining {
			continue
		}
		select {
		case out <- event:
		case <-ctx.Done():
			draining = true
		}
	}
}

func emitRaw(out chan<- aikit.StreamEvent, event aikit.StreamEvent) {
	out <- event
}

// CloseOnCancel closes closer when ctx is cancelled. Calling the returned stop
// function prevents a later cancellation from closing a normally completed
// body. The stop function is idempotent.
func CloseOnCancel(ctx context.Context, closer io.Closer) (stop func()) {
	done := make(chan struct{})
	var once sync.Once
	go func() {
		select {
		case <-ctx.Done():
			_ = closer.Close()
		case <-done:
		}
	}()
	return func() {
		once.Do(func() {
			close(done)
		})
	}
}
