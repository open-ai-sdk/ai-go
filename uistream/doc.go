// Package uistream provides protocol-neutral plumbing for event-driven UI
// streams. It owns framing, draining, terminal-error normalization, and panic
// recovery. Protocol adapters own their vocabulary, ordering, persistence,
// usage shape, and redaction. Imperative protocol writers are outside this
// seam.
package uistream
