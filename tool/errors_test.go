package tool_test

import (
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/tool"
)

func TestTypedErrorsSupportIsAndAs(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		sentinel error
		target   any
	}{
		{
			name:     "input",
			err:      &tool.InputError{ToolName: "search", Cause: errors.New("bad input")},
			sentinel: tool.ErrInput,
			target:   new(*tool.InputError),
		},
		{
			name:     "execution",
			err:      &tool.ExecutionError{ToolName: "search", Cause: errors.New("failed")},
			sentinel: tool.ErrExecution,
			target:   new(*tool.ExecutionError),
		},
		{
			name:     "denied",
			err:      &tool.DeniedError{ToolName: "search", Reason: "policy"},
			sentinel: tool.ErrDenied,
			target:   new(*tool.DeniedError),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !errors.Is(test.err, test.sentinel) {
				t.Fatalf("errors.Is(%v, %v) = false", test.err, test.sentinel)
			}
			if !errors.As(test.err, test.target) {
				t.Fatalf("errors.As(%T) = false", test.err)
			}
		})
	}
}
