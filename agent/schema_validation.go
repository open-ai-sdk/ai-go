package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// StructuredOutputErrorKind identifies which structured-output boundary
// failed.
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
// validating structured output. Path and Reason remain source-compatible with
// the original validation-only error.
type StructuredOutputError struct {
	Kind   StructuredOutputErrorKind
	Path   string
	Reason string
	Cause  error
}

func (e *StructuredOutputError) Error() string {
	if e == nil {
		return "agent: structured output failed"
	}
	reason := e.Reason
	if reason == "" && e.Cause != nil {
		reason = e.Cause.Error()
	}
	if reason == "" {
		reason = string(e.Kind)
	}
	if e.Path == "" {
		return "agent: structured output " + reason
	}
	return fmt.Sprintf("agent: structured output at %s %s", e.Path, reason)
}

func (e *StructuredOutputError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Is permits kind matching with errors.Is(err,
// &StructuredOutputError{Kind: kind}). Use errors.As for every kind.
func (e *StructuredOutputError) Is(target error) bool {
	want, ok := target.(*StructuredOutputError)
	return ok && e != nil && want != nil && want.Kind != "" && e.Kind == want.Kind
}

func validateStructuredOutput(raw json.RawMessage, output *OutputSchema) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return &StructuredOutputError{
			Kind: StructuredOutputErrorKindJSONDecode, Path: "$",
			Reason: "is invalid JSON: " + err.Error(), Cause: err,
		}
	}
	schema := output.Schema
	if schema == nil {
		schema = map[string]any{"type": output.Type}
		if output.Type == "json_object" {
			schema["type"] = "object"
		}
	}

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.AssertContent()
	const schemaURL = "urn:ai-go:structured-output-schema"
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return structuredSchemaError(err)
	}
	normalizedSchema, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return structuredSchemaError(err)
	}
	if err := compiler.AddResource(schemaURL, normalizedSchema); err != nil {
		return structuredSchemaError(err)
	}
	compiled, err := compiler.Compile(schemaURL)
	if err != nil {
		return structuredSchemaError(err)
	}
	if err := compiled.Validate(value); err != nil {
		return structuredValidationError(err)
	}
	return nil
}

func structuredSchemaError(err error) error {
	return &StructuredOutputError{
		Kind: StructuredOutputErrorKindValidation, Path: "$schema",
		Reason: "is invalid: " + err.Error(), Cause: err,
	}
}

func structuredValidationError(err error) error {
	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) {
		return &StructuredOutputError{
			Kind: StructuredOutputErrorKindValidation, Path: "$",
			Reason: "does not satisfy schema: " + err.Error(), Cause: err,
		}
	}
	leaf := validationErr
	for len(leaf.Causes) > 0 {
		leaf = leaf.Causes[0]
	}
	path := "$"
	for _, token := range leaf.InstanceLocation {
		if strings.IndexByte(token, '.') >= 0 {
			path += "[" + fmt.Sprintf("%q", token) + "]"
		} else {
			path += "." + token
		}
	}
	return &StructuredOutputError{
		Kind: StructuredOutputErrorKindValidation, Path: path,
		Reason: "does not satisfy schema: " + leaf.Error(), Cause: err,
	}
}
