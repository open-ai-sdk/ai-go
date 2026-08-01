// Package llm defines provider-neutral model contracts, normalized requests,
// and direct single-call completions.
//
// The package depends only on [aikit]. Providers implement [Model], while
// callers can either construct [Request] explicitly or use [RequestBuilder].
// Applications that want multi-step tool execution should use the higher-level
// ai package instead.
package llm
