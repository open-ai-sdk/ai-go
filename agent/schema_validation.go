package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// StructuredOutputError reports JSON that is syntactically valid but does not
// satisfy the requested output schema.
type StructuredOutputError struct {
	Path   string
	Reason string
}

func (e *StructuredOutputError) Error() string {
	if e.Path == "" {
		return "agent: structured output " + e.Reason
	}
	return fmt.Sprintf("agent: structured output at %s %s", e.Path, e.Reason)
}

func validateStructuredOutput(raw json.RawMessage, output *OutputSchema) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return &StructuredOutputError{Path: "$", Reason: "is invalid JSON: " + err.Error()}
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
		return &StructuredOutputError{Path: "$schema", Reason: "is invalid: " + err.Error()}
	}
	normalizedSchema, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return &StructuredOutputError{Path: "$schema", Reason: "is invalid: " + err.Error()}
	}
	if err := compiler.AddResource(schemaURL, normalizedSchema); err != nil {
		return &StructuredOutputError{Path: "$schema", Reason: "is invalid: " + err.Error()}
	}
	compiled, err := compiler.Compile(schemaURL)
	if err != nil {
		return &StructuredOutputError{Path: "$schema", Reason: "is invalid: " + err.Error()}
	}
	if err := compiled.Validate(value); err != nil {
		return structuredValidationError(err)
	}
	return nil
}

func structuredValidationError(err error) error {
	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) {
		return &StructuredOutputError{Path: "$", Reason: "does not satisfy schema: " + err.Error()}
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
	return &StructuredOutputError{Path: path, Reason: "does not satisfy schema: " + leaf.Error()}
}
