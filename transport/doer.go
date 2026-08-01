package transport

import (
	"errors"
	"net/http"
)

// Doer is the HTTP execution seam used by [Client]. *http.Client satisfies it.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// DoerFunc adapts a function to [Doer].
type DoerFunc func(*http.Request) (*http.Response, error)

// Do implements [Doer].
func (f DoerFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

// HandleResponse executes req, gives handler exclusive synchronous access to
// the response, and closes its body after handler returns.
func HandleResponse(doer Doer, req *http.Request, handler func(*http.Response) error) error {
	if handler == nil {
		return errors.New("transport: nil response handler")
	}
	resp, err := doer.Do(req)
	if err != nil {
		return err
	}
	if resp == nil || resp.Body == nil {
		return errors.New("transport: HTTP response has no body")
	}
	defer resp.Body.Close()
	return handler(resp)
}
