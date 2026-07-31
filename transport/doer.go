package transport

import "net/http"

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
