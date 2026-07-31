package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/open-ai-sdk/ai-go/tool"
)

// GenerateObjectRequest configures a single structured-output call whose
// result is unmarshalled into a caller-supplied type T. It carries the same
// model/message/settings surface as GenerateTextRequest, minus the tool-loop
// fields (Tools, StopWhen, ...) that a structured-output-only call has no use
// for.
type GenerateObjectRequest struct {
	// Model is the language model to call.
	Model LanguageModel
	// Instructions is an optional system prompt prepended before the conversation.
	Instructions string
	// Messages is the conversation history.
	Messages []Message
	// Settings controls per-request model parameters (temperature, maxTokens, etc.).
	Settings CallSettings
	// ProviderOptions carries provider-specific options keyed by provider name.
	ProviderOptions map[string]any
	// Middlewares holds deferred model middlewares set via WithMiddleware or
	// WithRetry, applied the same way GenerateTextRequest.Middlewares is.
	Middlewares []LanguageModelMiddleware
}

// ObjectResult holds the outcome of a GenerateObject call: the model's
// response already unmarshalled into T, plus the usage/finish metadata
// GenerateText reports for its final step.
type ObjectResult[T any] struct {
	Object           T
	Usage            Usage
	FinishReason     FinishReason
	RawFinishReason  string
	Warnings         []Warning
	ProviderMetadata map[string]any
}

// GenerateObject runs a single structured-output call and unmarshals the
// result into T.
//
// The JSON Schema sent to the model is derived from T's exported struct
// fields via reflection — the same derivation tool.New uses for tool input
// schemas, so a caller never hand-builds a schema.OutputObject map. T must be
// a struct (the same constraint tool.New has); nested structs and slices in T
// are supported, mirroring tool.New's field-schema derivation.
//
// GenerateObject is a typed convenience over GenerateText with req.Output set:
// callers who need the raw JSON (e.g. to defer unmarshalling, or because the
// shape is only known at runtime) still use GenerateText directly — this
// entry point does not replace it.
func GenerateObject[T any](ctx context.Context, req GenerateObjectRequest) (ObjectResult[T], error) {
	schema, err := tool.Schema[T]()
	if err != nil {
		return ObjectResult[T]{}, fmt.Errorf("ai: GenerateObject: %w", err)
	}

	result, err := GenerateText(ctx, GenerateTextRequest{
		Model:           req.Model,
		Instructions:    req.Instructions,
		Messages:        req.Messages,
		Settings:        req.Settings,
		ProviderOptions: req.ProviderOptions,
		Middlewares:     req.Middlewares,
		Output:          OutputObject(schema),
		MaxSteps:        1,
	})
	if err != nil {
		return ObjectResult[T]{}, err
	}
	if len(result.StructuredOutput) == 0 {
		return ObjectResult[T]{}, fmt.Errorf("ai: GenerateObject: model did not return structured output")
	}

	var obj T
	if err := json.Unmarshal(result.StructuredOutput, &obj); err != nil {
		return ObjectResult[T]{}, fmt.Errorf("ai: GenerateObject: unmarshal result: %w", err)
	}

	return ObjectResult[T]{
		Object:           obj,
		Usage:            result.Usage,
		FinishReason:     result.FinishReason,
		RawFinishReason:  result.RawFinishReason,
		Warnings:         result.Warnings,
		ProviderMetadata: result.ProviderMetadata,
	}, nil
}
