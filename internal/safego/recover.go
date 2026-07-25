// Package safego provides a single recovery boundary for library-owned
// goroutines. A library normally lets panics propagate, but here the panicking
// code is usually a *consumer's* callback running on a goroutine the consumer
// did not start and cannot defend — the same situation net/http recovers from
// per connection. The policy is recover-then-surface: never silently swallow.
// The recovered value becomes a typed *PanicError logged with a stack trace, so
// the bug is relocated from a process abort to a reported failure, not hidden.
//
// Do NOT use this to wrap callbacks invoked synchronously on the consumer's own
// goroutine: that panic is the consumer's to see, on their own stack.
package safego

import (
	"fmt"
	"log/slog"
	"runtime"
)

// PanicError wraps a value recovered from a panic along with the stack trace
// captured at recovery time.
type PanicError struct {
	Value any
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("panic recovered: %v", e.Value)
}

// Recover converts a panic into a *PanicError via onPanic and logs it at error
// level with the stack trace. It must be deferred as the first statement of a
// library-owned goroutine. When nothing panicked it is a no-op.
//
// Typical use, with close deferred first so it runs last (the error is emitted
// before the channel closes):
//
//	go func() {
//	    defer close(out)
//	    defer safego.Recover(logger, func(err error) { emit(err) }, "phase", "tool-loop")
//	    ...
//	}()
func Recover(logger *slog.Logger, onPanic func(error), attrs ...any) {
	v := recover()
	if v == nil {
		return
	}
	err := &PanicError{Value: v, Stack: debugStack()}
	if logger != nil {
		logger.Error("recovered from panic", append(attrs, "panic", v, "stack", string(err.Stack))...)
	}
	if onPanic != nil {
		onPanic(err)
	}
}

func debugStack() []byte {
	buf := make([]byte, 8192)
	n := runtime.Stack(buf, false)
	return buf[:n]
}
