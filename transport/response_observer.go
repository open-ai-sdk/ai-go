package transport

import (
	"context"
	"net/http"
)

// ResponseMetadata describes received HTTP response headers without exposing or
// consuming the response body. Header is a snapshot owned by the observer.
type ResponseMetadata struct {
	StatusCode int
	Header     http.Header
}

type (
	responseObserverKey struct{}
	errorBodyCaptureKey struct{}
)

// WithResponseObserver adds a synchronous response-header observer to ctx. It
// runs before the body is decoded and composes with an existing observer.
// Callbacks must not block and may be invoked concurrently when ctx is reused.
func WithResponseObserver(ctx context.Context, observer func(ResponseMetadata)) context.Context {
	if observer == nil {
		return ctx
	}
	if previous, ok := ctx.Value(responseObserverKey{}).(func(ResponseMetadata)); ok && previous != nil {
		next := observer
		observer = func(meta ResponseMetadata) {
			previous(ResponseMetadata{StatusCode: meta.StatusCode, Header: meta.Header.Clone()})
			next(meta)
		}
	}
	return context.WithValue(ctx, responseObserverKey{}, observer)
}

func observeResponse(ctx context.Context, response *http.Response) {
	if observer, ok := ctx.Value(responseObserverKey{}).(func(ResponseMetadata)); ok {
		observer(ResponseMetadata{StatusCode: response.StatusCode, Header: response.Header.Clone()})
	}
}

// WithErrorBodyCapture opts into retaining bounded provider error bodies in
// aikit.APIError.RawBody. Bodies are untrusted and may include sensitive data;
// the error's printable representation never includes the retained body.
func WithErrorBodyCapture(ctx context.Context) context.Context {
	return context.WithValue(ctx, errorBodyCaptureKey{}, true)
}

// CaptureErrorBody reports whether raw error-body diagnostics were requested.
func CaptureErrorBody(ctx context.Context) bool {
	capture, ok := ctx.Value(errorBodyCaptureKey{}).(bool)
	return ok && capture
}
