package llm

import "fmt"

// NativeSchemaSupport describes whether a model can enforce a JSON Schema and
// whether that constraint composes with tool declarations.
type NativeSchemaSupport int

const (
	NativeSchemaNone NativeSchemaSupport = iota
	NativeSchemaFull
	NativeSchemaSuppressesTools
)

// NativeSchemaCapable is implemented by providers that can report their
// schema capability before a request is sent.
type NativeSchemaCapable interface {
	NativeSchemaSupport() NativeSchemaSupport
}

// StructuredOutputErrorKind identifies which structured-output boundary failed.
type StructuredOutputErrorKind string

const (
	StructuredOutputErrorKindPrompt     StructuredOutputErrorKind = "prompt"
	StructuredOutputErrorKindJSONDecode StructuredOutputErrorKind = "json_decode"
	StructuredOutputErrorKindValidation StructuredOutputErrorKind = "validation"
	StructuredOutputErrorKindEmpty      StructuredOutputErrorKind = "empty"

	StructuredOutputErrorKindPromptFailure = StructuredOutputErrorKindPrompt
	StructuredOutputErrorKindEmptyResponse = StructuredOutputErrorKindEmpty
)

// StructuredOutputError reports a failure while producing, decoding, or
// validating structured output.
type StructuredOutputError struct {
	Kind   StructuredOutputErrorKind
	Path   string
	Reason string
	Cause  error
}

func (e *StructuredOutputError) Error() string {
	if e == nil {
		return "llm: structured output failed"
	}
	reason := e.Reason
	if reason == "" && e.Cause != nil {
		reason = e.Cause.Error()
	}
	if reason == "" {
		reason = string(e.Kind)
	}
	if e.Path == "" {
		return "llm: structured output " + reason
	}
	return fmt.Sprintf("llm: structured output at %s %s", e.Path, reason)
}

func (e *StructuredOutputError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *StructuredOutputError) Is(target error) bool {
	want, ok := target.(*StructuredOutputError)
	return ok && e != nil && want != nil && want.Kind != "" && e.Kind == want.Kind
}
