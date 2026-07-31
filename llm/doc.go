// Package llm defines model contracts and request construction.
//
// The package depends only on [aikit]. Providers implement [Model], while
// callers can either construct [Request] explicitly or use [RequestBuilder].
package llm
