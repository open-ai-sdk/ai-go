package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// DecodeStructured applies the shared structured-output acceptance rule: find
// the first JSON value in model text, validate it against output, then decode
// it into T. It accepts fenced or prose-wrapped output but never accepts a
// schema-invalid value.
func DecodeStructured[T any](text string, output *OutputSchema) (T, error) {
	var value T
	raw, err := ValidStructuredJSON(text, output)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, &StructuredOutputError{
			Kind:   StructuredOutputErrorKindJSONDecode,
			Path:   "$",
			Reason: "cannot decode JSON: " + err.Error(),
			Cause:  err,
		}
	}
	return value, nil
}

// ValidStructuredJSON extracts and validates the first JSON value in model
// text according to output.
func ValidStructuredJSON(text string, output *OutputSchema) (json.RawMessage, error) {
	if strings.TrimSpace(text) == "" {
		return nil, &StructuredOutputError{Kind: StructuredOutputErrorKindEmpty, Path: "$", Reason: "is empty"}
	}
	raw := FirstJSONValue(text)
	if raw == nil {
		return nil, &StructuredOutputError{
			Kind:   StructuredOutputErrorKindJSONDecode,
			Path:   "$",
			Reason: "is invalid JSON",
		}
	}
	if err := ValidateStructuredOutput(raw, output); err != nil {
		return nil, err
	}
	return raw, nil
}

// FirstJSONValue returns the first complete JSON value embedded in text.
func FirstJSONValue(text string) json.RawMessage {
	for offset, value := range text {
		if !jsonStart(value) {
			continue
		}
		decoder := json.NewDecoder(strings.NewReader(text[offset:]))
		var raw json.RawMessage
		if decoder.Decode(&raw) == nil {
			return raw
		}
	}
	return nil
}

func jsonStart(value rune) bool {
	return value == '{' || value == '[' || value == '"' || value == '-' ||
		(value >= '0' && value <= '9') || value == 't' || value == 'f' || value == 'n'
}

// ValidateStructuredOutput validates one JSON value with the same nil-schema
// policy used by every structured-output surface.
func ValidateStructuredOutput(raw json.RawMessage, output *OutputSchema) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return &StructuredOutputError{
			Kind:   StructuredOutputErrorKindJSONDecode,
			Path:   "$",
			Reason: "is invalid JSON: " + err.Error(),
			Cause:  err,
		}
	}
	if value == nil && output != nil && output.Type == "json" && output.Schema == nil {
		return &StructuredOutputError{Kind: StructuredOutputErrorKindValidation, Path: "$", Reason: "must not be null"}
	}
	return validateSchema(output, value)
}

// ValidateOutputConfiguration rejects unsupported output modes and invalid
// schemas before a request is sent.
func ValidateOutputConfiguration(output *OutputSchema) error {
	if output == nil || output.Type == "text" {
		return nil
	}
	switch output.Type {
	case "json", "json_object", "object", "array":
	default:
		return &StructuredOutputError{
			Kind:   StructuredOutputErrorKindValidation,
			Path:   "$schema.type",
			Reason: fmt.Sprintf("has unsupported output type %q", output.Type),
		}
	}
	_, err := compileStructuredSchema(output)
	return err
}

func validateSchema(output *OutputSchema, value any) error {
	compiled, err := compileStructuredSchema(output)
	if err != nil {
		return err
	}
	if err := compiled.Validate(value); err != nil {
		return &StructuredOutputError{
			Kind:   StructuredOutputErrorKindValidation,
			Path:   "$",
			Reason: fmt.Sprintf("is invalid: %v", err),
			Cause:  err,
		}
	}
	return nil
}

func compileStructuredSchema(output *OutputSchema) (*jsonschema.Schema, error) {
	var schema any
	if output != nil && output.Schema != nil {
		schema = output.Schema
	}
	if schema == nil {
		if output == nil || output.Type == "json" {
			schema = true
		} else if output.Type == "json_object" {
			schema = map[string]any{"type": "object"}
		} else {
			schema = map[string]any{"type": output.Type}
		}
	}
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return nil, &StructuredOutputError{
			Kind:   StructuredOutputErrorKindValidation,
			Path:   "$schema",
			Reason: "is invalid: " + err.Error(),
			Cause:  err,
		}
	}
	normalized, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return nil, &StructuredOutputError{
			Kind:   StructuredOutputErrorKindValidation,
			Path:   "$schema",
			Reason: "is invalid: " + err.Error(),
			Cause:  err,
		}
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.AssertContent()
	const schemaURL = "urn:ai-go:structured-output-schema"
	if err := compiler.AddResource(schemaURL, normalized); err != nil {
		return nil, &StructuredOutputError{
			Kind:   StructuredOutputErrorKindValidation,
			Path:   "$schema",
			Reason: "is invalid: " + err.Error(),
			Cause:  err,
		}
	}
	compiled, err := compiler.Compile(schemaURL)
	if err != nil {
		return nil, &StructuredOutputError{
			Kind:   StructuredOutputErrorKindValidation,
			Path:   "$schema",
			Reason: "is invalid: " + err.Error(),
			Cause:  err,
		}
	}
	return compiled, nil
}
