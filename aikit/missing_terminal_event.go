package aikit

import "io"

// MissingTerminalEventError means a provider stream ended before reporting a
// terminal response. It is an unexpected EOF, not a successful empty response.
type MissingTerminalEventError struct {
	Provider string
}

func (e *MissingTerminalEventError) Error() string {
	if e.Provider == "" {
		return "stream ended without a terminal event"
	}
	return e.Provider + ": stream ended without a terminal event"
}

// Unwrap allows generic pre-commit retry policies to classify truncated streams.
func (e *MissingTerminalEventError) Unwrap() error { return io.ErrUnexpectedEOF }
