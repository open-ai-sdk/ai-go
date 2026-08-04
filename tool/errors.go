package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrorKind is a stable, safe-to-present classification of a tool failure.
type ErrorKind string

const (
	ErrorKindInput     ErrorKind = "input"
	ErrorKindExecution ErrorKind = "execution"
	ErrorKindDenied    ErrorKind = "denied"
	ErrorKindNotFound  ErrorKind = "not_found"
	ErrorKindCancelled ErrorKind = "cancelled"
	ErrorKindTimeout   ErrorKind = "timeout"
	ErrorKindOther     ErrorKind = "other"
)

// ErrorDetails contains an optional application classification and a safe
// model presentation. Causes intentionally remain in the Go error chain.
type ErrorDetails struct {
	Kind        ErrorKind
	Retryable   *bool
	Code        string
	HTTPStatus  int
	Refusal     bool
	ModelOutput Output
}

// DetailedError lets applications provide safe tool-error presentation.
type DetailedError interface {
	error
	ToolErrorDetails() ErrorDetails
}

// Details normalizes a tool error without including arbitrary cause text in
// model-visible content.
func Details(err error) ErrorDetails {
	if err == nil {
		return ErrorDetails{}
	}
	var detailed DetailedError
	if errors.As(err, &detailed) {
		return detailed.ToolErrorDetails()
	}
	if errors.Is(err, context.Canceled) {
		return ErrorDetails{Kind: ErrorKindCancelled, ModelOutput: Text("Tool execution was cancelled.")}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorDetails{Kind: ErrorKindTimeout, ModelOutput: Text("Tool execution timed out.")}
	}
	if errors.Is(err, ErrInput) {
		return ErrorDetails{Kind: ErrorKindInput, ModelOutput: Text("Tool input was invalid.")}
	}
	if errors.Is(err, ErrDenied) {
		return ErrorDetails{Kind: ErrorKindDenied, ModelOutput: Text("Tool execution was denied.")}
	}
	if errors.Is(err, ErrNoSuchTool) {
		return ErrorDetails{Kind: ErrorKindNotFound, ModelOutput: Text("The requested tool is unavailable.")}
	}
	if errors.Is(err, ErrExecution) {
		return ErrorDetails{Kind: ErrorKindExecution, ModelOutput: Text("Tool execution failed.")}
	}
	return ErrorDetails{Kind: ErrorKindOther, ModelOutput: Text("Tool execution failed.")}
}

var (
	// ErrInput matches invalid or unsupported tool input.
	ErrInput = errors.New("tool: invalid input")
	// ErrExecution matches a failure while executing a tool.
	ErrExecution = errors.New("tool: execution failed")
	// ErrDenied matches a tool call rejected by policy.
	ErrDenied = errors.New("tool: denied by policy")
	// ErrNoSuchTool matches a call to an unregistered tool.
	ErrNoSuchTool = errors.New("tool: no such tool")

	errNilTool = errors.New("nil tool")
)

// InputError reports input that could not be decoded or validated.
type InputError struct {
	ToolName string
	Input    json.RawMessage
	Cause    error
}

func (e *InputError) Error() string {
	if e == nil {
		return ErrInput.Error()
	}
	if e.ToolName == "" {
		return fmt.Sprintf("%s: %v", ErrInput, e.Cause)
	}
	return fmt.Sprintf("%s for %q: %v", ErrInput, e.ToolName, e.Cause)
}

func (e *InputError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *InputError) Is(target error) bool { return target == ErrInput }

// ExecutionError reports a failure raised while a tool handler was running.
type ExecutionError struct {
	ToolName string
	Cause    error
}

func (e *ExecutionError) Error() string {
	if e == nil {
		return ErrExecution.Error()
	}
	if e.ToolName == "" {
		return fmt.Sprintf("%s: %v", ErrExecution, e.Cause)
	}
	return fmt.Sprintf("%s for %q: %v", ErrExecution, e.ToolName, e.Cause)
}

func (e *ExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *ExecutionError) Is(target error) bool { return target == ErrExecution }

// DeniedError reports a tool call rejected by policy.
type DeniedError struct {
	ToolName string
	Reason   string
	Cause    error
}

func (e *DeniedError) Error() string {
	if e == nil {
		return ErrDenied.Error()
	}
	message := ErrDenied.Error()
	if e.ToolName != "" {
		message += fmt.Sprintf(" for %q", e.ToolName)
	}
	if e.Reason != "" {
		message += ": " + e.Reason
	}
	return message
}

func (e *DeniedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *DeniedError) Is(target error) bool { return target == ErrDenied }

// NoSuchToolError indicates that a model called an unregistered tool.
type NoSuchToolError struct {
	ToolName       string
	AvailableTools []string
}

func (e *NoSuchToolError) Error() string {
	if len(e.AvailableTools) == 0 {
		return fmt.Sprintf("unknown tool %q", e.ToolName)
	}
	return fmt.Sprintf(
		"unknown tool %q (available: %v)",
		e.ToolName,
		e.AvailableTools,
	)
}

func (e *NoSuchToolError) Is(target error) bool {
	return target == ErrNoSuchTool || target == ErrInput
}
