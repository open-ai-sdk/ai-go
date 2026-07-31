// Package transport provides the shared HTTP, retry, error-mapping, and
// server-sent-event primitives used by providers.
//
// The package is deliberately low in the dependency graph: it depends only on
// the standard library and aikit. Provider-specific request encoding and event
// decoding stay in provider packages.
package transport
