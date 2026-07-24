package safego

import (
	"errors"
	"testing"
)

func TestRecover_CapturesPanicValueAndStack(t *testing.T) {
	var got error
	func() {
		defer Recover(nil, func(err error) { got = err })
		panic("boom")
	}()

	if got == nil {
		t.Fatal("expected onPanic to be invoked")
	}
	var pe *PanicError
	if !errors.As(got, &pe) {
		t.Fatalf("expected *PanicError, got %T", got)
	}
	if pe.Value != "boom" {
		t.Errorf("Value = %v, want boom", pe.Value)
	}
	if len(pe.Stack) == 0 {
		t.Error("expected a non-empty stack trace")
	}
}

func TestRecover_NoPanicIsNoOp(t *testing.T) {
	called := false
	func() {
		defer Recover(nil, func(error) { called = true })
	}()
	if called {
		t.Error("onPanic must not be called when nothing panicked")
	}
}

func TestRecover_InvokesOnPanicExactlyOnce(t *testing.T) {
	count := 0
	func() {
		defer Recover(nil, func(error) { count++ })
		panic("once")
	}()
	if count != 1 {
		t.Errorf("onPanic called %d times, want 1", count)
	}
}
