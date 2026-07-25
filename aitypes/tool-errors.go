package aitypes

import "fmt"

// NoSuchToolError indicates the model called a tool that is not active. It is
// defined once here so a value raised inside the engine's tool loop is the same
// type the caller matches with errors.As — previously the engine and ai layers
// each had their own copy, so errors.As matched in no code path.
type NoSuchToolError struct {
	ToolName       string
	AvailableTools []string
}

func (e *NoSuchToolError) Error() string {
	if len(e.AvailableTools) == 0 {
		return fmt.Sprintf("unknown tool %q", e.ToolName)
	}
	return fmt.Sprintf("unknown tool %q (available: %v)", e.ToolName, e.AvailableTools)
}

// Is lets errors.Is(err, ErrNoSuchTool) classify any NoSuchToolError.
func (e *NoSuchToolError) Is(target error) bool { return target == ErrNoSuchTool }

// InvalidToolArgumentsError indicates the model produced tool arguments that
// were not valid JSON. Cause carries the underlying decode error.
type InvalidToolArgumentsError struct {
	ToolName string
	Args     string
	Cause    error
}

func (e *InvalidToolArgumentsError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("invalid arguments for tool %q", e.ToolName)
}

func (e *InvalidToolArgumentsError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
