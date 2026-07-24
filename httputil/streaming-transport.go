package httputil

import (
	"context"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/open-ai-sdk/ai-go/ai"
)

// DefaultStreamingTransport returns an *http.Transport tuned for long-lived
// streaming responses. It sets connect, TLS, idle, and response-header
// deadlines but deliberately imposes NO overall client deadline.
//
// The three-layer liveness model for a streaming call is:
//   - DialContext.Timeout / TLSHandshakeTimeout — establishing the connection,
//   - ResponseHeaderTimeout — waiting for the model to start responding,
//   - the request context — the overall lifetime of the stream,
//   - the provider's ChunkTimeout — inter-chunk idle once streaming.
//
// A client-wide http.Client.Timeout is intentionally NOT used: it is a hard
// deadline on the entire exchange, including reading the streaming body, so it
// would kill a legitimately long generation at that deadline.
//
// headerTimeout bounds how long to wait for the response headers to arrive; pass
// 0 to wait indefinitely (relying on ctx and ChunkTimeout instead).
func DefaultStreamingTransport(headerTimeout time.Duration) *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: headerTimeout,
	}
}

// NewStreamingClient returns an *http.Client built on DefaultStreamingTransport
// with no overall Timeout, suitable for streaming provider calls.
func NewStreamingClient(headerTimeout time.Duration) *http.Client {
	return &http.Client{Transport: DefaultStreamingTransport(headerTimeout)}
}

// GuardStream wraps a raw provider event channel so that every forwarded send
// selects on ctx.Done(). This lets a decoder emit onto `raw` with plain sends
// while still honouring the LanguageModel.Stream context contract: when the
// consumer stops reading and cancels ctx, the relay stops forwarding and drains
// `raw` so the decoder goroutine can finish and release its HTTP body — the
// caller is not required to drain the returned channel.
func GuardStream(ctx context.Context, raw <-chan ai.StreamEvent) <-chan ai.StreamEvent {
	out := make(chan ai.StreamEvent, 64)
	go func() {
		defer close(out)
		for ev := range raw {
			select {
			case out <- ev:
			case <-ctx.Done():
				// Consumer gone: drain the decoder so it can close its body.
				for range raw {
				}
				return
			}
		}
	}()
	return out
}

// CloseOnCancel starts a watcher that closes c when ctx is cancelled, so a
// goroutine blocked reading the associated body (which context cancellation
// alone cannot interrupt) is unblocked and can observe ctx.Err().
//
// The returned stop function MUST be called when the read completes normally —
// typically `defer CloseOnCancel(ctx, body)()` — so the watcher exits without
// closing a connection that may be reused. Closing the body more than once is
// safe.
func CloseOnCancel(ctx context.Context, c io.Closer) (stop func()) {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = c.Close()
		case <-done:
		}
	}()
	return func() { close(done) }
}
